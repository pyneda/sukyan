package proxy

import (
	"net/http"
	"testing"
)

func TestHeaderContains(t *testing.T) {
	headers := http.Header{}
	headers.Add("Connection", "Upgrade, keep-alive")
	headers.Add("Upgrade", "websocket")
	
	if !headerContains(headers, "Connection", "Upgrade") {
		t.Error("Expected headerContains to return true for Connection: Upgrade")
	}
	
	if !headerContains(headers, "Upgrade", "websocket") {
		t.Error("Expected headerContains to return true for Upgrade: websocket")
	}
	
	if headerContains(headers, "Connection", "close") {
		t.Error("Expected headerContains to return false for Connection: close")
	}
}

func TestWebSocketFrameParsing(t *testing.T) {
	// Simple text frame: FIN=1, opcode=1 (text), mask=0, payload="hello"
	frameData := []byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'}
	
	frame, err := parseWebSocketFrame(frameData)
	if err != nil {
		t.Fatalf("Failed to parse WebSocket frame: %v", err)
	}
	
	if !frame.Fin {
		t.Error("Expected FIN bit to be set")
	}
	
	if frame.Opcode != 1 {
		t.Errorf("Expected opcode 1 (text), got %d", frame.Opcode)
	}
	
	if frame.Masked {
		t.Error("Expected frame to be unmasked")
	}
	
	if string(frame.PayloadData) != "hello" {
		t.Errorf("Expected payload 'hello', got '%s'", string(frame.PayloadData))
	}
}