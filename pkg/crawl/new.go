package crawl

import (
	"sync"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"time"
)

type Crawler2 struct {
	scope           scope.Scope
	maxPagesToCrawl int
	maxDepth        int
	startURLs       []string
	browser         *web.BrowserManager
	pages           sync.Map
	pageCounter     int
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

func NewCrawler(startURLs []string, maxPagesToCrawl int, maxDepth int, poolSize int) *Crawler2 {
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
		concLimit:       make(chan struct{}, poolSize+2), // Set max concurrency
		hijackChan:      hijackChan,
		browser:         browser,
	}
}

func (c *Crawler2) Run() {
	log.Info().Msg("Starting crawler")
	c.CreateScopeFromProvidedUrls()
	// Spawn a goroutine to listen to hijack results and schedule new pages for crawling
	go func() {
		for hijackResult := range c.hijackChan {
			if hijackResult.History.Method != "GET" {
				item := &CrawlItem{url: hijackResult.History.URL, depth: lib.CalculateURLDepth(hijackResult.History.URL), visited: true, isError: false}
				c.pages.Store(item.url, item)
			}
			for _, url := range hijackResult.DiscoveredURLs {
				// Calculate the depth of the URL
				depth := lib.CalculateURLDepth(url)

				// If the URL is within the depth limit, schedule it for crawling
				if depth <= c.maxDepth {
					c.wg.Add(1)
					go c.crawlPage(&CrawlItem{url: url, depth: depth})
					log.Info().Str("url", url).Msg("Scheduled page to crawl from hijack result")
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
	if c.scope.IsInScope(item.url) && c.isAllowedCrawlDepth(item) {
		if value, ok := c.pages.Load(item.url); ok {
			if value.(*CrawlItem).visited {
				log.Debug().Str("url", item.url).Msg("Skipping page because it has been crawled before")
				return false // If this page has been crawled before, skip it
			}
		}
		return true
	}
	log.Info().Str("url", item.url).Int("depth", item.depth).Msg("Skipping page because either exceeds the max depth or is not in scope")
	return false
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

	page := c.browser.NewPage()
	defer c.browser.ReleasePage(page)

	urlData := web.CrawlURL(url, page)
	if urlData.IsError {
		return // If there was an error crawling the page, skip it
	}

	interactionTimeout := time.Duration(viper.GetInt("crawl.interaction.timeout"))
	lib.DoWorkWithTimeout(c.browser.InteractWithPage, []interface{}{page}, interactionTimeout*time.Second)

	// Recursively crawl to links
	for _, link := range urlData.DiscoveredURLs {
		if c.shouldCrawl(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)}) {
			c.wg.Add(1)
			go c.crawlPage(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)})
			// log.Info().Str("url", link).Msg("Scheduled page to crawl")
		}
	}

	if value, ok := c.pages.Load(item.url); ok {
		value.(*CrawlItem).visited = true
	}
}
