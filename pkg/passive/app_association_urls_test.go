package passive

import (
	"reflect"
	"sort"
	"testing"
)

func TestIsAppSiteAssociationURL(t *testing.T) {
	tests := map[string]bool{
		"https://example.com/.well-known/apple-app-site-association":  true,
		"https://example.com/apple-app-site-association":              true,
		"https://example.com/.well-known/apple-app-site-association/": true,
		"https://example.com/.well-known/assetlinks.json":             false,
		"https://example.com/app/apple-app-site-association":          false,
		"https://example.com/":                                        false,
	}
	for rawURL, want := range tests {
		if got := isAppSiteAssociationURL(rawURL); got != want {
			t.Errorf("isAppSiteAssociationURL(%q) = %v, want %v", rawURL, got, want)
		}
	}
}

func TestExtractURLsFromAppSiteAssociation(t *testing.T) {
	base := mustParseBase(t, "https://example.com/.well-known/apple-app-site-association")
	body := `{
  "applinks": {
    "apps": [],
    "details": [
      {
        "appID": "ABC123.com.example.app",
        "paths": ["/user/*", "/order/*", "NOT /admin/*", "/checkout", "/report?"],
        "components": [
          {"/": "/promo/*", "comment": "promos"},
          {"/": "/secret/*", "exclude": true}
        ]
      }
    ]
  },
  "webcredentials": {"apps": ["ABC123.com.example.app"]}
}`

	got := extractURLsFromAppSiteAssociation([]byte(body), base).Web
	sort.Strings(got)
	want := []string{
		"https://example.com/checkout",
		"https://example.com/order/",
		"https://example.com/promo/",
		"https://example.com/report",
		"https://example.com/user/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("web URLs = %v, want %v", got, want)
	}
}

func TestExtractURLsFromAppSiteAssociationLegacyDetailsObject(t *testing.T) {
	base := mustParseBase(t, "https://example.com/apple-app-site-association")
	body := `{"applinks": {"details": {"ABC123.com.example.app": {"paths": ["/legacy/*"]}}}}`

	got := extractURLsFromAppSiteAssociation([]byte(body), base).Web
	if !reflect.DeepEqual(got, []string{"https://example.com/legacy/"}) {
		t.Errorf("web URLs = %v, want the legacy object's path", got)
	}
}

func TestExtractURLsFromAppSiteAssociationIgnoresUnrelatedDocuments(t *testing.T) {
	base := mustParseBase(t, "https://example.com/.well-known/apple-app-site-association")
	bodies := []string{
		`{"webcredentials": {"apps": ["ABC123.com.example.app"]}}`,
		`{"applinks": {"apps": [], "details": []}}`,
		`not json`,
	}
	for _, body := range bodies {
		if got := extractURLsFromAppSiteAssociation([]byte(body), base).Web; len(got) != 0 {
			t.Errorf("extractURLsFromAppSiteAssociation(%q) = %v, want none", body, got)
		}
	}
}

// The generic extractor turns these patterns into literal "/user/*" requests,
// which 404 while losing the directory the pattern actually describes.
func TestAppSiteAssociationReplacesGenericExtraction(t *testing.T) {
	body := []byte(`{"applinks":{"details":[{"appID":"A","paths":["/user/*"]}]}}`)
	got := extractURLsFromBody(body, "https://example.com/.well-known/apple-app-site-association")
	if !reflect.DeepEqual(got.Web, []string{"https://example.com/user/"}) {
		t.Errorf("web URLs = %v, want only the trimmed prefix", got.Web)
	}
}
