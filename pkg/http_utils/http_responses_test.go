package http_utils

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDumpRequestPermissive_PreservesCRLFInHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.URL = &url.URL{Scheme: "http", Host: "example.com", Path: "/"}
	req.Header = http.Header{
		"X-Injected": []string{"foo\r\nX-Smuggled: evil"},
	}
	out := dumpRequestPermissive(req, nil)
	if out == nil {
		t.Fatal("expected non-nil dump")
	}
	s := string(out)
	if !strings.Contains(s, "X-Injected: foo\r\nX-Smuggled: evil") {
		t.Fatalf("permissive dump did not preserve raw header value, got:\n%q", s)
	}
	if !strings.HasPrefix(s, "GET / HTTP/1.1\r\n") {
		t.Fatalf("missing request line, got:\n%q", s)
	}
	if !strings.Contains(s, "Host: example.com\r\n") {
		t.Fatalf("missing host line, got:\n%q", s)
	}
}

func TestDumpRequestPermissive_RespectsExplicitHost(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://example.com/x", strings.NewReader("body"))
	req.Header.Set("Host", "smuggled.example.com")
	out := dumpRequestPermissive(req, []byte("body"))
	s := string(out)
	if strings.Count(s, "Host:") != 1 {
		t.Fatalf("expected exactly one Host header, got:\n%q", s)
	}
	if !strings.Contains(s, "Host: smuggled.example.com\r\n") {
		t.Fatalf("explicit Host not preserved, got:\n%q", s)
	}
	if !strings.HasSuffix(s, "body") {
		t.Fatalf("body not appended, got tail:\n%q", s[len(s)-10:])
	}
}
