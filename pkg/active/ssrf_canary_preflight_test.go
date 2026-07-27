package active

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The canary oracle can only fire if the canary endpoint actually serves the
// marker. Shipping a canary_url that does not serve it leaves the detector
// running-but-never-firing: it spends a request on every url-like insertion
// point and reports nothing, which is indistinguishable from "target is not
// vulnerable". The preflight makes that state observable and dormant instead.
func TestCanaryEndpointServesMarker(t *testing.T) {
	const marker = "SUKYAN_SSRF_CANARY"

	serving := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := lastPathSegment(r.URL.Path)
		_, _ = w.Write([]byte(marker + ":" + token + ":OK"))
	}))
	defer serving.Close()

	wrongBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>404 not found</html>"))
	}))
	defer wrongBody.Close()

	markerNoToken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(marker))
	}))
	defer markerNoToken.Close()

	unreachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := unreachable.URL
	unreachable.Close()

	tests := []struct {
		name    string
		baseURL string
		marker  string
		want    bool
	}{
		{"endpoint serves marker and token", serving.URL, marker, true},
		{"endpoint serves an unrelated page", wrongBody.URL, marker, false},
		{"endpoint serves marker but not the token", markerNoToken.URL, marker, false},
		{"endpoint is unreachable", unreachableURL, marker, false},
		{"canary url not configured", "", marker, false},
		{"marker not configured", serving.URL, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canaryEndpointServesMarker(context.Background(), http.DefaultClient, tt.baseURL, tt.marker)
			if got != tt.want {
				t.Errorf("canaryEndpointServesMarker(%q, %q) = %v, want %v", tt.baseURL, tt.marker, got, tt.want)
			}
		})
	}
}

// The shipped default must not put the detector into the running-but-futile
// state: an operator who has not hosted a canary should get dormancy, which is
// what the config documents.
func TestDefaultCanaryURLIsUnset(t *testing.T) {
	if defaultCanaryURL != "" {
		t.Errorf("default canary_url is %q; it must be empty so detection is dormant until an operator hosts a canary. "+
			"A non-empty default that does not serve the marker makes the detector probe every url-like insertion point and never fire.", defaultCanaryURL)
	}
}
