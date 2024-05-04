package manual

import (
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser"
	"github.com/pyneda/sukyan/pkg/web"

	"github.com/go-rod/rod"
	// "github.com/go-rod/rod/lib/launcher"
	"github.com/rs/zerolog/log"
)

// LaunchUserBrowser launches a browser in non headless mode and logs all network requests
func LaunchUserBrowser(workspaceID uint, initialURL string) {
	log.Info().Uint("workspace", workspaceID).Str("url", initialURL).Msg("Launching browser")
	launcher := browser.GetBrowserLauncher()
	launcher.Delete("--headless")
	controlURL := launcher.MustLaunch()
	b := rod.New().ControlURL(controlURL).MustConnect()
	hc := browser.HijackConfig{
		AnalyzeJs:   true,
		AnalyzeHTML: true,
	}
	hijackResultsChannel := make(chan browser.HijackResult)

	browser.Hijack(hc, b, db.SourceBrowser, hijackResultsChannel, workspaceID, 0)
	var page *rod.Page
	if initialURL != "" {
		page = b.MustPage(initialURL)
	} else {
		page = b.MustPage("")
	}
	web.ListenForWebSocketEvents(page, workspaceID, 0, db.SourceBrowser)
	log.Info().Interface("url", page).Msg("Browser loaded")
	lib.SetupCloseHandler()
	for {
		time.Sleep(20 * time.Second)
	}
}
