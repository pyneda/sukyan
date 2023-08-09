package web

import (
	"github.com/go-rod/rod"
	"github.com/pyneda/sukyan/internal/browser"
)

type PageLoader struct {
	ExtraHeaders      map[string]string
	HijackEnabled     bool
	HijackConfig      browser.HijackConfig
	AuditDOM          bool
	Proxy             string
	EmulationConfig   browser.EmulationConfig
	IgnoreCertCerrors bool
	// https://go-rod.github.io/#/emulation?id=locale-and-timezone
	Timezone    string
	Source      string
	WorkspaceID uint
}

func (l *PageLoader) GetPage() (*rod.Browser, *rod.Page, error) {
	b := rod.New().MustConnect()
	// Should hijack if required

	hijackResultsChannel := make(chan browser.HijackResult)
	if l.HijackEnabled {
		browser.Hijack(l.HijackConfig, b, l.Source, hijackResultsChannel, l.WorkspaceID)
	}
	page := b.MustPage("")
	if l.IgnoreCertCerrors == true {
		IgnoreCertificateErrors(page)
	}
	return b, page, nil
}
