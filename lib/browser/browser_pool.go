package browser

import (
	"sync"

	"github.com/go-rod/rod"
	"github.com/pyneda/sukyan/db"
	"github.com/spf13/viper"
)

var (
	scannerBrowserPool *BrowserPoolManager
	once               sync.Once
)

// GetBrowserPoolManager returns a singleton instance of BrowserPoolManager used by active scanners
func GetScannerBrowserPoolManager() *BrowserPoolManager {
	once.Do(func() {
		scannerBrowserPool = NewBrowserPoolManager(BrowserPoolManagerConfig{PoolSize: viper.GetInt("scan.browser.pool_size"), Source: db.SourceScanner}, 0, 0)
	})
	return scannerBrowserPool
}

type BrowserPoolManagerConfig struct {
	PoolSize int
	Source   string
}

type BrowserPoolManager struct {
	// launcher             *launcher.Launcher
	pool                 rod.BrowserPool
	config               BrowserPoolManagerConfig
	HijackResultsChannel chan HijackResult
	hijack               bool
	workspaceID          uint
	taskID               uint
}

func NewBrowserPoolManager(config BrowserPoolManagerConfig, workspaceID, taskID uint) *BrowserPoolManager {
	manager := BrowserPoolManager{
		config:      config,
		workspaceID: workspaceID,
		taskID:      taskID,
	}
	manager.Start()

	return &manager
}

func NewHijackedBrowserPoolManager(config BrowserPoolManagerConfig, hijackResultsChannel chan HijackResult, workspaceID uint) *BrowserPoolManager {
	manager := BrowserPoolManager{
		config:               config,
		HijackResultsChannel: hijackResultsChannel,
		hijack:               true,
		workspaceID:          workspaceID,
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
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, browser, b.config.Source, b.HijackResultsChannel, b.workspaceID, b.taskID)
	}
	return browser
}

func (b *BrowserPoolManager) Cleanup() {
	b.pool.Cleanup(func(p *rod.Browser) { p.Close() })
}
