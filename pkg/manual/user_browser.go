package manual

import (
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
	HijackConfig   web.HijackConfig
}

func (b *UserBrowser) Launch() {
	u := launcher.New().Headless(false).Leakless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	hc := web.HijackConfig{
		AnalyzeJs:   true,
		AnalyzeHTML: true,
	}
	web.Hijack(hc, browser)
	var page *rod.Page
	if b.LaunchURL != "" {
		page = browser.MustPage(b.LaunchURL)
	} else {
		page = browser.MustPage("")
	}
	log.Info().Interface("url", page).Msg("Browser loaded")
	lib.SetupCloseHandler()
	for {
		time.Sleep(20 * time.Second)
	}
}
