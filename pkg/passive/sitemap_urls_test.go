package passive

import (
	"bytes"
	"compress/gzip"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func mustParseBase(t *testing.T, raw string) *url.URL {
	t.Helper()
	base, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing base %q: %v", raw, err)
	}
	return base
}

func TestParseSitemapURLSet(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/products?cat=1&amp;page=2</loc><lastmod>2024-01-01</lastmod></url>
  <url><loc>https://example.com/about</loc><changefreq>daily</changefreq></url>
</urlset>`

	sitemap, ok := ParseSitemap([]byte(body), base)
	if !ok {
		t.Fatal("expected body to be recognised as a sitemap")
	}
	want := []string{"https://example.com/products?cat=1&page=2", "https://example.com/about"}
	if !reflect.DeepEqual(sitemap.URLs, want) {
		t.Errorf("URLs = %v, want %v", sitemap.URLs, want)
	}
	if len(sitemap.Sitemaps) != 0 {
		t.Errorf("Sitemaps = %v, want none", sitemap.Sitemaps)
	}
}

// The escaped ampersand is the whole reason this parser exists: read as text the
// location keeps its &amp;, which requests a page that does not exist under a
// parameter named "amp;page" while the real one is never crawled.
func TestParseSitemapDecodesEntitiesTheTextExtractorCannot(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml")
	body := `<urlset><url><loc>https://example.com/p?a=1&amp;b=2</loc></url></urlset>`

	sitemap, ok := ParseSitemap([]byte(body), base)
	if !ok {
		t.Fatal("expected body to be recognised as a sitemap")
	}
	if len(sitemap.URLs) != 1 || sitemap.URLs[0] != "https://example.com/p?a=1&b=2" {
		t.Fatalf("URLs = %v, want the decoded location", sitemap.URLs)
	}
	for _, got := range ExtractAndAnalyzeURLS(body, "https://example.com/sitemap.xml").Web {
		if strings.Contains(got, "&amp;") {
			return
		}
	}
	t.Error("expected the generic extractor to still mangle the location, guarding the reason for this parser")
}

func TestParseSitemapIndex(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/sitemaps/products-1.xml.gz</loc></sitemap>
  <sitemap><loc>https://example.com/sitemaps/pages.xml</loc></sitemap>
</sitemapindex>`

	sitemap, ok := ParseSitemap([]byte(body), base)
	if !ok {
		t.Fatal("expected body to be recognised as a sitemap index")
	}
	want := []string{"https://example.com/sitemaps/products-1.xml.gz", "https://example.com/sitemaps/pages.xml"}
	if !reflect.DeepEqual(sitemap.Sitemaps, want) {
		t.Errorf("Sitemaps = %v, want %v", sitemap.Sitemaps, want)
	}
	if len(sitemap.URLs) != 0 {
		t.Errorf("URLs = %v, want none", sitemap.URLs)
	}
}

func TestParseSitemapGzipped(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml.gz")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`<urlset><url><loc>https://example.com/gz</loc></url></urlset>`)); err != nil {
		t.Fatalf("writing gzip body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}

	sitemap, ok := ParseSitemap(compressed.Bytes(), base)
	if !ok {
		t.Fatal("expected gzipped body to be recognised as a sitemap")
	}
	if len(sitemap.URLs) != 1 || sitemap.URLs[0] != "https://example.com/gz" {
		t.Errorf("URLs = %v, want the single decompressed location", sitemap.URLs)
	}
}

func TestParseSitemapRejectsOtherDocuments(t *testing.T) {
	base := mustParseBase(t, "https://example.com/")
	bodies := map[string]string{
		"html":       `<html><body><a href="/a">a</a></body></html>`,
		"json":       `{"urls": ["https://example.com/a"]}`,
		"rss":        `<rss version="2.0"><channel><item><link>https://example.com/a</link></item></channel></rss>`,
		"empty":      ``,
		"plain text": `https://example.com/a`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			if _, ok := ParseSitemap([]byte(body), base); ok {
				t.Errorf("ParseSitemap(%q) accepted a non-sitemap document", name)
			}
		})
	}
}

func TestParseSitemapKeepsLocationsFromTruncatedDocument(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml")
	body := `<urlset><url><loc>https://example.com/a</loc></url><url><loc>https://example.com/b</lo`

	sitemap, ok := ParseSitemap([]byte(body), base)
	if !ok {
		t.Fatal("expected truncated sitemap to still be recognised")
	}
	if len(sitemap.URLs) != 1 || sitemap.URLs[0] != "https://example.com/a" {
		t.Errorf("URLs = %v, want the locations read before the truncation", sitemap.URLs)
	}
}

func TestParseSitemapIgnoresNamespaceAndMetadata(t *testing.T) {
	base := mustParseBase(t, "https://example.com/sitemap.xml")
	body := `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">
  <url><loc>https://example.com/a</loc><priority>0.8</priority></url>
</urlset>`

	sitemap, ok := ParseSitemap([]byte(body), base)
	if !ok {
		t.Fatal("expected body to be recognised as a sitemap")
	}
	if !reflect.DeepEqual(sitemap.Locations(), []string{"https://example.com/a"}) {
		t.Errorf("Locations() = %v, want only the declared location", sitemap.Locations())
	}
}
