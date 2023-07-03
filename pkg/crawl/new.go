package crawl

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"strings"
	"sync"
	"time"
)

type Crawler2 struct {
	scope           scope.Scope
	maxPagesToCrawl int
	maxDepth        int
	startURLs       []string
	excludePatterns []string
	browser         *web.BrowserManager
	pages           sync.Map
	pageCounter     int
	clickedEleemnts sync.Map
	submittedForms  sync.Map
	counterLock     sync.Mutex
	wg              sync.WaitGroup
	concLimit       chan struct{}
	hijackChan      chan web.HijackResult
}

type CrawlItem struct {
	url     string
	depth   int
	visited bool
	isError bool
}

type CrawledPageResut struct {
	URL            string
	DiscoveredURLs []string
	IsError        bool
}

type ClickedElement struct {
	xpath string
	html  string
}

type SubmittedForm struct {
	xpath string
	html  string
}

func NewCrawler(startURLs []string, maxPagesToCrawl int, maxDepth int, poolSize int, excludePatterns []string) *Crawler2 {
	hijackChan := make(chan web.HijackResult)

	browser := web.NewHijackedBrowserManager(
		web.BrowserManagerConfig{
			PoolSize: poolSize,
		},
		"Crawler",
		hijackChan,
	)
	return &Crawler2{
		maxPagesToCrawl: maxPagesToCrawl,
		maxDepth:        maxDepth,
		startURLs:       startURLs,
		excludePatterns: excludePatterns,
		concLimit:       make(chan struct{}, poolSize+2), // Set max concurrency
		hijackChan:      hijackChan,
		browser:         browser,
	}
}

func (c *Crawler2) Run() []*db.History {
	log.Info().Msg("Starting crawler")
	c.CreateScopeFromProvidedUrls()
	// Spawn a goroutine to listen to hijack results and schedule new pages for crawling
	var inScopeHistoryItems []*db.History
	go func() {
		for hijackResult := range c.hijackChan {
			if hijackResult.History.Method != "GET" {
				item := &CrawlItem{url: hijackResult.History.URL, depth: lib.CalculateURLDepth(hijackResult.History.URL), visited: true, isError: false}
				c.pages.Store(item.url, item)
			}
			// Process the history item
			if c.scope.IsInScope(hijackResult.History.URL) {
				inScopeHistoryItems = append(inScopeHistoryItems, hijackResult.History)
			}
			for _, url := range hijackResult.DiscoveredURLs {
				// Calculate the depth of the URL
				depth := lib.CalculateURLDepth(url)

				// If the URL is within the depth limit, schedule it for crawling
				if depth <= c.maxDepth {
					c.wg.Add(1)
					go c.crawlPage(&CrawlItem{url: url, depth: depth})
					log.Debug().Str("url", url).Msg("Scheduled page to crawl from hijack result")
				}
			}
		}
	}()
	for _, url := range c.startURLs {
		c.wg.Add(1)
		go c.crawlPage(&CrawlItem{url: url, depth: lib.CalculateURLDepth(url)})
		// Also crawl common files

		baseURL, err := lib.GetBaseURL(url)
		if err != nil {
			continue
		}

		for _, u := range viper.GetStringSlice("crawl.common_files") {
			c.wg.Add(1)
			go c.crawlPage(&CrawlItem{url: baseURL + u, depth: lib.CalculateURLDepth(u)})
		}
	}

	c.wg.Wait()
	log.Info().Msg("Finished crawling")
	return inScopeHistoryItems
}

// CreateScopeFromProvidedUrls creates scope items given the received urls
func (c *Crawler2) CreateScopeFromProvidedUrls() {
	// When it can be provided via CLI, the initial scope should be reused
	c.scope.CreateScopeItemsFromUrls(c.startURLs, "www")
	log.Warn().Interface("scope", c.scope).Msg("Crawler scope created")
}

func (c *Crawler2) isAllowedCrawlDepth(item *CrawlItem) bool {
	if c.maxDepth == 0 {
		return true
	}
	return item.depth <= c.maxDepth
}

func (c *Crawler2) shouldCrawl(item *CrawlItem) bool {
	// Should start by checking if the url is in an excluded pattern from c.excludePatterns
	for _, pattern := range c.excludePatterns {
		if strings.Contains(item.url, pattern) {
			log.Debug().Str("url", item.url).Str("pattern", pattern).Msg("Skipping page because it matches an exclude pattern")
			return false
		}
	}
	if c.scope.IsInScope(item.url) && c.isAllowedCrawlDepth(item) {
		if value, ok := c.pages.Load(item.url); ok {
			if value.(*CrawlItem).visited {
				log.Debug().Str("url", item.url).Msg("Skipping page because it has been crawled before")
				return false // If this page has been crawled before, skip it
			}
		}
		return true
	}
	log.Debug().Str("url", item.url).Int("depth", item.depth).Msg("Skipping page because either exceeds the max depth or is not in scope")
	return false
}

func (c *Crawler2) getBrowserPage() *rod.Page {
	page := c.browser.NewPage()
	web.IgnoreCertificateErrors(page)
	// Enabling audits, security, etc
	auditEnableError := proto.AuditsEnable{}.Call(page)
	if auditEnableError != nil {
		log.Error().Err(auditEnableError).Msg("Error enabling browser audit events")
	}
	securityEnableError := proto.SecurityEnable{}.Call(page)
	if securityEnableError != nil {
		log.Error().Err(securityEnableError).Msg("Error enabling browser security events")
	}
	return page
}

func (c *Crawler2) crawlPage(item *CrawlItem) {
	defer c.wg.Done()
	log.Debug().Str("url", item.url).Msg("Crawling page")
	c.concLimit <- struct{}{}
	defer func() { <-c.concLimit }()

	if !c.shouldCrawl(item) {
		return
	}
	// Increment pageCounter
	c.counterLock.Lock()
	if c.maxPagesToCrawl != 0 && c.pageCounter >= c.maxPagesToCrawl {
		log.Info().Int("max_pages_to_crawl", c.maxPagesToCrawl).Int("crawled", c.pageCounter).Msg("Stopping crawler due to max pages to crawl")
		c.browser.Close()
		c.counterLock.Unlock()
		return
	}
	c.pageCounter++
	c.counterLock.Unlock()

	c.pages.Store(item.url, item)
	url := item.url

	page := c.getBrowserPage()
	defer c.browser.ReleasePage(page)

	// There's another implementation which applies to the whole browser which might be better ()
	web.ListenForPageEvents(url, page)

	urlData := c.loadPageAndGetAnchors(url, page)

	if !urlData.IsError {
		c.interactWithPage(page)
	}

	// Recursively crawl to links
	for _, link := range urlData.DiscoveredURLs {
		if c.shouldCrawl(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)}) {
			c.wg.Add(1)
			go c.crawlPage(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)})
		}
	}

	if value, ok := c.pages.Load(item.url); ok {
		value.(*CrawlItem).visited = true
	}
}

func (c *Crawler2) loadPageAndGetAnchors(url string, page *rod.Page) CrawledPageResut {
	navigationTimeout := time.Duration(viper.GetInt("navigation.timeout"))
	navigateError := page.Timeout(navigationTimeout * time.Second).Navigate(url)
	if navigateError != nil {
		log.Warn().Err(navigateError).Str("url", url).Msg("Error navigating to page")
		return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: true}
	}

	err := page.Timeout(navigationTimeout * time.Second).WaitLoad()

	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("Error waiting for page complete load while crawling")
		// here, even though the page has not complete loading, we could still try to get some data
		return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: true}
	}

	anchors, err := web.GetPageAnchors(page)
	if err != nil {
		log.Error().Msg("Could not get page anchors")
		return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: false}
	}
	return CrawledPageResut{URL: url, DiscoveredURLs: anchors, IsError: false}
}

func (c *Crawler2) interactWithPage(page *rod.Page) {
	interactionTimeout := time.Duration(viper.GetInt("crawl.interaction.timeout"))
	lib.DoWorkWithTimeout(c.browser.InteractWithPage, []interface{}{page}, interactionTimeout*time.Second)
}
