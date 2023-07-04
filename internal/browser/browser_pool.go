package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	// "github.com/go-rod/rod/lib/proto"
	"github.com/spf13/viper"
)

type BrowserPoolManagerConfig struct {
	PoolSize int
	Source   string
}

type BrowserPoolManager struct {
	launcher             *launcher.Launcher
	browser              *rod.Browser
	pool                 rod.BrowserPool
	config               BrowserPoolManagerConfig
	HijackResultsChannel chan HijackResult
	hijack               bool
}

func NewBrowserPoolManager(config BrowserPoolManagerConfig) *BrowserPoolManager {
	manager := BrowserPoolManager{
		config: config,
	}
	manager.Start()

	return &manager
}

func NewHijackedBrowserPoolManager(config BrowserPoolManagerConfig, hijackResultsChannel chan HijackResult) *BrowserPoolManager {
	manager := BrowserPoolManager{
		config:               config,
		HijackResultsChannel: hijackResultsChannel,
		hijack:               true,
	}
	manager.Start()

	return &manager
}

func (b *BrowserPoolManager) Start() {
	poolSize := 4
	if b.config.PoolSize > 0 {
		poolSize = b.config.PoolSize
	}

	b.pool = rod.NewBrowserPool(poolSize)
}

func (b *BrowserPoolManager) NewBrowser() *rod.Browser {
	browser := b.pool.Get(b.createBrowser)

	// if b.config.UserAgent != "" {
	// 	_ = browser.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Test"})
	// } else if viper.GetString("navigation.user_agent") != "" {
	// 	_ = browser.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: viper.GetString("navigation.user_agent")})
	// }

	return browser
}

func (b *BrowserPoolManager) ReleaseBrowser(browser *rod.Browser) {
	b.pool.Put(browser)
}

func (b *BrowserPoolManager) createBrowser() *rod.Browser {
	l := GetBrowserLauncher()
	controlURL := l.MustLaunch()
	browser := rod.New().ControlURL(controlURL).MustConnect()
	go browser.HandleAuth(viper.GetString("navigation.auth.basic.username"), viper.GetString("navigation.auth.basic.password"))()
	if b.hijack {
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, browser, b.config.Source, b.HijackResultsChannel)
	}
	return browser
}

func (b *BrowserPoolManager) Close() {
	b.pool.Cleanup(func(p *rod.Browser) { p.MustClose() })
	b.browser.Close()
}
