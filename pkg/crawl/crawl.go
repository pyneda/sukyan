package crawl

import (
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Crawler is used to crawl and inspect web pages
type Crawler struct {
	Scope           scope.Scope
	StartUrls       []string
	Depth           int
	MaxPagesToCrawl int
	PagesPoolSize   int
	PageTimeout     time.Duration
	CrawlTimeout    time.Duration
	ExtraHeaders    map[string]string
	Include         *regexp.Regexp
	Exclude         *regexp.Regexp
	browserManager  *web.BrowserManager
	processed       map[string]bool
}

// CreateScopeFromProvidedUrls creates scope items given the received urls
func (c *Crawler) CreateScopeFromProvidedUrls() {
	// When it can be provided via CLI, the initial scope should be reused
	c.Scope.CreateScopeItemsFromUrls(c.StartUrls, "www")
	log.Warn().Interface("scope", c.Scope).Msg("Crawler scope created")
}

// Run starts the crawler
func (c *Crawler) Run() []web.WebPage {
	// create channels
	foundLinksChannel := make(chan string, 3000)
	// Pages pending to be crawled
	pendingPagesChannel := make(chan string)
	// Number of pending pages to crawl
	totalPendingPagesChannel := make(chan int)
	hijackResultsChannel := make(chan web.HijackResult)
	c.processed = make(map[string]bool)

	var wg sync.WaitGroup
	// var to store crawl results
	var crawledPagesResults = []web.WebPage{}

	// create scope
	c.CreateScopeFromProvidedUrls()

	c.browserManager = web.NewHijackedBrowserManager(
		web.BrowserManagerConfig{
			PoolSize: c.PagesPoolSize,
		},
		hijackResultsChannel,
	)

	// Get the crawled pages results
	crawledPagesChannel := make(chan web.WebPage)

	go func() {
		for _, startURL := range c.StartUrls {
			foundLinksChannel <- startURL
		}
	}()
	go func() {
		for crawledPage := range crawledPagesChannel {
			crawledPagesResults = append(crawledPagesResults, crawledPage)
			crawledPage.LogPageData()
			//log.Info().Str("url", crawledPage.url).Str("responseUrl", crawledPage.responseURL).Msg("Crawled page completed")
		}
	}()

	go c.ProcessCrawledLinks(foundLinksChannel, pendingPagesChannel, totalPendingPagesChannel)
	go c.ProcessHijackResults(foundLinksChannel, pendingPagesChannel, totalPendingPagesChannel, hijackResultsChannel)
	go c.CrawlMonitor(pendingPagesChannel, crawledPagesChannel, foundLinksChannel, totalPendingPagesChannel, hijackResultsChannel)

	// Add Startings urls in its channel

	log.Info().Msg("Starting to crawl")
	for i := 0; i < c.PagesPoolSize; i++ {
		wg.Add(1)
		go c.CrawlPages(&wg, foundLinksChannel, pendingPagesChannel, crawledPagesChannel, totalPendingPagesChannel)
	}

	wg.Wait()
	return crawledPagesResults
}

// CrawlPages crawls pages
func (c *Crawler) CrawlPages(wg *sync.WaitGroup, foundLinksChannel chan string, pendingPagesChannel chan string, crawledPagesChannel chan web.WebPage, totalPendingPagesChannel chan int) {
	totalCrawledPages := 0
	log.Info().Msg("Crawl goroutine pages started")

	for url := range pendingPagesChannel {
		if c.Scope.IsInScope(url) && strings.HasPrefix(url, "http") {

			log.Info().Str("url", url).Msg("Page crawl started")
			page := c.browserManager.NewPage()
			// urlData := web.CrawlURL(url, page)

			result, err := lib.DoWorkWithTimeout(web.CrawlURL, []interface{}{url, page}, 5*time.Second)
			if err != nil {
				log.Error().Err(err).Str("url", url).Msg("Timeout error crawling page")
			}

			urlData, ok := result.(web.WebPage) //Cating
			if !ok {
				log.Error().Err(err).Str("url", url).Msg("Failed to cast result to web.Webpage")
			}
			// c.browserManager.FocusPageAndInteractWithpage(page)
			lib.DoWorkWithTimeout(c.browserManager.InteractWithPage, []interface{}{page}, 5*time.Second)
			log.Info().Int("anchors", len(urlData.Anchors)).Str("url", url).Msg("Crawler total anchors found")
			c.browserManager.ReleasePage(page)

			// Add the found links to its channel
			log.Debug().Str("url", url).Msg("Sending found links to foundLinksChannel")
			for _, link := range urlData.Anchors {
				if c.Scope.IsInScope(link) && strings.HasPrefix(link, "http") {
					//go func() {
					log.Debug().Str("link", link).Msg("Sending found link to foundLinksChannel")
					foundLinksChannel <- link
					//}()
				}
			}
			// Send the crawled page data to crawled pages
			crawledPagesChannel <- urlData
			log.Debug().Str("url", url).Msg("Sent current url data to crawledPagesChannel")

			log.Debug().Str("url", url).Msg("Sending a -1 to totalPendingPagesChannel")
			totalCrawledPages++

			// log.Info().Str("url", url).Msg("Page crawl completed")
		}
		totalPendingPagesChannel <- -1

	}
	wg.Done()
	log.Debug().Int("total", totalCrawledPages).Msg("End of CrawlPages")
	c.browserManager.Close()

}

// CrawlMonitor monitors crawl state
func (c Crawler) CrawlMonitor(pendingPagesChannel chan string, crawledPagesChannel chan web.WebPage, foundLinksChannel chan string, totalPendingPagesChannel chan int, hijackResultsChannel chan web.HijackResult) {
	count := 0
	log.Debug().Msg("Crawl monitor started")
	for c := range totalPendingPagesChannel {
		log.Debug().Int("count", count).Int("received", c).Msg("Crawl monitor received from totalPendingPagesChannel")
		count += c

		if count == 0 && len(foundLinksChannel) == 0 {
			// Close the channels
			log.Debug().Msg("CrawlMonitor closing all the communication channels")
			close(foundLinksChannel)
			close(pendingPagesChannel)
			close(hijackResultsChannel)
			close(totalPendingPagesChannel)
			close(crawledPagesChannel)
		} else {
			log.Warn().Int("count", count).Msg("Crawl monitor received from totalPendingPagesChannel")
		}
	}

}

// ProcessCrawledLinks receives crawler found links via a channel and adds them to crawl if they are in scope and have not been crawled previously
func (c *Crawler) ProcessCrawledLinks(foundLinksChannel chan string, pendingPagesChannel chan string, totalPendingPagesChannel chan int) {
	// processed := make(map[string]bool)
	log.Debug().Msg("Process crawled links started")

	for link := range foundLinksChannel {
		// log.Info().Str("link", link).Msg("ProcessCrawledLinks received data from foundLinksChannel")
		if !c.processed[link] {
			log.Debug().Str("link", link).Int("total_processed", len(c.processed)).Msg("Adding new in scope link  to pendingPagesChannel")
			totalPendingPagesChannel <- 1
			pendingPagesChannel <- link
			c.processed[link] = true
			log.Debug().Str("link", link).Int("total_processed", len(c.processed)).Msg("Added new in scope link  to pendingPagesChannel")
		} else {
			log.Debug().Str("link", link).Msg("Received an already processed link")
		}
	}
}

func (c *Crawler) ProcessHijackResults(foundLinksChannel chan string, pendingPagesChannel chan string, totalPendingPagesChannel chan int, hijackResultsChannel chan web.HijackResult) {
	// processed := make(map[string]bool)
	log.Info().Msg("Process hijack results started")

	for result := range hijackResultsChannel {
		c.processed[result.History.URL] = true
		// log.Warn().Str("url", result.History.URL).Msg("Received hijack result")
		for _, link := range result.DiscoveredURLs {
			if !c.processed[link] && link != result.History.URL && !isIgnoredExtension(link) {
				log.Debug().Str("link", link).Int("total_processed", len(c.processed)).Msg("Adding new in scope link  to pendingPagesChannel")
				c.processed[link] = true
				totalPendingPagesChannel <- 1
				pendingPagesChannel <- link
				log.Debug().Str("link", link).Int("total_processed", len(c.processed)).Msg("Added new in scope link  to pendingPagesChannel")
			} else {
				log.Debug().Str("link", link).Msg("Received an already processed link")
			}
		}
	}
	log.Info().Msg("Process hijack results finished")

}
