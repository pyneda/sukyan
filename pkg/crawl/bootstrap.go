package crawl

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/rs/zerolog/log"
)

const (
	bootstrapRequestTimeout = 15 * time.Second
	maxSitemapDocuments     = 20
	maxSitemapIndexDepth    = 3
	// maxBootstrapLocations bounds what one origin's map can contribute to the
	// frontier, using the sitemaps.org per-document limit as the per-origin
	// budget. An index of large documents would otherwise be unbounded.
	maxBootstrapLocations = 50000
)

// fallbackSitemapPaths are tried only when robots.txt names no sitemap and
// /sitemap.xml does not parse as one. The authoritative answer to "where is the
// sitemap" is the robots.txt Sitemap directive, so this list stays short: it
// covers the few conventional names, and anything beyond that is a wordlist
// problem that belongs to the discovery phase.
var fallbackSitemapPaths = []string{
	"/sitemap_index.xml",
	"/sitemap-index.xml",
	"/sitemap.xml.gz",
}

// bootstrapOutcome collects what reading an origin's machine-readable map
// produced: locations to crawl, and the responses themselves so they reach
// passive analysis like any other crawled response.
type bootstrapOutcome struct {
	Locations []string
	Histories []*db.History
	Fetched   []string
}

func (o *bootstrapOutcome) record(history *db.History, target string) {
	o.Fetched = append(o.Fetched, target)
	if history != nil {
		o.Histories = append(o.Histories, history)
	}
}

// bootstrapFetcher retrieves a bootstrap document. The flag reports whether the
// response is worth parsing; the history is returned either way so failures are
// still recorded. It is a parameter so the traversal's bounds can be exercised
// without a browser, a network or a database.
type bootstrapFetcher func(ctx context.Context, target string) (*db.History, []byte, bool)

// bootstrapOrigin reads the files an origin publishes to describe itself:
// robots.txt, the sitemaps it names, and the universal link declaration.
//
// This runs as its own step rather than as crawlable seed paths because these
// documents are not pages: they need no browser, and a scan that turned seed
// paths off would otherwise lose the site's own map of itself.
func (c *Crawler) bootstrapOrigin(ctx context.Context, origin string, fetch bootstrapFetcher) bootstrapOutcome {
	outcome := bootstrapOutcome{}
	base := strings.TrimSuffix(origin, "/")

	sitemaps := c.readRobotsTxt(ctx, base, fetch, &outcome)

	if len(sitemaps) == 0 {
		sitemaps = []string{base + "/sitemap.xml"}
	}
	// Guessing is the last resort: robots.txt is where a site declares a sitemap
	// under a non-standard name, so the fallbacks only run when it declared none
	// and the conventional location held no sitemap either.
	if !c.walkSitemaps(ctx, sitemaps, fetch, &outcome) {
		for _, path := range fallbackSitemapPaths {
			if c.walkSitemaps(ctx, []string{base + path}, fetch, &outcome) {
				break
			}
		}
	}

	c.readAppSiteAssociation(ctx, base, fetch, &outcome)

	return outcome
}

func (c *Crawler) readRobotsTxt(ctx context.Context, base string, fetch bootstrapFetcher, outcome *bootstrapOutcome) []string {
	target := base + "/robots.txt"
	history, body, ok := fetch(ctx, target)
	outcome.record(history, target)
	if !ok {
		return nil
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return nil
	}

	robots := passive.ParseRobotsTxt(string(body), parsed)
	outcome.Locations = append(outcome.Locations, robots.Paths...)
	return robots.Sitemaps
}

// walkSitemaps reads the given documents and any sitemap index they point at,
// bounded in both breadth and depth because an index may reference itself or
// fan out further than a scan can afford. It reports whether any of them parsed
// as a sitemap, which is what tells the caller a guess was not needed.
func (c *Crawler) walkSitemaps(ctx context.Context, roots []string, fetch bootstrapFetcher, outcome *bootstrapOutcome) bool {
	type queued struct {
		url   string
		depth int
	}

	queue := make([]queued, 0, len(roots))
	for _, root := range roots {
		queue = append(queue, queued{url: root})
	}

	seen := make(map[string]bool, len(roots))
	documents := 0
	parsedAny := false

	for len(queue) > 0 && documents < maxSitemapDocuments {
		select {
		case <-ctx.Done():
			return parsedAny
		default:
		}

		current := queue[0]
		queue = queue[1:]

		if seen[current.url] || !c.scope.IsInScope(current.url) {
			continue
		}
		seen[current.url] = true

		history, body, ok := fetch(ctx, current.url)
		outcome.record(history, current.url)
		documents++
		if !ok {
			continue
		}

		parsed, err := url.Parse(current.url)
		if err != nil {
			continue
		}
		sitemap, isSitemap := passive.ParseSitemap(body, parsed)
		if !isSitemap {
			continue
		}
		parsedAny = true
		outcome.Locations = append(outcome.Locations, sitemap.URLs...)
		if len(outcome.Locations) >= maxBootstrapLocations {
			log.Warn().Str("url", current.url).Int("locations", len(outcome.Locations)).Int("queued_documents", len(queue)).Msg("Stopping sitemap traversal at the per-origin location budget")
			return true
		}

		if current.depth < maxSitemapIndexDepth {
			for _, child := range sitemap.Sitemaps {
				queue = append(queue, queued{url: child, depth: current.depth + 1})
			}
		}
	}

	return parsedAny
}

func (c *Crawler) readAppSiteAssociation(ctx context.Context, base string, fetch bootstrapFetcher, outcome *bootstrapOutcome) {
	target := base + "/.well-known/apple-app-site-association"
	history, body, ok := fetch(ctx, target)
	outcome.record(history, target)
	if !ok {
		return
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return
	}
	outcome.Locations = append(outcome.Locations, passive.ParseAppSiteAssociation(body, parsed)...)
}

// scheduleBootstrapLocations feeds the discovered locations into the frontier.
//
// Locations are dispatched by a fixed set of workers rather than a goroutine per
// URL: a single sitemap may declare tens of thousands of locations, and parking
// that many goroutines on the concurrency semaphore costs more memory than the
// crawl itself. Depth and scope are applied here as well as in shouldCrawl so the
// two counts can be reported - a sitemap silently losing most of its URLs to a
// depth limit reads as coverage the scan never had.
func (c *Crawler) scheduleBootstrapLocations(origin string, locations []string) {
	seen := make(map[string]bool, len(locations))
	items := make([]*CrawlItem, 0, len(locations))
	skippedDepth, skippedScope := 0, 0

	for _, location := range locations {
		if seen[location] {
			continue
		}
		seen[location] = true

		if !c.scope.IsInScope(location) {
			skippedScope++
			continue
		}
		depth := lib.CalculateURLDepth(location)
		if c.config.MaxDepth != 0 && depth > c.config.MaxDepth {
			skippedDepth++
			continue
		}
		items = append(items, &CrawlItem{url: location, depth: depth})
	}

	log.Info().
		Str("origin", origin).
		Int("declared", len(locations)).
		Int("scheduled", len(items)).
		Int("skipped_max_depth", skippedDepth).
		Int("skipped_out_of_scope", skippedScope).
		Int("max_depth", c.config.MaxDepth).
		Msg("Scheduling locations declared by the origin's robots.txt and sitemaps")

	if len(items) == 0 {
		return
	}

	workers := max(cap(c.concLimit), 1)
	queue := make(chan *CrawlItem)
	for range workers {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for item := range queue {
				c.wg.Add(1)
				c.crawlPage(item)
			}
		}()
	}

	go func() {
		defer close(queue)
		for _, item := range items {
			select {
			case queue <- item:
			case <-c.ctx.Done():
				return
			}
		}
	}()
}

// bootstrapFetch performs a plain HTTP GET, deliberately skipping the browser:
// these documents are never rendered, so a page from the pool would only cost
// the navigation and stability waits a text file cannot benefit from. The
// returned flag reports whether the response is worth parsing; the history is
// returned either way so failures are still recorded.
func (c *Crawler) bootstrapFetch(ctx context.Context, target string) (*db.History, []byte, bool) {
	for _, pattern := range c.excludePatterns {
		if strings.Contains(target, pattern) {
			log.Debug().Str("url", target).Str("pattern", pattern).Msg("Skipping bootstrap fetch because it matches an exclude pattern")
			return nil, nil, false
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, false
	}
	if c.crawlOptions.UserAgent != "" {
		request.Header.Set("User-Agent", c.crawlOptions.UserAgent)
	}
	for name, values := range c.config.ExtraHeaders {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	result := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        c.config.HTTPClient,
		Timeout:       bootstrapRequestTimeout,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceCrawler,
			WorkspaceID: c.workspaceID,
			TaskID:      c.taskID,
			ScanID:      c.scanID,
			ScanJobID:   c.scanJobID,
		},
	})
	if result.Err != nil || result.History == nil {
		log.Debug().Err(result.Err).Str("url", target).Msg("Bootstrap fetch failed")
		return nil, nil, false
	}
	if result.History.StatusCode != http.StatusOK {
		return result.History, nil, false
	}

	body, err := result.History.ResponseBody()
	if err != nil {
		return result.History, nil, false
	}
	return result.History, body, true
}
