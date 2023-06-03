package crawl

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"github.com/pyneda/sukyan/pkg/scope"
	"github.com/pyneda/sukyan/pkg/web"

	"github.com/go-rod/rod"
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
}

var (
	browser *rod.Browser
	// pool    *rod.PagePool
)

// CreateScopeFromProvidedUrls creates scope items given the received urls
func (c *Crawler) CreateScopeFromProvidedUrls() {
	// When it can be provided via CLI, the initial scope should be reused
	c.Scope.CreateScopeItemsFromUrls(c.StartUrls, "www")
	log.Warn().Interface("scope", c.Scope).Msg("Crawler scope created")
}

// Run starts the crawler
func (c *Crawler) Run() []web.WebPage {

	var wg sync.WaitGroup
	// var to store crawl results
	var crawledPagesResults = []web.WebPage{}
	// create scope
	c.CreateScopeFromProvidedUrls()

	// create channels
	foundLinksChannel := make(chan string, 3000)
	// Pages pending to be crawled
	pendingPagesChannel := make(chan string)
	// Number of pending pages to crawl
	totalPendingPagesChannel := make(chan int)

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
	go c.CrawlMonitor(pendingPagesChannel, crawledPagesChannel, foundLinksChannel, totalPendingPagesChannel)

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
	log.Debug().Msg("Crawl goroutine pages started")

	for url := range pendingPagesChannel {
		log.Info().Str("url", url).Msg("Page crawl started")
		// Should inspect url
		urlData := web.InspectURL(url)

		log.Info().Int("anchors", len(urlData.Anchors)).Str("url", url).Msg("Crawler total anchors found")

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
		totalPendingPagesChannel <- -1
		log.Debug().Str("url", url).Msg("Sending a -1 to totalPendingPagesChannel")
		totalCrawledPages++
		// log.Info().Str("url", url).Msg("Page crawl completed")
	}
	wg.Done()
	log.Debug().Int("total", totalCrawledPages).Msg("End of CrawlPages")

}

// CrawlMonitor monitors crawl state
func (c Crawler) CrawlMonitor(pendingPagesChannel chan string, crawledPagesChannel chan web.WebPage, foundLinksChannel chan string, totalPendingPagesChannel chan int) {
	count := 0
	log.Debug().Msg("Crawl monitor started")
	for c := range totalPendingPagesChannel {
		log.Debug().Int("count", count).Int("received", c).Msg("Crawl monitor received from totalPendingPagesChannel")
		count += c

		if count == 0 && len(foundLinksChannel) == 0 {
			// 	// Close the channels
			log.Debug().Msg("CrawlMonitor closing all the communication channels")
			close(foundLinksChannel)
			close(pendingPagesChannel)
			close(totalPendingPagesChannel)
			close(crawledPagesChannel)
		}
	}

}

// ProcessCrawledLinks receives crawler found links via a channel and adds them to crawl if they are in scope and have not been crawled previously
func (c *Crawler) ProcessCrawledLinks(foundLinksChannel chan string, pendingPagesChannel chan string, totalPendingPagesChannel chan int) {
	processed := make(map[string]bool)
	log.Debug().Msg("Process crawled links started")

	for link := range foundLinksChannel {
		log.Debug().Str("link", link).Msg("ProcessCrawledLinks received data from foundLinksChannel")
		if c.Scope.IsInScope(link) { // Maybe not need to double check
			if !processed[link] {
				log.Debug().Str("link", link).Int("total_processed", len(processed)).Msg("Adding new in scope link  to pendingPagesChannel")
				totalPendingPagesChannel <- 1
				pendingPagesChannel <- link
				processed[link] = true
				log.Debug().Str("link", link).Int("total_processed", len(processed)).Msg("Added new in scope link  to pendingPagesChannel")
			} else {
				log.Debug().Str("link", link).Msg("Received an already processed link")
			}
		} else {
			log.Debug().Str("link", link).Msg("Crawler found link which is out of the current scope")
		}

	}
}
