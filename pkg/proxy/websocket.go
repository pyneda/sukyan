package proxy

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// WebSocket opcodes
const (
	OpcodeContinuation = 0x0
	OpcodeText         = 0x1
	OpcodeBinary       = 0x2
	OpcodeClose        = 0x8
	OpcodePing         = 0x9
	OpcodePong         = 0xA
)

// MaxFrameSize is the maximum size of a single WebSocket frame we'll buffer
// Messages larger than this will be truncated in storage but still proxied
const MaxFrameSize = 1024 * 1024 // 1MB

// frameAssembler handles reassembly of fragmented WebSocket messages
type frameAssembler struct {
	buffer        bytes.Buffer
	startOpcode   uint8 // The opcode of the first frame in the sequence
	isCompressed  bool  // RSV1 of the first frame
	inProgress    bool  // Whether we're currently assembling a fragmented message
	maxSize       int
	truncated     bool
	truncatedSize int64
}

func newFrameAssembler(maxSize int) *frameAssembler {
	return &frameAssembler{
		maxSize: maxSize,
	}
}

func (fa *frameAssembler) reset() {
	fa.buffer.Reset()
	fa.startOpcode = 0
	fa.isCompressed = false
	fa.inProgress = false
	fa.truncated = false
	fa.truncatedSize = 0
}

// addFrame adds a frame to the assembler and returns (completePayload, opcode, isCompressed, isComplete)
func (fa *frameAssembler) addFrame(frame *WebSocketFrame) ([]byte, uint8, bool, bool) {
	isControlFrame := frame.Opcode >= OpcodeClose

	// Control frames (close, ping, pong) are never fragmented and can appear
	// in the middle of a fragmented message sequence
	if isControlFrame {
		return frame.PayloadData, frame.Opcode, false, true
	}

	if frame.Opcode != OpcodeContinuation {
		// This is the start of a new message (or a complete single-frame message)
		fa.reset()
		fa.startOpcode = frame.Opcode
		fa.isCompressed = frame.RSV1
		fa.inProgress = !frame.Fin

		if len(frame.PayloadData) <= fa.maxSize {
			fa.buffer.Write(frame.PayloadData)
		} else {
			// Truncate but track original size
			fa.buffer.Write(frame.PayloadData[:fa.maxSize])
			fa.truncated = true
			fa.truncatedSize = int64(len(frame.PayloadData))
		}

		if frame.Fin {
			// Complete single-frame message
			result := make([]byte, fa.buffer.Len())
			copy(result, fa.buffer.Bytes())
			opcode := fa.startOpcode
			compressed := fa.isCompressed
			fa.reset()
			return result, opcode, compressed, true
		}
		return nil, 0, false, false
	}

	// Continuation frame
	if !fa.inProgress {
		// Orphan continuation frame - shouldn't happen in valid WebSocket
		log.Warn().Msg("Received continuation frame without start frame")
		return nil, 0, false, false
	}

	remainingSpace := fa.maxSize - fa.buffer.Len()
	if remainingSpace > 0 {
		if len(frame.PayloadData) <= remainingSpace {
			fa.buffer.Write(frame.PayloadData)
		} else {
			fa.buffer.Write(frame.PayloadData[:remainingSpace])
			fa.truncated = true
			fa.truncatedSize += int64(len(frame.PayloadData))
		}
	} else {
		fa.truncatedSize += int64(len(frame.PayloadData))
	}

	if frame.Fin {
		// Message complete
		result := make([]byte, fa.buffer.Len())
		copy(result, fa.buffer.Bytes())
		opcode := fa.startOpcode
		compressed := fa.isCompressed
		fa.reset()
		return result, opcode, compressed, true
	}

	return nil, 0, false, false
}

// WebSocketInterceptor handles WebSocket message interception and storage
type WebSocketInterceptor struct {
	connection  *db.WebSocketConnection
	workspaceID uint
	messageChan chan *db.WebSocketMessage
	wg          sync.WaitGroup
	done        chan struct{}
	closed      bool
	closeMu     sync.Mutex

	// Compression enabled for this connection (permessage-deflate)
	compressionEnabled bool

	// Frame assemblers for each direction (to handle fragmented messages)
	clientAssembler *frameAssembler
	serverAssembler *frameAssembler

	// Read buffers for handling partial frame reads
	clientReadBuffer bytes.Buffer
	serverReadBuffer bytes.Buffer
	bufferMu         sync.Mutex
}

func NewWebSocketInterceptor(connection *db.WebSocketConnection, workspaceID uint, compressionEnabled bool) *WebSocketInterceptor {
	interceptor := &WebSocketInterceptor{
		connection:         connection,
		workspaceID:        workspaceID,
		messageChan:        make(chan *db.WebSocketMessage, 1000),
		done:               make(chan struct{}),
		compressionEnabled: compressionEnabled,
		clientAssembler:    newFrameAssembler(MaxFrameSize),
		serverAssembler:    newFrameAssembler(MaxFrameSize),
	}

	interceptor.wg.Add(1)
	go interceptor.messageProcessor()

	return interceptor
}

func (w *WebSocketInterceptor) Close() {
	w.closeMu.Lock()
	if w.closed {
		w.closeMu.Unlock()
		return
	}
	w.closed = true
	w.closeMu.Unlock()

	close(w.done)
	w.wg.Wait()

	// Update ClosedAt timestamp
	now := time.Now()
	w.connection.ClosedAt = now
	err := db.Connection().UpdateWebSocketConnection(w.connection)
	if err != nil {
		log.Error().Uint("workspace", w.workspaceID).Err(err).Str("url", w.connection.URL).Msg("Failed to update WebSocket connection closed at")
	}
}

// decompressPayload decompresses a permessage-deflate compressed WebSocket payload.
// For permessage-deflate, the compression uses raw DEFLATE (no zlib header).
// Each message needs the DEFLATE flush marker appended before decompression.
func (w *WebSocketInterceptor) decompressPayload(payload []byte) ([]byte, error) {
	// permessage-deflate requires appending the DEFLATE tail before decompression
	// The tail is: 0x00 0x00 0xff 0xff (BFINAL=0, BTYPE=00, empty stored block)
	deflateTail := []byte{0x00, 0x00, 0xff, 0xff}
	compressedData := make([]byte, len(payload)+len(deflateTail))
	copy(compressedData, payload)
	copy(compressedData[len(payload):], deflateTail)

	// Create a new flate reader for each message
	reader := flate.NewReader(bytes.NewReader(compressedData))
	defer reader.Close()

	var decompressed bytes.Buffer
	_, err := io.Copy(&decompressed, reader)
	if err != nil {
		return nil, err
	}

	return decompressed.Bytes(), nil
}

func (w *WebSocketInterceptor) messageProcessor() {
	defer w.wg.Done()

	for {
		select {
		case message := <-w.messageChan:
			err := db.Connection().CreateWebSocketMessage(message)
			if err != nil {
				log.Error().Uint("workspace", w.workspaceID).Err(err).Str("data", message.PayloadData).Msg("Failed to create WebSocket message")
			}
		case <-w.done:
			// Drain remaining messages before exiting
			for {
				select {
				case message := <-w.messageChan:
					err := db.Connection().CreateWebSocketMessage(message)
					if err != nil {
						log.Error().Uint("workspace", w.workspaceID).Err(err).Msg("Failed to create WebSocket message during shutdown")
					}
				default:
					return
				}
			}
		}
	}
}

func (w *WebSocketInterceptor) getAssembler(direction db.MessageDirection) *frameAssembler {
	if direction == db.MessageSent {
		return w.clientAssembler
	}
	return w.serverAssembler
}

func (w *WebSocketInterceptor) getReadBuffer(direction db.MessageDirection) *bytes.Buffer {
	if direction == db.MessageSent {
		return &w.clientReadBuffer
	}
	return &w.serverReadBuffer
}

func (w *WebSocketInterceptor) InterceptedCopy(dst io.Writer, src io.Reader, direction db.MessageDirection) (written int64, err error) {
	buffer := make([]byte, 64*1024) // Increased to 64KB for better performance
	assembler := w.getAssembler(direction)
	readBuffer := w.getReadBuffer(direction)

	directionStr := "received"
	if direction == db.MessageSent {
		directionStr = "sent"
	}

	for {
		nr, er := src.Read(buffer)
		if nr > 0 {
			// Write to destination immediately (proxy function)
			nw, ew := dst.Write(buffer[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrShortWrite
				}
			}
			written += int64(nw)

			// Now process for interception (append to read buffer)
			w.bufferMu.Lock()
			readBuffer.Write(buffer[:nr])

			// Try to parse complete frames from the buffer
			for {
				data := readBuffer.Bytes()
				if len(data) < 2 {
					break // Need at least 2 bytes for frame header
				}

				frame, frameLen, parseErr := parseWebSocketFrameWithLength(data)
				if parseErr == io.ErrShortBuffer {
					// Not enough data for a complete frame, wait for more
					break
				}
				if parseErr != nil {
					log.Debug().Err(parseErr).Str("direction", directionStr).Msg("Failed to parse WebSocket frame")
					// Clear buffer on parse error to avoid infinite loop
					readBuffer.Reset()
					break
				}

				// Successfully parsed a frame, remove it from the buffer
				readBuffer.Next(frameLen)

				// Handle close frame - update ClosedAt immediately
				if frame.Opcode == OpcodeClose {
					log.Info().
						Uint("connection_id", w.connection.ID).
						Str("direction", directionStr).
						Msg("WebSocket close frame detected")
				}

				// Add frame to assembler for potential reassembly
				payload, opcode, isCompressed, isComplete := assembler.addFrame(frame)
				if !isComplete {
					continue // Waiting for more continuation frames
				}

				// We have a complete message, process it
				w.processCompleteMessage(payload, opcode, isCompressed, direction, directionStr)
			}
			w.bufferMu.Unlock()

			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func (w *WebSocketInterceptor) processCompleteMessage(payload []byte, opcode uint8, isCompressed bool, direction db.MessageDirection, directionStr string) {
	// Skip ping/pong frames from storage (they're just keep-alive)
	if opcode == OpcodePing || opcode == OpcodePong {
		log.Debug().
			Uint("connection_id", w.connection.ID).
			Str("direction", directionStr).
			Uint8("opcode", opcode).
			Msg("Skipping ping/pong frame")
		return
	}

	// If compressed, try to decompress
	if isCompressed && w.compressionEnabled && len(payload) > 0 {
		decompressed, decompErr := w.decompressPayload(payload)
		if decompErr == nil {
			payload = decompressed
			log.Debug().
				Uint("connection_id", w.connection.ID).
				Int("decompressed_len", len(decompressed)).
				Msg("Successfully decompressed WebSocket payload")
		} else {
			log.Warn().
				Err(decompErr).
				Uint("connection_id", w.connection.ID).
				Str("direction", directionStr).
				Msg("Failed to decompress WebSocket payload")
		}
	}

	// Check if payload is valid UTF-8, if not encode as base64
	var payloadStr string
	isBinary := !utf8.Valid(payload)
	if isBinary {
		payloadStr = base64.StdEncoding.EncodeToString(payload)
	} else {
		payloadStr = string(payload)
	}

	message := &db.WebSocketMessage{
		ConnectionID: w.connection.ID,
		Opcode:       float64(opcode),
		Mask:         false, // We unmask during parsing
		PayloadData:  payloadStr,
		IsBinary:     isBinary,
		Timestamp:    time.Now(),
		Direction:    direction,
	}

	log.Info().
		Uint("connection_id", w.connection.ID).
		Str("direction", directionStr).
		Uint8("opcode", opcode).
		Int("payload_len", len(payload)).
		Bool("compressed", isCompressed).
		Str("url", w.connection.URL).
		Msg("WebSocket message intercepted")

	// Use a timeout to avoid blocking forever, but give enough time for normal processing
	select {
	case w.messageChan <- message:
	case <-time.After(5 * time.Second):
		log.Warn().Uint("workspace", w.workspaceID).Uint("connection_id", w.connection.ID).Msg("WebSocket message channel full after 5s timeout, dropping message")
	case <-w.done:
		// Interceptor is closing, don't block
	}
}

type WebSocketFrame struct {
	Fin         bool
	RSV1        bool // Used by permessage-deflate to indicate compressed message
	Opcode      uint8
	Masked      bool
	PayloadData []byte
}

// parseWebSocketFrameWithLength parses a WebSocket frame and returns the frame and its total length in bytes
func parseWebSocketFrameWithLength(data []byte) (*WebSocketFrame, int, error) {
	if len(data) < 2 {
		return nil, 0, io.ErrShortBuffer
	}

	frame := &WebSocketFrame{}

	frame.Fin = (data[0] & 0x80) != 0
	frame.RSV1 = (data[0] & 0x40) != 0 // RSV1 bit indicates compression
	frame.Opcode = data[0] & 0x0F
	frame.Masked = (data[1] & 0x80) != 0

	payloadLen := uint64(data[1] & 0x7F)
	offset := 2

	if payloadLen == 126 {
		if len(data) < offset+2 {
			return nil, 0, io.ErrShortBuffer
		}
		payloadLen = uint64(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	} else if payloadLen == 127 {
		if len(data) < offset+8 {
			return nil, 0, io.ErrShortBuffer
		}
		payloadLen = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	var maskKey []byte
	if frame.Masked {
		if len(data) < offset+4 {
			return nil, 0, io.ErrShortBuffer
		}
		maskKey = data[offset : offset+4]
		offset += 4
	}

	if uint64(len(data)) < uint64(offset)+payloadLen {
		return nil, 0, io.ErrShortBuffer
	}

	frame.PayloadData = make([]byte, payloadLen)
	copy(frame.PayloadData, data[offset:uint64(offset)+payloadLen])

	if frame.Masked {
		for i := range frame.PayloadData {
			frame.PayloadData[i] ^= maskKey[i%4]
		}
	}

	totalLen := offset + int(payloadLen)
	return frame, totalLen, nil
}

// parseWebSocketFrame is kept for backwards compatibility
func parseWebSocketFrame(data []byte) (*WebSocketFrame, error) {
	frame, _, err := parseWebSocketFrameWithLength(data)
	return frame, err
}

func (w *WebSocketInterceptor) InterceptWebSocketTraffic(remoteConn io.ReadWriter, proxyClient io.ReadWriter) {
	defer w.Close()

	waitChan := make(chan struct{}, 2)

	go func() {
		_, _ = w.InterceptedCopy(remoteConn, proxyClient, db.MessageSent)
		waitChan <- struct{}{}
	}()

	go func() {
		_, _ = w.InterceptedCopy(proxyClient, remoteConn, db.MessageReceived)
		waitChan <- struct{}{}
	}()

	<-waitChan

	now := time.Now()
	w.connection.ClosedAt = now
	err := db.Connection().UpdateWebSocketConnection(w.connection)
	if err != nil {
		log.Error().Uint("workspace", w.workspaceID).Err(err).Str("url", w.connection.URL).Msg("Failed to update WebSocket connection closed at")
	}
}
