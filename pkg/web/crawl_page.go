package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"time"
)

type CrawledPageResut struct {
	URL            string
	History        *db.History
	DiscoveredURLs []string
	IsError        bool
}

func CrawlURL(url string, page *rod.Page) CrawledPageResut {

	IgnoreCertificateErrors(page)
	// Enabling audits, security, etc
	auditEnableError := proto.AuditsEnable{}.Call(page)
	if auditEnableError != nil {
		log.Error().Err(auditEnableError).Str("url", url).Msg("Error enabling browser audit events")
	}
	securityEnableError := proto.SecurityEnable{}.Call(page)
	if securityEnableError != nil {
		log.Error().Err(securityEnableError).Str("url", url).Msg("Error enabling browser security events")
	}
	ListenForPageEvents(url, page)

	navigationTimeout := time.Duration(viper.GetInt("navigation.timeout"))
	navigateError := page.Timeout(navigationTimeout * time.Second).Navigate(url)
	if navigateError != nil {
		log.Warn().Err(navigateError).Str("url", url).Msg("Error navigating to page")
		// return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: true}
	}

	err := page.Timeout(navigationTimeout * time.Second).WaitLoad()

	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("Error waiting for page complete load while crawling")
		// here, even though the page has not complete loading, we could still try to get some data
		// return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: true}
	}

	// https://chromedevtools.github.io/devtools-protocol/tot/Runtime/#method-globalLexicalScopeNames
	// globalScopeNames, err := proto.RuntimeGlobalLexicalScopeNames{}.Call(page)
	// if err != nil {
	// 	log.Info().Err(err).Msg("Could not get global scope names")
	// }
	// log.Info().Interface("names", globalScopeNames).Msg("Global scope names")

	anchors, err := GetPageAnchors(page)
	if err != nil {
		log.Error().Msg("Could not get page anchors")
		return CrawledPageResut{URL: url, DiscoveredURLs: []string{}, IsError: false}
	}
	return CrawledPageResut{URL: url, DiscoveredURLs: anchors, IsError: false}
}
