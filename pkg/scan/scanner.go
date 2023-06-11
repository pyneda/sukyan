package scan

import (
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"regexp"
	"time"

	"github.com/rs/zerolog/log"
)

// Scanner is used to schedule web scans
type Scanner struct {
	ShouldCrawl     bool
	Scope           scope.Scope
	StartUrls       []string
	ActiveTests     []string
	Depth           int
	MaxPagesToCrawl int
	PagesPoolSize   int
	DiscoverParams  bool
	AuditHeaders    bool
	PageTimeout     time.Duration
	CrawlTimeout    time.Duration
	ExtraHeaders    map[string]string
	Include         *regexp.Regexp
	Exclude         *regexp.Regexp
	// https://chromedevtools.github.io/devtools-protocol/tot/Network/#method-setBlockedURLs
	// URLs which are blocked from loading, could block by default google analytics and such things.
	BlockedURLs []string
	// https://chromedevtools.github.io/devtools-protocol/tot/Network/#method-setBypassServiceWorker
	// Should bypass service worker?
	BypassServiceWorker bool
	InteractionsManager *integrations.InteractionsManager
}

// Run schedules the scan
func (s *Scanner) Run() {
	s.InteractionsManager = &integrations.InteractionsManager{
		GetAsnInfo:            false,
		PollingInterval:       10 * time.Second,
		OnInteractionCallback: SaveInteractionCallback,
	}
	s.InteractionsManager.Start()
	pagesToScan := []web.WebPage{}
	if s.ShouldCrawl == true {
		pagesToScan = s.Crawl()
	} else {
		pagesToScan = web.InspectMultipleURLs(s.StartUrls)
	}
	for _, pageToScan := range pagesToScan {
		s.ScanURL(pageToScan)
	}
	s.InteractionsManager.Stop()
}

// InitialChecks performs basic initial checks against scoped sites
func (s *Scanner) InitialChecks() {
	// For each evaluated site should check for things such as:
	// - Content differs between devices / user-agents, etc?
	// - Look more common files such as robots.txt, sitemap.xml, crossdomain.xml, etc
	// - Check if Ajax is used

}

// Crawl crawls the scoped sites
func (s *Scanner) Crawl() []web.WebPage {

	c := crawl.Crawler{
		Scope:         s.Scope,
		StartUrls:     s.StartUrls,
		PagesPoolSize: s.PagesPoolSize,
		PageTimeout:   s.PageTimeout,
	}
	log.Info().Interface("config", c).Msg("Starting crawl")
	return c.Run()
}

// ScanURL performs different checks to a found url
func (s *Scanner) ScanURL(webPage web.WebPage) {
	// TODO: Should get the parameters to test from user
	var specificParamsToTest []string
	// params = append(params, "url")
	hasParams, _ := webPage.HasParameters()
	log.Info().Interface("webPage", webPage).Msg("Scanning URL")
	if hasParams {
		ssrf := active.SSRFAudit{
			URL:                 webPage.URL,
			ParamsToTest:        specificParamsToTest,
			Concurrency:         5,
			StopAfterSuccess:    false,
			InteractionsManager: s.InteractionsManager,
		}
		ssrf.Run()
		active.TestXSS(webPage.URL, specificParamsToTest, "default.txt", false)
		// ssti := active.SSTIAudit{
		// 	URL:              webPage.URL,
		// 	Params:           specificParamsToTest,
		// 	Concurrency:      5,
		// 	StopAfterSuccess: false,
		// }
		// ssti.Run()
		// pathTraversal := active.PathTraversalAudit{
		// 	URL:              webPage.URL,
		// 	Params:           specificParamsToTest,
		// 	Concurrency:      5,
		// 	PayloadsDepth:    5,
		// 	Platform:         "all",
		// 	StopAfterSuccess: false,
		// }
		// pathTraversal.Run()
	}
	if s.AuditHeaders {
		hostHeader := active.HostHeaderInjectionAudit{
			URL:         webPage.URL,
			Concurrency: 10,
		}
		hostHeader.Run()
	}
}
