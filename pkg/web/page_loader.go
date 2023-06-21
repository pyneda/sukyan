package web

import (
	"github.com/go-rod/rod"
)

type PageLoader struct {
	ExtraHeaders      map[string]string
	HijackEnabled     bool
	HijackConfig      HijackConfig
	AuditDOM          bool
	Proxy             string
	EmulationConfig   EmulationConfig
	IgnoreCertCerrors bool
	// https://go-rod.github.io/#/emulation?id=locale-and-timezone
	Timezone string
	Source   string
}

func (l *PageLoader) GetPage() (*rod.Browser, *rod.Page, error) {
	browser := rod.New().MustConnect()
	// Should hijack if required

	hijackResultsChannel := make(chan HijackResult)
	if l.HijackEnabled {
		Hijack(l.HijackConfig, browser, l.Source, hijackResultsChannel)
	}
	page := browser.MustPage("")
	if l.IgnoreCertCerrors == true {
		IgnoreCertificateErrors(page)
	}
	return browser, page, nil
}
