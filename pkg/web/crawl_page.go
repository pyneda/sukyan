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
}

func CrawlURL(url string, page *rod.Page) WebPage {

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

	// Requesting page
	// var e proto.NetworkResponseReceived
	// // https://github.com/go-rod/rod/issues/213
	// wait := page.WaitEvent(&e)
	navigateError := page.Navigate(url)
	if navigateError != nil {
		log.Error().Err(navigateError).Str("url", url).Msg("Error navigating to page")
		return WebPage{URL: url, Anchors: []string{}}
	}

	// wait()
	navigationTimeout := time.Duration(viper.GetInt("navigation.timeout"))
	err := page.Timeout(navigationTimeout * time.Second).WaitLoad()

	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("Error waiting for page complete load while crawling")
		return WebPage{URL: url, Anchors: []string{}}
	} else {
		log.Debug().Str("url", url).Msg("Page fully loaded on browser and ready to be analyzed")
	}

	// https://chromedevtools.github.io/devtools-protocol/tot/Runtime/#method-globalLexicalScopeNames
	// globalScopeNames, err := proto.RuntimeGlobalLexicalScopeNames{}.Call(page)

	// if err != nil {
	// 	log.Info().Err(err).Msg("Could not get global scope names")
	// }
	// log.Info().Interface("names", globalScopeNames).Msg("Global scope names")

	data := GetPageData(page, url)
	return data
}
