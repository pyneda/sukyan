package active

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/pkg/http_utils"
)

func brFrom(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestReadRawHTTPResponseContentLength(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"
	got, err := readRawHTTPResponse(brFrom(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != raw {
		t.Fatalf("got %q, want %q", got, raw)
	}
}

// The core guarantee: one call consumes exactly one message, so a second
// pipelined response stays available for the next call.
func TestReadRawHTTPResponseStopsAtMessageBoundary(t *testing.T) {
	first := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"
	second := "HTTP/1.1 501 Not Implemented\r\nContent-Length: 11\r\n\r\nXKQM marker"
	br := brFrom(first + second)

	got1, err := readRawHTTPResponse(br)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if string(got1) != first {
		t.Fatalf("first response: got %q, want %q", got1, first)
	}

	got2, err := readRawHTTPResponse(br)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if string(got2) != second {
		t.Fatalf("second response: got %q, want %q", got2, second)
	}
	if !bytes.Contains(got2, []byte("XKQM")) {
		t.Fatalf("marker missing from second response: %q", got2)
	}
}

func TestReadRawHTTPResponseChunked(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"
	next := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"
	br := brFrom(raw + next)

	got, err := readRawHTTPResponse(br)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != raw {
		t.Fatalf("got %q, want %q", got, raw)
	}
	// The following message must still be intact.
	got2, err := readRawHTTPResponse(br)
	if err != nil || string(got2) != next {
		t.Fatalf("follow-on message corrupted: got %q err=%v", got2, err)
	}
}

func TestReadRawHTTPResponseBodylessStatuses(t *testing.T) {
	for _, tc := range []string{
		"HTTP/1.1 204 No Content\r\n\r\n",
		"HTTP/1.1 304 Not Modified\r\n\r\n",
	} {
		next := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"
		br := brFrom(tc + next)
		got, err := readRawHTTPResponse(br)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tc, err)
		}
		if string(got) != tc {
			t.Fatalf("got %q, want %q", got, tc)
		}
		if got2, _ := readRawHTTPResponse(br); string(got2) != next {
			t.Fatalf("follow-on corrupted after bodyless status: %q", got2)
		}
	}
}

// A 1xx interim response must not be mistaken for the final one.
func TestReadRawHTTPResponseSkipsInterim(t *testing.T) {
	raw := "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"
	got, err := readRawHTTPResponse(brFrom(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(got, []byte("200 OK")) {
		t.Fatalf("final response not reached: %q", got)
	}
	if http_utils.ParseStatusCodeFromRawResponse(got) == 100 {
		t.Fatalf("returned the interim response instead of the final one: %q", got)
	}
}

// Regression test for the real defect: a single conn.Read() returns whatever is
// in the socket buffer, so a response split across segments leaked its tail into
// the *next* read and the smuggled marker was never seen.
func TestSendRawPipelinedReadsWholeResponsesWhenSplit(t *testing.T) {
	const marker = "XKQMZZ"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8192)

		// Read the smuggling payload, then answer in two TCP segments.
		_, _ = conn.Read(buf)
		body := "first-response-body-padding"
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body))
		time.Sleep(60 * time.Millisecond) // force a segment boundary mid-message
		_, _ = conn.Write([]byte(body))

		// Read the follow-up, then answer with the smuggled marker.
		_, _ = conn.Read(buf)
		second := "Method " + marker + " not implemented"
		fmt.Fprintf(conn, "HTTP/1.1 501 Not Implemented\r\nContent-Length: %d\r\n\r\n%s",
			len(second), second)
		time.Sleep(50 * time.Millisecond)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	client := NewSmugglingClient(5*time.Second, http_utils.HistoryCreationOptions{})

	resp, err := client.SendRawPipelined(context.Background(), "127.0.0.1", addr.Port, false,
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\n\r\n"),
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\n\r\nx=1"),
		marker)
	if err != nil {
		t.Fatalf("SendRawPipelined: %v", err)
	}

	if !bytes.Contains(resp.SecondResponse, []byte(marker)) {
		t.Fatalf("marker %q not found in SecondResponse; got first=%q second=%q",
			marker, resp.FirstResponse, resp.SecondResponse)
	}
	if bytes.Contains(resp.SecondResponse, []byte("first-response-body-padding")) {
		t.Fatalf("SecondResponse leaked the tail of the first response: %q", resp.SecondResponse)
	}
	if !resp.MarkerFound || resp.MarkerLocation != "second response" {
		t.Fatalf("marker bookkeeping wrong: found=%v location=%q",
			resp.MarkerFound, resp.MarkerLocation)
	}
}

// Both responses arriving in one segment must also be split correctly.
func TestSendRawPipelinedSplitsCoalescedResponses(t *testing.T) {
	const marker = "PLQW77"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8192)
		_, _ = conn.Read(buf)
		// Answer both requests at once, before the follow-up is even read.
		second := "Method " + marker + " not implemented"
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"+
			"HTTP/1.1 501 Not Implemented\r\nContent-Length: %d\r\n\r\n%s", len(second), second)
		time.Sleep(80 * time.Millisecond)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	client := NewSmugglingClient(5*time.Second, http_utils.HistoryCreationOptions{})
	resp, err := client.SendRawPipelined(context.Background(), "127.0.0.1", addr.Port, false,
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\n\r\n"),
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\n\r\nx=1"),
		marker)
	if err != nil {
		t.Fatalf("SendRawPipelined: %v", err)
	}
	if string(resp.FirstResponse) != "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok" {
		t.Fatalf("FirstResponse should hold exactly one message, got %q", resp.FirstResponse)
	}
	if !bytes.Contains(resp.SecondResponse, []byte(marker)) {
		t.Fatalf("marker not isolated into SecondResponse: %q", resp.SecondResponse)
	}
}

// A body-echoing endpoint returns the marker in its own response to the smuggling
// payload. That is reflection, not desync: the follow-up response is clean and the
// connection was never poisoned. Verified false twice in the field (casino/FastAPI
// 422, prototype-pollution-matrix/nginx 200), where it reported High at 95%.
func TestSendRawPipelinedFirstResponseEchoIsNotDesyncEvidence(t *testing.T) {
	const marker = "ZZQAPROBE"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8192)

		// First response echoes the request body, marker and all.
		_, _ = conn.Read(buf)
		echoed := "you sent: GET /" + marker + " HTTP/1.1"
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(echoed), echoed)

		// Follow-up is served normally: no smuggled request was ever queued.
		_, _ = conn.Read(buf)
		clean := "ok"
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(clean), clean)
		time.Sleep(50 * time.Millisecond)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	client := NewSmugglingClient(5*time.Second, http_utils.HistoryCreationOptions{})

	resp, err := client.SendRawPipelined(context.Background(), "127.0.0.1", addr.Port, false,
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 33\r\n\r\nGET /"+marker+" HTTP/1.1\r\n\r\n"),
		[]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"),
		marker)
	if err != nil {
		t.Fatalf("SendRawPipelined: %v", err)
	}

	if !bytes.Contains(resp.FirstResponse, []byte(marker)) {
		t.Fatalf("test setup wrong: first response should echo the marker, got %q", resp.FirstResponse)
	}
	if bytes.Contains(resp.SecondResponse, []byte(marker)) {
		t.Fatalf("test setup wrong: second response must be clean, got %q", resp.SecondResponse)
	}
	if resp.MarkerInSecondResponse {
		t.Fatalf("first-response echo must not count as desync evidence (location=%q)", resp.MarkerLocation)
	}
	if !resp.MarkerFound || resp.MarkerLocation != "first response" {
		t.Fatalf("the echo should still be recorded for diagnostics: found=%v location=%q",
			resp.MarkerFound, resp.MarkerLocation)
	}
}

// The genuine signal: the marker surfaces in the follow-up response.
func TestSendRawPipelinedSecondResponseMarkerIsDesyncEvidence(t *testing.T) {
	const marker = "QQDESYNC"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 8192)
		_, _ = conn.Read(buf)
		fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		_, _ = conn.Read(buf)
		second := "Method " + marker + " not implemented"
		fmt.Fprintf(conn, "HTTP/1.1 501 Not Implemented\r\nContent-Length: %d\r\n\r\n%s", len(second), second)
		time.Sleep(50 * time.Millisecond)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	client := NewSmugglingClient(5*time.Second, http_utils.HistoryCreationOptions{})
	resp, err := client.SendRawPipelined(context.Background(), "127.0.0.1", addr.Port, false,
		[]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\n\r\n"),
		[]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"),
		marker)
	if err != nil {
		t.Fatalf("SendRawPipelined: %v", err)
	}
	if !resp.MarkerInSecondResponse {
		t.Fatalf("marker in the follow-up must count as desync evidence: location=%q", resp.MarkerLocation)
	}
}
