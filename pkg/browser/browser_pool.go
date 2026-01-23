package browser

import (
	"sync"

	"github.com/go-rod/rod"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	scannerBrowserPool    *BrowserPoolManager
	playgroundBrowserPool *BrowserPoolManager
	scannerOnce           sync.Once
	playgroundOnce        sync.Once
)

// GetBrowserPoolManager returns a singleton instance of BrowserPoolManager used by active scanners
func GetScannerBrowserPoolManager() *BrowserPoolManager {
	scannerOnce.Do(func() {
		scannerBrowserPool = NewBrowserPoolManager(BrowserPoolManagerConfig{PoolSize: viper.GetInt("scan.browser.pool_size"), Source: db.SourceScanner}, 0, 0)
	})
	return scannerBrowserPool
}

// GetPlaygroundBrowserPoolManager returns a singleton instance of BrowserPoolManager used by the playground
func GetPlaygroundBrowserPoolManager() *BrowserPoolManager {
	playgroundOnce.Do(func() {
		playgroundBrowserPool = NewBrowserPoolManager(BrowserPoolManagerConfig{PoolSize: 3, Source: db.SourceRepeater}, 0, 0)
	})
	return playgroundBrowserPool
}

type BrowserPoolManagerConfig struct {
	PoolSize int
	Source   string
}

type BrowserPoolManager struct {
	// launcher             *launcher.Launcher
	pool                 rod.Pool[rod.Browser]
	config               BrowserPoolManagerConfig
	HijackResultsChannel chan HijackResult
	hijack               bool
	workspaceID          uint
	taskID               uint
	scanID               uint
	scanJobID            uint
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
	browser, err := b.pool.Get(b.createBrowser)
	if err != nil {
		log.Error().Err(err).Msg("Error getting browser from pool")
	}

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

func (b *BrowserPoolManager) createBrowser() (*rod.Browser, error) {
	l := GetBrowserLauncher()
	controlURL := l.MustLaunch()
	browser := rod.New().ControlURL(controlURL).MustConnect()
	// browser.IgnoreCertErrors(true)
	go browser.HandleAuth(viper.GetString("navigation.auth.basic.username"), viper.GetString("navigation.auth.basic.password"))()
	if b.hijack {
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, browser, b.config.Source, b.HijackResultsChannel, b.workspaceID, b.taskID, b.scanID, b.scanJobID)
	}
	return browser, nil
}

func (b *BrowserPoolManager) Cleanup() {
	b.pool.Cleanup(func(p *rod.Browser) { p.Close() })
}

// ShutdownBrowserPools gracefully shuts down all singleton browser pools,
// closing all browser processes. Should be called during application shutdown.
func ShutdownBrowserPools() {
	if scannerBrowserPool != nil {
		log.Info().Msg("Shutting down scanner browser pool")
		scannerBrowserPool.Cleanup()
	}
	if playgroundBrowserPool != nil {
		log.Info().Msg("Shutting down playground browser pool")
		playgroundBrowserPool.Cleanup()
	}
}
