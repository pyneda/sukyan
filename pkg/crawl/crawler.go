package crawl

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type CrawlOptions struct {
	ExtraHeaders    map[string][]string
	MaxDepth        int
	MaxPagesToCrawl int
}

type Crawler struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
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
	scanID                  uint
	scanJobID               uint
	clickedElements         sync.Map
	submittedForms          sync.Map
	processedResponseHashes sync.Map
	counterLock             sync.Mutex
	wg                      sync.WaitGroup
	concLimit               chan struct{}
	hijackChan              chan browser.HijackResult
	normalizedURLCounts     sync.Map
	eventStore              sync.Map
	maxPagesWithSameParams  int
}

type CrawlItem struct {
	url       string
	depth     int
	visited   bool
	scheduled bool
	isError   bool
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

func NewCrawler(startURLs []string, maxPagesToCrawl int, maxDepth int, poolSize int, excludePatterns []string, workspaceID, taskID, scanID, scanJobID uint, extraHeaders map[string][]string) *Crawler {
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
		scanID,
		scanJobID,
	)
	return &Crawler{
		Options:                options,
		startURLs:              startURLs,
		excludePatterns:        excludePatterns,
		concLimit:              make(chan struct{}, poolSize+2), // Set max concurrency
		hijackChan:             hijackChan,
		browser:                browser,
		ignoredExtensions:      viper.GetStringSlice("crawl.ignored_extensions"),
		workspaceID:            workspaceID,
		taskID:                 taskID,
		scanID:                 scanID,
		scanJobID:              scanJobID,
		maxPagesWithSameParams: viper.GetInt("crawl.max_pages_with_same_params"),
	}
}

// NewCrawlerWithContext creates a new Crawler with context for cancellation support
func NewCrawlerWithContext(ctx context.Context, startURLs []string, maxPagesToCrawl int, maxDepth int, poolSize int, excludePatterns []string, workspaceID, taskID, scanID, scanJobID uint, extraHeaders map[string][]string) *Crawler {
	crawler := NewCrawler(startURLs, maxPagesToCrawl, maxDepth, poolSize, excludePatterns, workspaceID, taskID, scanID, scanJobID, extraHeaders)
	crawler.ctx, crawler.cancel = context.WithCancel(ctx)
	return crawler
}

func (c *Crawler) Run() []*db.History {
	return c.RunWithContext(context.Background())
}

// RunWithContext runs the crawler with context support for cancellation
func (c *Crawler) RunWithContext(ctx context.Context) []*db.History {
	// If crawler was created with context, use that; otherwise use provided context
	if c.ctx == nil {
		c.ctx, c.cancel = context.WithCancel(ctx)
	}
	defer c.cancel()

	taskLog := log.With().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Logger()
	taskLog.Info().Msg("Starting crawler")
	c.CreateScopeFromProvidedUrls()
	// Spawn a goroutine to listen to hijack results and schedule new pages for crawling
	var inScopeHistoryItems []*db.History
	var historyMutex sync.Mutex
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				taskLog.Info().Msg("Crawler hijack listener stopped due to context cancellation")
				return
			case hijackResult, ok := <-c.hijackChan:
				if !ok {
					return
				}
				taskLog.Info().Str("url", hijackResult.History.URL).Int("status_code", hijackResult.History.StatusCode).Int("response_body_size", hijackResult.History.ResponseBodySize).Str("method", hijackResult.History.Method).Int("discovered_urls", len(hijackResult.DiscoveredURLs)).Msg("Received crawl response")
				if hijackResult.History.Method != "GET" {
					item := &CrawlItem{url: hijackResult.History.URL, depth: lib.CalculateURLDepth(hijackResult.History.URL), visited: true, isError: false}
					c.pages.Store(item.url, item)
				}
				// Process the history item
				if c.scope.IsInScope(hijackResult.History.URL) {
					historyMutex.Lock()
					inScopeHistoryItems = append(inScopeHistoryItems, hijackResult.History)
					historyMutex.Unlock()
				}
				// Check if the same response has been processed before
				responseHash := hijackResult.History.ResponseHash()
				_, processed := c.processedResponseHashes.Load(responseHash)
				if !processed {
					c.processedResponseHashes.Store(responseHash, true)
					for _, url := range hijackResult.DiscoveredURLs {
						// Check context before scheduling new pages
						select {
						case <-c.ctx.Done():
							taskLog.Info().Msg("Crawler stopping URL scheduling due to context cancellation")
							return
						default:
						}
						// Checking if max pages to crawl are reached
						c.counterLock.Lock()
						if c.Options.MaxPagesToCrawl != 0 && c.pageCounter >= c.Options.MaxPagesToCrawl {
							taskLog.Info().Int("max_pages_to_crawl", c.Options.MaxPagesToCrawl).Int("crawled", c.pageCounter).Msg("Not processing new crawler urls due to max pages to crawl")
							c.counterLock.Unlock()
							continue // Max pages reached, skip processing the rest of the discovered URLs
						}
						c.counterLock.Unlock()
						// Calculate the depth of the URL
						depth := lib.CalculateURLDepth(url)

						// If the URL is within the depth limit, schedule it for crawling
						if c.Options.MaxDepth == 0 || depth <= c.Options.MaxDepth {
							c.wg.Add(1)
							go c.crawlPage(&CrawlItem{url: url, depth: depth})
							taskLog.Debug().Str("url", url).Msg("Scheduled page to crawl from hijack result")
						}
					}
				}
			}
		}
	}()
	taskLog.Info().Interface("start_urls", c.startURLs).Msg("Crawling start urls")
	for _, url := range c.startURLs {
		// Check context before scheduling start URLs
		select {
		case <-c.ctx.Done():
			taskLog.Info().Msg("Crawler cancelled before processing all start URLs")
			c.wg.Wait()
			c.browser.Close()
			historyMutex.Lock()
			defer historyMutex.Unlock()
			return inScopeHistoryItems
		default:
		}
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

	c.wg.Wait()
	taskLog.Info().Msg("Finished crawling")
	c.browser.Close()
	historyMutex.Lock()
	defer historyMutex.Unlock()
	for _, item := range inScopeHistoryItems {
		events, ok := c.eventStore.Load(item.URL)
		if ok {
			eventsList := events.(*[]web.PageEvent)
			web.AnalyzeGatheredEvents(item, *eventsList)
		}
	}
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
	if c.maxPagesWithSameParams > 0 {
		// Check how many times the URL with the same parameters has been crawled
		normalizedURL, err := lib.NormalizeURLParams(item.url)
		if err != nil {
			log.Error().Err(err).Str("url", item.url).Msg("Error normalizing URL")
			return false
		}

		count, _ := c.normalizedURLCounts.Load(normalizedURL)

		if count != nil && count.(int) >= c.maxPagesWithSameParams {
			log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Int("maxPagesWithSameParams", c.maxPagesWithSameParams).Str("normalized", normalizedURL).Str("url", item.url).Msg("Skipping page because it has reached the maximum count of crawled pages with the same parameters")
			return false
		}
	}

	// Check if the url is in scope and if it's within the max depth
	if c.scope.IsInScope(item.url) && c.isAllowedCrawlDepth(item) {
		if value, ok := c.pages.Load(item.url); ok {
			if value.(*CrawlItem).visited || value.(*CrawlItem).scheduled {
				log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Msg("Skipping page because it has been visited or scheduled")
				return false // If this page has been crawled before, skip it
			} else {
				value.(*CrawlItem).scheduled = true
			}
		}
		return true
	}
	log.Debug().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Int("depth", item.depth).Msg("Skipping page because either exceeds the max depth or is not in scope")
	return false
}

func (c *Crawler) getBrowserPage(targetURL string) *rod.Page {
	page := c.browser.NewPage()
	setupTimeout := time.Duration(viper.GetInt("crawl.page_setup_timeout"))
	page = page.Timeout(setupTimeout * time.Second)

	web.IgnoreCertificateErrors(page)
	// Set extra headers and cookies if provided
	if c.Options.ExtraHeaders != nil {
		err := browser.SetPageHeadersAndCookies(page, c.Options.ExtraHeaders, targetURL)
		if err != nil {
			log.Error().Err(err).Msg("Error setting page headers and cookies")
		}
	}

	// Enabling audits, security, etc
	if !page.LoadState(&proto.AuditsEnable{}) {
		auditEnableError := proto.AuditsEnable{}.Call(page)
		if auditEnableError != nil {
			log.Error().Err(auditEnableError).Msg("Error enabling browser audit events")
		}
	}

	if !page.LoadState(&proto.SecurityEnable{}) {
		securityEnableError := proto.SecurityEnable{}.Call(page)
		if securityEnableError != nil {
			log.Error().Err(securityEnableError).Msg("Error enabling browser security events")
		}
	}

	page = page.CancelTimeout()

	return page
}

func (c *Crawler) crawlPage(item *CrawlItem) {
	defer c.wg.Done()

	// Check context at the start
	if c.ctx != nil {
		select {
		case <-c.ctx.Done():
			log.Debug().Uint("workspace", c.workspaceID).Str("url", item.url).Msg("Crawler page skipped due to context cancellation")
			return
		default:
		}
	}

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

	if c.maxPagesWithSameParams > 0 {
		// Track the URL with the same parameters values to avoid crawling the same URL a high amount of times
		normalizedURL, err := lib.NormalizeURLParams(item.url)
		if err != nil {
			log.Error().Err(err).Str("url", item.url).Msg("Error normalizing URL")
		} else {
			// Increment or initialize count for normalized URL
			count, _ := c.normalizedURLCounts.LoadOrStore(normalizedURL, 0)
			c.normalizedURLCounts.Store(normalizedURL, count.(int)+1)
		}
	}

	url := item.url

	page := c.getBrowserPage(url)
	defer c.browser.ReleasePage(page)
	ctx, cancel := context.WithCancel(context.Background())
	eventStream := web.ListenForPageEvents(ctx, item.url, page, c.workspaceID, c.taskID, c.scanID, c.scanJobID, db.SourceCrawler)
	defer cancel()

	go func() {
		for {
			select {
			case event, ok := <-eventStream:
				if !ok {
					return // exit if channel is closed
				}
				log.Info().Uint("workspace", c.workspaceID).Uint("task", c.taskID).Str("url", item.url).Interface("event", event).Msg("Received page event")
				val, _ := c.eventStore.LoadOrStore(event.URL, &[]web.PageEvent{})
				events := val.(*[]web.PageEvent)
				*events = append(*events, event)
				c.eventStore.Store(event.URL, events)
			case <-ctx.Done():
				return
			}
		}
	}()
	urlData := c.loadPageAndGetAnchors(url, page)

	if value, ok := c.pages.Load(item.url); ok {
		value.(*CrawlItem).visited = true
	}

	if !urlData.IsError {
		log.Debug().Uint("workspace", c.workspaceID).Str("url", url).Msg("Starting to interact with page")
		interactionTimeout := time.Duration(viper.GetInt("crawl.interaction.timeout"))
		_, err := lib.DoWorkWithTimeout(c.interactWithPage, []interface{}{page}, interactionTimeout*time.Second)
		if err != nil {
			log.Warn().Err(err).Uint("workspace", c.workspaceID).Str("url", url).Msg("Timeout interacting with page")
		}
		log.Debug().Uint("workspace", c.workspaceID).Str("url", url).Msg("Finished interacting with page")
	}

	// Recursively crawl to links
	for _, link := range urlData.DiscoveredURLs {
		// Check context before scheduling child page
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				log.Debug().Uint("workspace", c.workspaceID).Str("url", item.url).Msg("Crawler stopping child page scheduling due to context cancellation")
				return
			default:
			}
		}
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

	if viper.GetBool("navigation.wait_stable") {
		waitStableDuration := time.Duration(viper.GetInt("navigation.wait_stable_duration"))
		waitStableTimeout := time.Duration(viper.GetInt("navigation.wait_stable_timeout"))

		ctx, cancel := context.WithTimeout(context.Background(), waitStableTimeout*time.Second)
		defer cancel()

		pageWithTimeout := page.Context(ctx)

		err = pageWithTimeout.WaitStable(waitStableDuration * time.Second)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.Warn().Str("url", url).Msg("Timeout reached while waiting for page to be stable, trying to get data anyway")
			} else {
				log.Warn().Err(err).Str("url", url).Msg("Error waiting for page to be stable, trying to get data anyway")
			}
		}
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
	if len(formElements) == 0 {
		log.Debug().Uint("workspace", c.workspaceID).Msg("No forms found on page")
		return nil
	}
	log.Info().Uint("workspace", c.workspaceID).Int("forms_found", len(formElements)).Msg("Found forms on page")
	for _, form := range formElements {
		xpath, err := form.GetXPath(true)
		if err != nil {
			log.Error().Err(err).Msg("Error getting XPath for form")
			continue
		}
		e := SubmittedForm{
			xpath: xpath,
		}

		skipPreviouslySubmittedForms := viper.GetBool("crawl.interaction.skip_previously_submitted_forms")
		_, submitted := c.submittedForms.Load(e)
		if skipPreviouslySubmittedForms && submitted {
			log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Skipping already submitted form")
		} else {
			log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Filling form")
			web.AutoFillForm(form, page)
			log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Submitting form")
			ok := web.SubmitForm(form, page)
			if ok {
				c.submittedForms.Store(e, true)
				log.Info().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Submitted form")
			} else {
				log.Warn().Uint("workspace", c.workspaceID).Str("xpath", xpath).Msg("Could not submit form")
			}
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
				text, err := btn.Text()
				if err == nil && strings.Contains(strings.ToLower(text), "logout") {
					log.Debug().Uint("workspace", c.workspaceID).Str("xpath", xpath).Str("text", text).Msg("Skipping logout element")
					continue
				}

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
					// c.handleClickableElements(page)
					return err
				}
			}
		}
	}
	log.Debug().Uint("workspace", c.workspaceID).Str("selector", selector).Msg("Finished clicking elements")
	return err
}
