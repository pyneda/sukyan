package http_utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithoutRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("target"))
	}))
	defer server.Close()

	t.Run("preserves 3xx and Location header", func(t *testing.T) {
		client := WithoutRedirects(CreateHttpClient())
		resp, err := client.Get(server.URL + "/redirect")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMovedPermanently {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMovedPermanently)
		}
		if loc := resp.Header.Get("Location"); loc != "/target" {
			t.Errorf("Location = %q, want %q", loc, "/target")
		}
	})

	t.Run("default client follows the redirect", func(t *testing.T) {
		resp, err := CreateHttpClient().Get(server.URL + "/redirect")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d (redirect should have been followed)", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("nil client yields a working non-redirecting client", func(t *testing.T) {
		client := WithoutRedirects(nil)
		resp, err := client.Get(server.URL + "/redirect")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMovedPermanently {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMovedPermanently)
		}
	})

	t.Run("does not mutate the source client", func(t *testing.T) {
		source := CreateHttpClient()
		_ = WithoutRedirects(source)
		if source.CheckRedirect != nil {
			t.Error("source client CheckRedirect was mutated")
		}
	})
}
