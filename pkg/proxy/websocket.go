package proxy

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// WebSocketInterceptor handles WebSocket message interception and storage
type WebSocketInterceptor struct {
	connection  *db.WebSocketConnection
	workspaceID uint
	messageChan chan *db.WebSocketMessage
	wg          sync.WaitGroup
	done        chan struct{}
}

func NewWebSocketInterceptor(connection *db.WebSocketConnection, workspaceID uint) *WebSocketInterceptor {
	interceptor := &WebSocketInterceptor{
		connection:  connection,
		workspaceID: workspaceID,
		messageChan: make(chan *db.WebSocketMessage, 100),
		done:        make(chan struct{}),
	}

	interceptor.wg.Add(1)
	go interceptor.messageProcessor()

	return interceptor
}

func (w *WebSocketInterceptor) Close() {
	close(w.done)
	w.wg.Wait()
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
			return
		}
	}
}

func (w *WebSocketInterceptor) InterceptedCopy(dst io.Writer, src io.Reader, direction db.MessageDirection) (written int64, err error) {
	buffer := make([]byte, 32*1024)

	for {
		nr, er := src.Read(buffer)
		if nr > 0 {
			frame, err := parseWebSocketFrame(buffer[:nr])
			if err == nil && frame != nil {
				message := &db.WebSocketMessage{
					ConnectionID: w.connection.ID,
					Opcode:       float64(frame.Opcode),
					Mask:         frame.Masked,
					PayloadData:  string(frame.PayloadData),
					Timestamp:    time.Now(),
					Direction:    direction,
				}

				select {
				case w.messageChan <- message:
				default:
					log.Warn().Uint("workspace", w.workspaceID).Msg("WebSocket message channel full, dropping message")
				}
			}

			nw, ew := dst.Write(buffer[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrShortWrite
				}
			}
			written += int64(nw)
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

type WebSocketFrame struct {
	Fin         bool
	Opcode      uint8
	Masked      bool
	PayloadData []byte
}

func parseWebSocketFrame(data []byte) (*WebSocketFrame, error) {
	if len(data) < 2 {
		return nil, io.ErrShortBuffer
	}

	frame := &WebSocketFrame{}

	frame.Fin = (data[0] & 0x80) != 0
	frame.Opcode = data[0] & 0x0F
	frame.Masked = (data[1] & 0x80) != 0

	payloadLen := uint64(data[1] & 0x7F)
	offset := 2

	if payloadLen == 126 {
		if len(data) < offset+2 {
			return nil, io.ErrShortBuffer
		}
		payloadLen = uint64(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	} else if payloadLen == 127 {
		if len(data) < offset+8 {
			return nil, io.ErrShortBuffer
		}
		payloadLen = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	var maskKey []byte
	if frame.Masked {
		if len(data) < offset+4 {
			return nil, io.ErrShortBuffer
		}
		maskKey = data[offset : offset+4]
		offset += 4
	}

	if len(data) < offset+int(payloadLen) {
		return nil, io.ErrShortBuffer
	}

	frame.PayloadData = make([]byte, payloadLen)
	copy(frame.PayloadData, data[offset:offset+int(payloadLen)])

	if frame.Masked {
		for i := range frame.PayloadData {
			frame.PayloadData[i] ^= maskKey[i%4]
		}
	}

	return frame, nil
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
