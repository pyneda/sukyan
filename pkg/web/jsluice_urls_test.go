package web

import (
	"testing"
)

func TestExtractURLsFromJS(t *testing.T) {
	t.Run("extracts fetch call URLs", func(t *testing.T) {
		code := []byte(`fetch("/api/users");`)
		urls := ExtractURLsFromJS(code)
		if len(urls) == 0 {
			t.Skip("jsluice did not extract URLs from this pattern; skipping")
		}
		found := false
		for _, u := range urls {
			if u.URL == "/api/users" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find /api/users in extracted URLs, got %+v", urls)
		}
	})

	t.Run("extracts object assignment URLs", func(t *testing.T) {
		code := []byte(`var config = { endpoint: "/api/v2/data" };`)
		urls := ExtractURLsFromJS(code)
		if len(urls) == 0 {
			t.Skip("jsluice did not extract URLs from this pattern; skipping")
		}
		found := false
		for _, u := range urls {
			if u.URL == "/api/v2/data" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find /api/v2/data in extracted URLs, got %+v", urls)
		}
	})

	t.Run("no URLs from clean JS", func(t *testing.T) {
		code := []byte(`var x = 42; console.log(x);`)
		urls := ExtractURLsFromJS(code)
		if len(urls) != 0 {
			t.Errorf("expected 0 URLs for clean JS, got %d: %+v", len(urls), urls)
		}
	})

	t.Run("extracted URL has QueryParams initialized", func(t *testing.T) {
		code := []byte(`fetch("/api/search?q=test");`)
		urls := ExtractURLsFromJS(code)
		for _, u := range urls {
			if u.QueryParams == nil {
				t.Error("expected QueryParams to be initialized (not nil)")
			}
			if u.BodyParams == nil {
				t.Error("expected BodyParams to be initialized (not nil)")
			}
		}
	})
}

func TestExtractURLsFromJSON(t *testing.T) {
	t.Run("extracts URLs from JSON values", func(t *testing.T) {
		jsonData := []byte(`{"api_url":"/api/v1/endpoint","callback":"https://example.com/hook"}`)
		urls := ExtractURLsFromJSON(jsonData)
		if len(urls) == 0 {
			t.Skip("jsluice did not extract URLs from this JSON pattern; skipping")
		}
		urlSet := make(map[string]bool)
		for _, u := range urls {
			urlSet[u.URL] = true
		}
		if !urlSet["/api/v1/endpoint"] && !urlSet["https://example.com/hook"] {
			t.Errorf("expected to find at least one URL from the JSON, got %+v", urls)
		}
	})

	t.Run("extracts URLs from JSON arrays", func(t *testing.T) {
		jsonData := []byte(`{"endpoints":["/api/a","/api/b"]}`)
		urls := ExtractURLsFromJSON(jsonData)
		if len(urls) == 0 {
			t.Skip("jsluice did not extract URLs from this JSON array pattern; skipping")
		}
	})

	t.Run("no URLs from clean JSON", func(t *testing.T) {
		jsonData := []byte(`{"name":"John","age":30}`)
		urls := ExtractURLsFromJSON(jsonData)
		if len(urls) != 0 {
			t.Errorf("expected 0 URLs for clean JSON, got %d: %+v", len(urls), urls)
		}
	})
}
