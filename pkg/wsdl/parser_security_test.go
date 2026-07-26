package wsdl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// importingWSDL builds a WSDL whose wsdl:import points wherever the caller wants.
func importingWSDL(location string) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <import namespace="urn:i" location="%s"/>
</definitions>`, location)
}

func emptyWSDL() string {
	return `<?xml version="1.0"?><definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:i"/>`
}

// A WSDL is fetched from a target we do not trust. Its import locations must not
// be able to steer the scanner at unrelated internal hosts.
func TestImportsToOtherHostsAreNotFollowedByDefault(t *testing.T) {
	var mu sync.Mutex
	var internalHits int

	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		internalHits++
		mu.Unlock()
		fmt.Fprint(w, emptyWSDL())
	}))
	defer internal.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, importingWSDL(internal.URL+"/internal.wsdl"))
	}))
	defer target.Close()

	if _, err := NewParser().ParseFromURL(target.URL + "/svc?wsdl"); err != nil {
		t.Fatalf("parse should degrade gracefully, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if internalHits != 0 {
		t.Errorf("parser fetched %d cross-host import(s); attacker-controlled locations must not be followed by default", internalHits)
	}
}

// Same-origin imports are ordinary WSDL modularisation and must keep working.
func TestSameOriginImportsAreFollowed(t *testing.T) {
	var mu sync.Mutex
	var importHits int

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(r.URL.Path, "imported") {
			mu.Lock()
			importHits++
			mu.Unlock()
			fmt.Fprint(w, emptyWSDL())
			return
		}
		fmt.Fprint(w, importingWSDL(srv.URL+"/imported.wsdl"))
	}))
	defer srv.Close()

	if _, err := NewParser().ParseFromURL(srv.URL + "/svc?wsdl"); err != nil {
		t.Fatalf("parse: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if importHits == 0 {
		t.Error("same-origin import was not followed")
	}
}

// Credentials supplied for the target must never be replayed to a third party.
func TestCredentialsAreNotSentToOtherHosts(t *testing.T) {
	var mu sync.Mutex
	var leaked []string
	contacted := false

	third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		contacted = true
		if v := r.Header.Get("Authorization"); v != "" {
			leaked = append(leaked, "Authorization: "+v)
		}
		if v := r.Header.Get("Cookie"); v != "" {
			leaked = append(leaked, "Cookie: "+v)
		}
		mu.Unlock()
		fmt.Fprint(w, emptyWSDL())
	}))
	defer third.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, importingWSDL(third.URL+"/steal.wsdl"))
	}))
	defer target.Close()

	// Cross-origin imports must be enabled, otherwise the import is refused
	// before any request is made and this would pass without exercising the
	// credential gate at all.
	parser := NewParser().
		WithCrossOriginImports(true).
		WithHeaders(map[string]string{
			"Authorization": "Bearer scan-token",
			"Cookie":        "session=secret",
		})
	parser.ParseFromURL(target.URL + "/svc?wsdl")

	mu.Lock()
	defer mu.Unlock()
	if !contacted {
		t.Fatal("third-party host was never contacted; the credential gate was not exercised")
	}
	if len(leaked) > 0 {
		t.Errorf("scan credentials leaked to a third-party host: %v", leaked)
	}
}

func TestNonHTTPImportSchemesAreRejected(t *testing.T) {
	for _, location := range []string{
		"file:///etc/passwd",
		"gopher://127.0.0.1:11211/_stats",
		"ftp://internal/types.xsd",
	} {
		t.Run(location, func(t *testing.T) {
			doc := importingWSDL(location)
			if _, err := NewParser().ParseFromBytes([]byte(doc), "http://h/svc?wsdl"); err != nil {
				t.Fatalf("parse should not hard-fail on a rejected import: %v", err)
			}
		})
	}
}

// An oversized document must not be able to exhaust worker memory.
func TestResponseSizeIsCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?><definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t"><documentation>`))
		chunk := strings.Repeat("A", 64*1024)
		for i := 0; i < 400; i++ { // ~25 MB
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
		w.Write([]byte(`</documentation></definitions>`))
	}))
	defer srv.Close()

	parser := NewParser().WithMaxDocumentSize(1 << 20) // 1 MB
	if _, err := parser.ParseFromURL(srv.URL + "/svc?wsdl"); err == nil {
		t.Error("expected an error when the document exceeds the size cap")
	}
}

// A malicious WSDL must not be able to stall a scan worker indefinitely.
func TestParseRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := NewParser().WithContext(ctx).ParseFromURL(srv.URL + "/svc?wsdl")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error once the context is cancelled")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("parse ignored context cancellation")
	}
}

// Bounded traversal: a document graph that fans out must not fetch without limit.
func TestTotalDocumentCountIsBounded(t *testing.T) {
	var mu sync.Mutex
	fetched := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetched++
		n := fetched
		mu.Unlock()

		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <import namespace="urn:a" location="%s/a%d.wsdl"/>
  <import namespace="urn:b" location="%s/b%d.wsdl"/>
</definitions>`, srv.URL, n, srv.URL, n)
	}))
	defer srv.Close()

	parser := NewParser().WithMaxDocuments(25)
	done := make(chan struct{})
	go func() {
		parser.ParseFromURL(srv.URL + "/root.wsdl")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("unbounded import fan-out did not terminate")
	}

	mu.Lock()
	defer mu.Unlock()
	if fetched > 25 {
		t.Errorf("fetched %d documents, want at most the configured cap of 25", fetched)
	}
}

func TestMaxDepthIsEnforced(t *testing.T) {
	var mu sync.Mutex
	depthSeen := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		depthSeen++
		n := depthSeen
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <import namespace="urn:next" location="%s/level%d.wsdl"/>
</definitions>`, srv.URL, n)
	}))
	defer srv.Close()

	parser := NewParser().WithMaxDepth(3)
	done := make(chan struct{})
	go func() {
		parser.ParseFromURL(srv.URL + "/level0.wsdl")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deep import chain did not terminate")
	}

	mu.Lock()
	defer mu.Unlock()
	if depthSeen > 5 {
		t.Errorf("followed %d levels with maxDepth=3", depthSeen)
	}
}

// The XML decoder must not resolve entities; a WSDL is untrusted input.
func TestEntityExpansionIsNotPerformed(t *testing.T) {
	const bomb = `<?xml version="1.0"?>
<!DOCTYPE definitions [
  <!ENTITY a "aaaaaaaaaa">
  <!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">
  <!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">
  <!ENTITY d "&c;&c;&c;&c;&c;&c;&c;&c;&c;&c;">
]>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <documentation>&d;</documentation>
</definitions>`

	done := make(chan struct{})
	go func() {
		doc, err := NewParser().ParseFromBytes([]byte(bomb), "http://h/s?wsdl")
		if err == nil && doc != nil && len(doc.Documentation) > 100000 {
			t.Error("entity expansion produced an oversized document")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("entity expansion did not terminate promptly")
	}
}

func TestExternalEntityIsNotFetched(t *testing.T) {
	var mu sync.Mutex
	hits := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		fmt.Fprint(w, "SECRET")
	}))
	defer srv.Close()

	xxe := fmt.Sprintf(`<?xml version="1.0"?>
<!DOCTYPE definitions [<!ENTITY xxe SYSTEM "%s/secret">]>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" targetNamespace="urn:t">
  <documentation>&xxe;</documentation>
</definitions>`, srv.URL)

	NewParser().ParseFromBytes([]byte(xxe), "http://h/s?wsdl")

	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Errorf("parser resolved an external entity (%d fetches) — XXE in the scanner itself", hits)
	}
}

// Workers parse many definitions concurrently; a shared parser must not race.
func TestConcurrentParsesAreIsolated(t *testing.T) {
	data := loadFixture(t, "dotnet_asmx.wsdl")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doc, err := NewParser().ParseFromBytes(data, "http://h/svc?wsdl")
			if err != nil {
				t.Errorf("concurrent parse failed: %v", err)
				return
			}
			if len(doc.Services) != 1 {
				t.Errorf("concurrent parse produced %d services, want 1", len(doc.Services))
			}
		}()
	}
	wg.Wait()
}

// Reusing one parser across documents must not let the first parse's import
// bookkeeping suppress the second parse's imports.
func TestParserReuseDoesNotSuppressImports(t *testing.T) {
	var mu sync.Mutex
	importHits := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(r.URL.Path, "imported") {
			mu.Lock()
			importHits++
			mu.Unlock()
			fmt.Fprint(w, emptyWSDL())
			return
		}
		fmt.Fprint(w, importingWSDL(srv.URL+"/imported.wsdl"))
	}))
	defer srv.Close()

	parser := NewParser()
	for i := 0; i < 2; i++ {
		if _, err := parser.ParseFromURL(srv.URL + "/svc?wsdl"); err != nil {
			t.Fatalf("parse %d: %v", i, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if importHits < 2 {
		t.Errorf("import fetched %d times across 2 independent parses, want 2", importHits)
	}
}

// Callers with no context of their own must still get a bounded parse, or a
// target that stalls every import pins a scan worker open indefinitely.
func TestParseIsBoundedWithoutCallerContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	parser := NewParser().WithMaxTotalDuration(300 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := parser.ParseFromURL(srv.URL + "/svc?wsdl")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error once the total parse deadline elapses")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("parse was not bounded by the default total duration")
	}
}

// A caller with only bytes and no source URL has no origin to compare against.
// Failing open there would silently restore both SSRF and credential replay.
func TestEmptySourceURLRefusesImportsAndCredentials(t *testing.T) {
	var mu sync.Mutex
	contacted := false

	third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		contacted = true
		mu.Unlock()
		fmt.Fprint(w, emptyWSDL())
	}))
	defer third.Close()

	parser := NewParser().WithHeaders(map[string]string{"Authorization": "Bearer scan-token"})
	if _, err := parser.ParseFromBytes([]byte(importingWSDL(third.URL+"/x.wsdl")), ""); err != nil {
		t.Fatalf("parse should degrade gracefully: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if contacted {
		t.Error("import was followed despite the document having no origin")
	}
}
