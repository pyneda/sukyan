package manual

import (
	"github.com/pyneda/sukyan/internal/browser"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/web"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/rs/zerolog/log"
)

type UserBrowser struct {
	LaunchURL      string
	HijackRequests bool
	HijackConfig   browser.HijackConfig
	WorkspaceID    uint
}

func (ub *UserBrowser) Launch() {
	u := launcher.New().Headless(false).Leakless(true).MustLaunch()
	b := rod.New().ControlURL(u).MustConnect()

	hc := browser.HijackConfig{
		AnalyzeJs:   true,
		AnalyzeHTML: true,
	}
	hijackResultsChannel := make(chan browser.HijackResult)

	browser.Hijack(hc, b, "Browser", hijackResultsChannel, ub.WorkspaceID)
	var page *rod.Page
	if ub.LaunchURL != "" {
		page = b.MustPage(ub.LaunchURL)
	} else {
		page = b.MustPage("")
	}
	web.ListenForWebSocketEvents(page)
	log.Info().Interface("url", page).Msg("Browser loaded")
	lib.SetupCloseHandler()
	for {
		time.Sleep(20 * time.Second)
	}
}
