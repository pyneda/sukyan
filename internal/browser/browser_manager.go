package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	// "github.com/go-rod/rod/lib/proto"
	"github.com/spf13/viper"
)

type BrowserManagerConfig struct {
	PoolSize  int
	UserAgent string
}

type BrowserManager struct {
	launcher             *launcher.Launcher
	browser              *rod.Browser
	pool                 rod.BrowserPool
	config               BrowserManagerConfig
	HijackResultsChannel chan HijackResult
}

func NewBrowserManager(config BrowserManagerConfig, source string) *BrowserManager {
	manager := BrowserManager{
		config: config,
	}
	manager.Start(false, source)

	return &manager
}

func NewHijackedBrowserManager(config BrowserManagerConfig, source string, hijackResultsChannel chan HijackResult) *BrowserManager {
	manager := BrowserManager{
		config:               config,
		HijackResultsChannel: hijackResultsChannel,
	}
	manager.Start(true, source)

	return &manager
}

func (b *BrowserManager) Start(hijack bool, source string) {
	l := GetBrowserLauncher()
	controlURL := l.MustLaunch()
	b.browser = rod.New().
		ControlURL(controlURL).
		MustConnect()

	go b.browser.HandleAuth(viper.GetString("navigation.auth.basic.username"), viper.GetString("navigation.auth.basic.password"))()
	poolSize := 4
	if b.config.PoolSize > 0 {
		poolSize = b.config.PoolSize
	}
	if hijack {
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, b.browser, source, b.HijackResultsChannel)
	}
	// b.pool = rod.NewPagePool(poolSize)
	b.pool = rod.NewBrowserPool(poolSize)

}

func (b *BrowserManager) NewBrowser() *rod.Browser {
	browser := b.pool.Get(b.createBrowser)

	// if b.config.UserAgent != "" {
	// 	_ = browser.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Test"})
	// } else if viper.GetString("navigation.user_agent") != "" {
	// 	_ = browser.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: viper.GetString("navigation.user_agent")})
	// }

	return browser
}

func (b *BrowserManager) ReleaseBrowser(browser *rod.Browser) {
	b.pool.Put(browser)
}

func (b *BrowserManager) createBrowser() *rod.Browser {
	l := GetBrowserLauncher()
	controlURL := l.MustLaunch()
	return rod.New().ControlURL(controlURL).MustConnect()
}

func (b *BrowserManager) Close() {
	b.pool.Cleanup(func(p *rod.Browser) { p.MustClose() })
	b.browser.Close()
}
