package web

// import (
// 	"github.com/go-rod/rod"
// 	"github.com/go-rod/rod/lib/launcher"
// 	"github.com/go-rod/rod/lib/proto"
// 	"github.com/spf13/viper"
// )

// func getBrowserLauncher() *launcher.Launcher {
// 	options := launcher.New().
// 		Headless(viper.GetBool("crawl.headless")).
// 		Set("allow-running-insecure-content").
// 		Set("disable-infobars").
// 		Set("disable-extensions")

// 	if viper.GetString("navigation.proxy") != "" {
// 		options.Proxy(viper.GetString("navigation.proxy"))
// 	}
// 	if viper.GetBool("navigation.browser.disable_images") {
// 		options = options.Set("disable-images")
// 	}
// 	if viper.GetBool("navigation.browser.disable_gpu") {
// 		options = options.Set("disable-gpu")
// 	}
// 	return options
// }

// type BrowserManagerConfig struct {
// 	PoolSize  int
// 	UserAgent string
// }

// type BrowserManager struct {
// 	launcher             *launcher.Launcher
// 	browser              *rod.Browser
// 	pool                 rod.PagePool
// 	config               BrowserManagerConfig
// 	HijackResultsChannel chan HijackResult
// }

// func NewBrowserManager(config BrowserManagerConfig, source string) *BrowserManager {
// 	manager := BrowserManager{
// 		config: config,
// 	}
// 	manager.Start(false, source)

// 	return &manager
// }

// func NewHijackedBrowserManager(config BrowserManagerConfig, source string, hijackResultsChannel chan HijackResult) *BrowserManager {
// 	manager := BrowserManager{
// 		config:               config,
// 		HijackResultsChannel: hijackResultsChannel,
// 	}
// 	manager.Start(true, source)

// 	return &manager
// }

// func (b *BrowserManager) Start(hijack bool, source string) {
// 	l := getBrowserLauncher()
// 	controlURL := l.MustLaunch()
// 	b.browser = rod.New().
// 		ControlURL(controlURL).
// 		MustConnect()

// 	go b.browser.HandleAuth(viper.GetString("navigation.auth.basic.username"), viper.GetString("navigation.auth.basic.password"))()
// 	poolSize := 4
// 	if b.config.PoolSize > 0 {
// 		poolSize = b.config.PoolSize
// 	}
// 	if hijack {
// 		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, b.browser, source, b.HijackResultsChannel)
// 	}
// 	// b.pool = rod.NewPagePool(poolSize)
// 	b.pool = rod.NewPagePool(poolSize)

// }

// func (b *BrowserManager) NewPage() *rod.Page {
// 	page := b.pool.Get(b.createPage)
// 	// page.HandleDialog()
// 	// Set user-agent provided by browser manager config or config file
// 	if b.config.UserAgent != "" {
// 		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Test"})
// 	} else if viper.GetString("navigation.user_agent") != "" {
// 		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: viper.GetString("navigation.user_agent")})
// 	}

// 	return page
// }

// func (b *BrowserManager) ReleasePage(page *rod.Page) {
// 	b.pool.Put(page)
// }

// func (b *BrowserManager) createPage() *rod.Page {
// 	return b.browser.MustPage()
// }

// func (b *BrowserManager) Close() {
// 	b.pool.Cleanup(func(p *rod.Page) { p.MustClose() })
// 	b.browser.Close()
// }
