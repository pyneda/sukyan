package reflection

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

// Endpoints that persist what we submit render earlier probes on every later
// response. A canary whose only recognisable part is fixed cannot tell that
// residue apart from a live reflection of the current probe, so insertion points
// that never reflect report "[stripped]" — a reachable-but-filtered signal that
// keeps callers spending payloads on them. The per-request nonce is what makes a
// match attributable to the request that sent it.
func TestCharacterEfficiencyIgnoresResidueFromEarlierProbes(t *testing.T) {
	var stored []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.URL.Query().Get("stored"); v != "" && v != "orig" {
			stored = append(stored, v)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body>%s</body></html>", strings.Join(stored, " "))
	}))
	defer srv.Close()

	newItem := func() *db.History {
		return &db.History{
			URL:    srv.URL + "/?stored=orig&other=orig",
			Method: "GET",
			RawRequest: []byte("GET /?stored=orig&other=orig HTTP/1.1\r\nHost: " +
				srv.Listener.Addr().String() + "\r\n\r\n"),
		}
	}

	reflected := testCharacterEfficiency(
		newItem(),
		InsertionPointInfo{Name: "stored", Type: "parameter", OriginalData: "orig"},
		"<", srv.Client(), http_utils.HistoryCreationOptions{},
	)
	if reflected.Efficiency != 100 {
		t.Fatalf("a sink that echoes the probe verbatim must report efficiency 100, got %d (%s)",
			reflected.Efficiency, reflected.EncodedAs)
	}

	if len(stored) == 0 {
		t.Fatal("expected the endpoint to have stored the first probe; the residue case is not being exercised")
	}

	residueOnly := testCharacterEfficiency(
		newItem(),
		InsertionPointInfo{Name: "other", Type: "parameter", OriginalData: "orig"},
		"<", srv.Client(), http_utils.HistoryCreationOptions{},
	)
	if residueOnly.EncodedAs != "[not reflected]" {
		t.Errorf("insertion point %q is never reflected, but the earlier probe's residue in the body "+
			"made it report %q", "other", residueOnly.EncodedAs)
	}
}

// A filter that drops the tested character and everything after it leaves the
// probe's prefix and nonce in the body but no suffix. The probe demonstrably
// reached the response, so this is a filtered character, not an unreflected
// insertion point — requiring the fixed suffix here would report the opposite
// and hand the decision to whatever residue happened to be on the page.
func TestCharacterEfficiencyReportsStrippedWhenSuffixIsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query().Get("q")
		if i := strings.Index(v, "<"); i >= 0 {
			v = v[:i]
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body>%s</body></html>", v)
	}))
	defer srv.Close()

	item := &db.History{
		URL:        srv.URL + "/?q=orig",
		Method:     "GET",
		RawRequest: []byte("GET /?q=orig HTTP/1.1\r\nHost: " + srv.Listener.Addr().String() + "\r\n\r\n"),
	}

	result := testCharacterEfficiency(
		item,
		InsertionPointInfo{Name: "q", Type: "parameter", OriginalData: "orig"},
		"<", srv.Client(), http_utils.HistoryCreationOptions{},
	)
	if result.EncodedAs != "[stripped]" {
		t.Errorf("the probe reflected but the character was filtered; expected %q, got %q",
			"[stripped]", result.EncodedAs)
	}
}

// Two probes of the same insertion point must not see each other's stored
// output. Without a per-request nonce every character test after the first
// matches the first one's payload.
func TestCanaryNoncesAreUniquePerProbe(t *testing.T) {
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		nonce := NewCanaryNonce()
		if nonce == "" {
			t.Fatal("NewCanaryNonce returned an empty nonce; every probe would share one canary")
		}
		if seen[nonce] {
			t.Fatalf("NewCanaryNonce repeated %q within 64 draws", nonce)
		}
		seen[nonce] = true
	}

	payload := canaryPayload("abcdefgh", "<")
	if !strings.HasPrefix(payload, CanaryPrefix) || !strings.HasSuffix(payload, CanarySuffix) {
		t.Fatalf("canary %q must keep the fixed family markers so scanner traffic stays recognisable", payload)
	}
	if !strings.Contains(payload, "abcdefgh<") {
		t.Fatalf("canary %q must carry the nonce next to the tested character", payload)
	}
}
