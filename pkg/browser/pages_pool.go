package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type PagePoolManagerConfig struct {
	PoolSize  int
	UserAgent string
}

type PagePoolManager struct {
	browser              *rod.Browser
	pool                 rod.Pool[rod.Page]
	config               PagePoolManagerConfig
	workspaceID          uint
	taskID               uint
	HijackResultsChannel chan HijackResult
}

func NewPagePoolManager(config PagePoolManagerConfig, source string) *PagePoolManager {
	manager := PagePoolManager{
		config: config,
	}
	manager.Start(false, source)

	return &manager
}

func NewHijackedPagePoolManager(config PagePoolManagerConfig, source string, hijackResultsChannel chan HijackResult, workspaceID, taskID uint) *PagePoolManager {
	manager := PagePoolManager{
		config:               config,
		HijackResultsChannel: hijackResultsChannel,
		workspaceID:          workspaceID,
		taskID:               taskID,
	}
	manager.Start(true, source)

	return &manager
}

func (b *PagePoolManager) Start(hijack bool, source string) {
	l := GetBrowserLauncher()
	controlURL := l.MustLaunch()
	b.browser = rod.New().
		ControlURL(controlURL).
		MustConnect()

	go b.browser.HandleAuth(viper.GetString("navigation.auth.basic.username"), viper.GetString("navigation.auth.basic.password"))()
	// b.browser.IgnoreCertErrors(true)
	poolSize := 4
	if b.config.PoolSize > 0 {
		poolSize = b.config.PoolSize
	}
	if hijack {
		Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, b.browser, source, b.HijackResultsChannel, b.workspaceID, b.taskID)
	}
	// b.pool = rod.NewPagePool(poolSize)
	b.pool = rod.NewPagePool(poolSize)

}

func (b *PagePoolManager) NewPage() *rod.Page {
	page, err := b.pool.Get(b.createPage)
	// page.HandleDialog()
	// Set user-agent provided by browser manager config or config file
	if err != nil {
		log.Error().Err(err).Msg("Error getting page from pool")
	}

	if b.config.UserAgent != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Test"})
	} else if viper.GetString("navigation.user_agent") != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: viper.GetString("navigation.user_agent")})
	}

	return page
}

func (b *PagePoolManager) ReleasePage(page *rod.Page) {
	b.pool.Put(page)
}

func (b *PagePoolManager) createPage() (*rod.Page, error) {
	return b.browser.Page(proto.TargetCreateTarget{})
}

func (b *PagePoolManager) Close() {
	b.pool.Cleanup(func(p *rod.Page) { p.Close() })
	b.browser.Close()
}
