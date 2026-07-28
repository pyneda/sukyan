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
