package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

func getBrowserLauncher() *launcher.Launcher {
	return launcher.New().Headless(true)
}

type BrowserManagerConfig struct {
	PoolSize  int
	UserAgent string
}

type BrowserManager struct {
	launcher             *launcher.Launcher
	browser              *rod.Browser
	pool                 rod.PagePool
	config               BrowserManagerConfig
	HijackResultsChannel chan HijackResult
	focusChan            chan *rod.Page
}

func NewBrowserManager(config BrowserManagerConfig) *BrowserManager {
	manager := BrowserManager{
		config:    config,
		focusChan: make(chan *rod.Page, 1), // buffered channel to allow one page to be focused at a time
	}
	manager.Start(false)

	return &manager
}

func NewHijackedBrowserManager(config BrowserManagerConfig, hijackResultsChannel chan HijackResult) *BrowserManager {
	manager := BrowserManager{
		config:               config,
		HijackResultsChannel: hijackResultsChannel,
	}
	manager.Start(true)

	return &manager
}

func (b *BrowserManager) InteractWithPage(p *rod.Page) {
	// b.focusChan <- p  // send page to channel, blocking if another function is currently focusing
	// p.Activate()
	InteractWithPage(p)
	// <-b.focusChan  // receive from channel, unblocking the next function that wants to focus
}

func (b *BrowserManager) Start(hijack bool) {
	l := getBrowserLauncher()
	controlURL := l.MustLaunch()
	b.browser = rod.New().
		ControlURL(controlURL).
		MustConnect()

	poolSize := 4
	if b.config.PoolSize > 0 {
		poolSize = b.config.PoolSize
	}
	if hijack {
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, b.browser, b.HijackResultsChannel)
	}
	b.pool = rod.NewPagePool(poolSize)
}

func (b *BrowserManager) NewPage() *rod.Page {
	page := b.pool.Get(b.createPage)
	if b.config.UserAgent != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Test"})
	}
	return page
}

func (b *BrowserManager) ReleasePage(page *rod.Page) {
	b.pool.Put(page)
}

func (b *BrowserManager) createPage() *rod.Page {
	return b.browser.MustPage()
}

func (b *BrowserManager) Close() {
	b.pool.Cleanup(func(p *rod.Page) { p.MustClose() })
	b.browser.Close()
}
