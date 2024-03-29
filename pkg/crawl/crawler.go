package crawl

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/browser"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"strings"
	"sync"
	"time"
)

type CrawlOptions struct {
	ExtraHeaders    map[string][]string
	MaxDepth        int
	MaxPagesToCrawl int
}
type Crawler struct {
	Options                 CrawlOptions
	scope                   scope.Scope
	startURLs               []string
	excludePatterns         []string
	ignoredExtensions       []string
	browser                 *browser.PagePoolManager
	pages                   sync.Map
	pageCounter             int
	workspaceID             uint
	taskID                  uint
	clickedElements         sync.Map
	submittedForms          sync.Map
	processedResponseHashes sync.Map
	counterLock             sync.Mutex
	wg                      sync.WaitGroup
	concLimit               chan struct{}
	hijackChan              chan browser.HijackResult
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
}

type SubmittedForm struct {
	xpath string
}

func NewCrawler(startURLs []string, maxPagesToCrawl int, maxDepth int, poolSize int, excludePatterns []string, workspaceID, taskID uint, extraHeaders map[string][]string) *Crawler {
	hijackChan := make(chan browser.HijackResult)
	options := CrawlOptions{
		ExtraHeaders:    extraHeaders,
		MaxDepth:        maxDepth,
		MaxPagesToCrawl: maxPagesToCrawl,
	}
	browser := browser.NewHijackedPagePoolManager(
		browser.PagePoolManagerConfig{
			PoolSize: poolSize,
		},
		"Crawler",
		hijackChan,
		workspaceID,
		taskID,
	)
	return &Crawler{
		Options:           options,
		startURLs:         startURLs,
		excludePatterns:   excludePatterns,
		concLimit:         make(chan struct{}, poolSize+2), // Set max concurrency
		hijackChan:        hijackChan,
		browser:           browser,
		ignoredExtensions: viper.GetStringSlice("crawl.ignored_extensions"),
		workspaceID:       workspaceID,
		taskID:            taskID,
	}
}

func (c *Crawler) Run() []*db.History {
	log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Msg("Starting crawler")
	c.CreateScopeFromProvidedUrls()
	// Spawn a goroutine to listen to hijack results and schedule new pages for crawling
	var inScopeHistoryItems []*db.History
	go func() {
		for hijackResult := range c.hijackChan {
			log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", hijackResult.History.URL).Int("status_code", hijackResult.History.StatusCode).Str("method", hijackResult.History.Method).Int("discovered_urls", len(hijackResult.DiscoveredURLs)).Msg("Received hijack result")
			if hijackResult.History.Method != "GET" {
				item := &CrawlItem{url: hijackResult.History.URL, depth: lib.CalculateURLDepth(hijackResult.History.URL), visited: true, isError: false}
				c.pages.Store(item.url, item)
			}
			// Process the history item
			if c.scope.IsInScope(hijackResult.History.URL) {
				inScopeHistoryItems = append(inScopeHistoryItems, hijackResult.History)
			}
			// Check if the same response has been processed before
			responseHash := lib.HashBytes(hijackResult.History.ResponseBody)
			_, processed := c.processedResponseHashes.Load(responseHash)
			if !processed {
				c.processedResponseHashes.Store(responseHash, true)
				for _, url := range hijackResult.DiscoveredURLs {
					// Checking if max pages to crawl are reached
					c.counterLock.Lock()
					if c.Options.MaxPagesToCrawl != 0 && c.pageCounter >= c.Options.MaxPagesToCrawl {
						log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Int("max_pages_to_crawl", c.Options.MaxPagesToCrawl).Int("crawled", c.pageCounter).Msg("Stopping crawler hijacking due to max pages to crawl")
						c.counterLock.Unlock()
						return // terminate the goroutine
					}
					c.counterLock.Unlock()
					// Calculate the depth of the URL
					depth := lib.CalculateURLDepth(url)

					// If the URL is within the depth limit, schedule it for crawling
					if c.Options.MaxDepth == 0 || depth <= c.Options.MaxDepth {
						c.wg.Add(1)
						go c.crawlPage(&CrawlItem{url: url, depth: depth})
						log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", url).Msg("Scheduled page to crawl from hijack result")
					}
				}
			}
		}
	}()
	log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Interface("start_urls", c.startURLs).Msg("Crawling start urls")
	for _, url := range c.startURLs {
		c.wg.Add(1)
		go c.crawlPage(&CrawlItem{url: url, depth: lib.CalculateURLDepth(url)})
		baseURL, err := lib.GetBaseURL(url)
		if err != nil {
			continue
		}
		for _, u := range viper.GetStringSlice("crawl.common.files") {
			c.wg.Add(1)
			go c.crawlPage(&CrawlItem{url: baseURL + u, depth: lib.CalculateURLDepth(u)})
		}
	}

	time.Sleep(5 * time.Second)
	c.wg.Wait()
	log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Msg("Finished crawling")
	c.browser.Close()
	return inScopeHistoryItems
}

// CreateScopeFromProvidedUrls creates scope items given the received urls
func (c *Crawler) CreateScopeFromProvidedUrls() {
	// When it can be provided via CLI, the initial scope should be reused
	c.scope.CreateScopeItemsFromUrls(c.startURLs, "www")
	log.Warn().Interface("scope", c.scope).Msg("Crawler scope created")
}

func (c *Crawler) isAllowedCrawlDepth(item *CrawlItem) bool {
	if c.Options.MaxDepth == 0 {
		return true
	}
	return item.depth <= c.Options.MaxDepth
}

func (c *Crawler) shouldCrawl(item *CrawlItem) bool {
	// Check if the url is in an excluded pattern from c.excludePatterns
	for _, pattern := range c.excludePatterns {
		if strings.Contains(item.url, pattern) {
			log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Str("pattern", pattern).Msg("Skipping page because it matches an exclude pattern")
			return false
		}
	}
	// Check if the url has an ignored extension
	for _, extension := range c.ignoredExtensions {
		if strings.HasSuffix(item.url, extension) {
			log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Str("extension", extension).Msg("Skipping page because it has an ignored extension")
			return false
		}
	}
	// Check if the url is in scope and if it's within the max depth
	if c.scope.IsInScope(item.url) && c.isAllowedCrawlDepth(item) {
		if value, ok := c.pages.Load(item.url); ok {
			if value.(*CrawlItem).visited {
				log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Msg("Skipping page because it has been crawled before")
				return false // If this page has been crawled before, skip it
			}
		}
		return true
	}
	log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Int("depth", item.depth).Msg("Skipping page because either exceeds the max depth or is not in scope")
	return false
}

func (c *Crawler) getBrowserPage() *rod.Page {
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
	if c.Options.ExtraHeaders != nil {
		extraHeaders := browser.ConvertToNetworkHeaders(c.Options.ExtraHeaders)
		page.EnableDomain(&proto.NetworkEnable{})
		err := proto.NetworkSetExtraHTTPHeaders{Headers: extraHeaders}.Call(page)
		if err != nil {
			log.Error().Err(err).Interface("headers", extraHeaders).Msg("Error setting extra HTTP headers")
		}
	}
	return page
}

func (c *Crawler) crawlPage(item *CrawlItem) {
	defer c.wg.Done()
	log.Debug().Uint("workspace", c.workspaceID).Str("url", item.url).Msg("Crawling page")
	c.concLimit <- struct{}{}
	defer func() { <-c.concLimit }()

	if !c.shouldCrawl(item) {
		return
	}
	// Increment pageCounter
	c.counterLock.Lock()
	if c.Options.MaxPagesToCrawl != 0 && c.pageCounter >= c.Options.MaxPagesToCrawl {
		log.Debug().Uint("workspace", c.workspaceID).Int("max_pages_to_crawl", c.Options.MaxPagesToCrawl).Int("crawled", c.pageCounter).Str("url", item.url).Msg("Crawler skipping page due to having reached max pages to crawl")
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
	web.ListenForPageEvents(url, page, c.workspaceID, c.taskID, db.SourceCrawler)

	urlData := c.loadPageAndGetAnchors(url, page)

	if value, ok := c.pages.Load(item.url); ok {
		value.(*CrawlItem).visited = true
	}

	if !urlData.IsError {
		log.Debug().Uint("workspace", c.workspaceID).Str("url", url).Msg("Starting to interact with page")
		interactionTimeout := time.Duration(viper.GetInt("crawl.interaction.timeout"))
		lib.DoWorkWithTimeout(c.interactWithPage, []interface{}{page}, interactionTimeout*time.Second)
		log.Debug().Uint("workspace", c.workspaceID).Str("url", url).Msg("Finished interacting with page")
	}

	// Recursively crawl to links
	for _, link := range urlData.DiscoveredURLs {
		if c.shouldCrawl(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)}) {
			c.wg.Add(1)
			go c.crawlPage(&CrawlItem{url: link, depth: lib.CalculateURLDepth(link)})
		}
	}

}

func (c *Crawler) loadPageAndGetAnchors(url string, page *rod.Page) CrawledPageResut {
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

func (c *Crawler) interactWithPage(page *rod.Page) {
	if viper.GetBool("crawl.interaction.submit_forms") {
		c.handleForms(page)
	}
	if viper.GetBool("crawl.interaction.click_buttons") {
		c.handleClickableElements(page)
	}
}

func (c *Crawler) handleForms(page *rod.Page) (err error) {
	formElements, err := page.Elements("form")
	if err != nil {
		return err
	}
	for _, form := range formElements {
		xpath, err := form.GetXPath(true)
		if err != nil {
			continue
		}
		e := SubmittedForm{
			xpath: xpath,
		}
		_, submitted := c.submittedForms.Load(e)
		if !submitted {
			web.AutoFillForm(form, page)
			web.SubmitForm(form, page)
			c.submittedForms.Store(e, true)
			log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Submitted form")
		}
	}
	return err
}

func (c *Crawler) handleClickableElements(page *rod.Page) {
	c.getAndClickElements("button", page)

	// getAndClickElements("input[type=submit]", p)
	c.getAndClickElements("input[type=button]", page)
	// c.getAndClickElements("a", page)
	log.Debug().Uint("workspace", c.workspaceID).Msg("Finished clicking all elements")
}

func (c *Crawler) getAndClickElements(selector string, page *rod.Page) (err error) {
	elements, err := page.Elements(selector)

	if err == nil {
		for _, btn := range elements {
			xpath, err := btn.GetXPath(true)
			if err != nil {
				continue
			}
			_, clicked := c.clickedElements.Load(xpath)
			if !clicked {
				err = btn.Click(proto.InputMouseButtonLeft, 1)
				if err != nil {
					log.Error().Err(err).Str("xpath", xpath).Str("selector", selector).Msg("Error clicking element")
				} else {
					log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Str("selector", selector).Msg("Clicked button")
					c.clickedElements.Store(xpath, true)
					// Try to handle possible new forms/buttons that might have appeared due to the click (ex. forms inside a modal)
					// Since the forms have been submitted previously, in theory, if the same form appears again, it should be skipped
					// NOTE: Listening for DOM changes might be a better approach
					c.handleForms(page)
					c.handleClickableElements(page)
					return err
				}
			}
		}
	}
	log.Debug().Uint("workspace", c.workspaceID).Str("selector", selector).Msg("Finished clicking elements")
	return err
}
