package crawl

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

// stubOrigin serves bootstrap documents from a map, recording what was asked for
// so the tests can assert on the requests the crawler chose to make.
type stubOrigin struct {
	documents map[string]string
	requested []string
}

func (s *stubOrigin) fetch(ctx context.Context, target string) (*db.History, []byte, bool) {
	s.requested = append(s.requested, target)
	body, found := s.documents[target]
	if !found {
		return &db.History{URL: target, StatusCode: 404}, nil, false
	}
	return &db.History{URL: target, StatusCode: 200}, []byte(body), true
}

func newBootstrapCrawler(t *testing.T, origin string) *Crawler {
	t.Helper()
	c := &Crawler{}
	c.scope.CreateScopeItemsFromUrls([]string{origin}, "www")
	return c
}

func urlset(locations ...string) string {
	var builder strings.Builder
	builder.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, location := range locations {
		builder.WriteString("<url><loc>" + location + "</loc></url>")
	}
	builder.WriteString(`</urlset>`)
	return builder.String()
}

func sitemapindex(locations ...string) string {
	var builder strings.Builder
	builder.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, location := range locations {
		builder.WriteString("<sitemap><loc>" + location + "</loc></sitemap>")
	}
	builder.WriteString(`</sitemapindex>`)
	return builder.String()
}

func TestBootstrapOriginReadsRobotsDeclaredSitemap(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		origin + "/robots.txt":         "User-agent: *\nDisallow: /admin/*\nSitemap: https://example.com/custom-sitemap.xml\n",
		origin + "/custom-sitemap.xml": urlset("https://example.com/a", "https://example.com/b?x=1&amp;y=2"),
		origin + "/sitemap.xml":        urlset("https://example.com/should-not-be-read"),
		origin + "/sitemap_index.xml":  urlset("https://example.com/should-not-be-read"),
	}}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	want := []string{
		"https://example.com/admin/",
		"https://example.com/a",
		"https://example.com/b?x=1&y=2",
	}
	if !reflect.DeepEqual(outcome.Locations, want) {
		t.Errorf("Locations = %v, want %v", outcome.Locations, want)
	}
	for _, requested := range stub.requested {
		if strings.Contains(requested, "sitemap_index.xml") {
			t.Errorf("fallback names were guessed even though robots.txt declared a sitemap: %v", stub.requested)
		}
	}
}

func TestBootstrapOriginFallsBackOnlyWhenNothingParsed(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		// A single-page app answering every path with its shell is the reason the
		// fallbacks are validated by content and not by status code.
		origin + "/sitemap.xml":       `<html><body>Not found</body></html>`,
		origin + "/sitemap_index.xml": urlset("https://example.com/from-fallback"),
	}}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	if !reflect.DeepEqual(outcome.Locations, []string{"https://example.com/from-fallback"}) {
		t.Errorf("Locations = %v, want the fallback sitemap's location", outcome.Locations)
	}
	for _, requested := range stub.requested {
		if strings.Contains(requested, "sitemap-index.xml") || strings.Contains(requested, "sitemap.xml.gz") {
			t.Errorf("kept guessing after a fallback parsed: %v", stub.requested)
		}
	}
}

func TestBootstrapOriginFollowsSitemapIndex(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		origin + "/sitemap.xml":      sitemapindex(origin+"/sitemaps/one.xml", origin+"/sitemaps/two.xml"),
		origin + "/sitemaps/one.xml": urlset("https://example.com/one"),
		origin + "/sitemaps/two.xml": urlset("https://example.com/two"),
	}}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	got := append([]string{}, outcome.Locations...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"https://example.com/one", "https://example.com/two"}) {
		t.Errorf("Locations = %v, want both children's locations", got)
	}
}

// A sitemap index that points back at itself, directly or through a chain, is a
// hang the target controls. Bounding breadth and depth is what prevents it.
func TestBootstrapOriginBoundsSelfReferencingIndex(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		origin + "/sitemap.xml": sitemapindex(origin + "/sitemap-child.xml"),
	}}
	for i := range maxSitemapDocuments + 10 {
		stub.documents[fmt.Sprintf("%s/sitemap-child.xml", origin)] = sitemapindex(
			fmt.Sprintf("%s/sitemap-child-%d.xml", origin, i),
		)
	}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	sitemapRequests := 0
	for _, requested := range stub.requested {
		if strings.Contains(requested, "sitemap") {
			sitemapRequests++
		}
	}
	if sitemapRequests > maxSitemapDocuments {
		t.Errorf("fetched %d sitemap documents, want at most %d", sitemapRequests, maxSitemapDocuments)
	}
	if len(outcome.Locations) != 0 {
		t.Errorf("Locations = %v, want none from an index chain that declares no content", outcome.Locations)
	}
}

func TestBootstrapOriginSkipsOutOfScopeSitemaps(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		origin + "/robots.txt":  "Sitemap: https://cdn.other-domain.com/sitemap.xml\n",
		origin + "/sitemap.xml": urlset("https://example.com/a"),
	}}

	newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	for _, requested := range stub.requested {
		if strings.Contains(requested, "other-domain.com") {
			t.Errorf("fetched an out-of-scope sitemap: %v", stub.requested)
		}
	}
}

func TestBootstrapOriginReadsAppSiteAssociation(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{
		origin + "/.well-known/apple-app-site-association": `{"applinks":{"details":[{"appID":"A","paths":["/user/*","NOT /admin/*"]}]}}`,
	}}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	if !reflect.DeepEqual(outcome.Locations, []string{"https://example.com/user/"}) {
		t.Errorf("Locations = %v, want the trimmed universal link prefix", outcome.Locations)
	}
}

func TestBootstrapOutcomeRecordsEveryFetchedDocument(t *testing.T) {
	origin := "https://example.com"
	stub := &stubOrigin{documents: map[string]string{}}

	outcome := newBootstrapCrawler(t, origin).bootstrapOrigin(context.Background(), origin, stub.fetch)

	// Recorded even on 404 so the crawler marks them visited and the responses
	// still reach passive analysis.
	if len(outcome.Fetched) != len(stub.requested) {
		t.Errorf("Fetched = %v, want one entry per request %v", outcome.Fetched, stub.requested)
	}
	if len(outcome.Histories) != len(stub.requested) {
		t.Errorf("Histories = %d, want one per request (%d)", len(outcome.Histories), len(stub.requested))
	}
}
