package passive

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"strings"

	wappalyzer "github.com/projectdiscovery/wappalyzergo"
)

func ScanHistoryItem(item *db.History) {
	headers, _ := item.GetResponseHeadersAsMap()
	wappalyzerClient, _ := wappalyzer.New()
	fingerprints := wappalyzerClient.Fingerprint(headers, []byte(item.ResponseBody))
	log.Info().Interface("fingerprints", fingerprints).Msg("Fingerprints found")
	if strings.Contains(item.ContentType, "text/html") {
		PassiveJavascriptScan(item)
		DirectoryListingScan(item)
	} else if strings.Contains(item.ContentType, "javascript") {
		PassiveJavascriptScan(item)
	}

}

func PassiveJavascriptScan(item *db.History) {
	jsSources := FindJsSources(item.ResponseBody)
	jsSinks := FindJsSinks(item.ResponseBody)
	jquerySinks := FindJquerySinks(item.ResponseBody)
	log.Info().Str("url", item.URL).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked HTML response")
	if len(jsSources) > 0 || len(jsSinks) > 0 || len(jquerySinks) > 0 {
		CreateJavascriptSourcesAndSinksInformationalIssue(item, jsSources, jsSinks, jquerySinks)
	}
}

func DirectoryListingScan(item *db.History) {
	matches := []string{
		"Index of", 
		"Parent Directory", 
		"Directory Listing",
		"Directory listing for",
		"Directory: /",
		"[To Parent Directory]",
	}
	isDirectoryListing := false
	for _, match := range matches {
		if strings.Contains(item.ResponseBody, match) {
			isDirectoryListing = true
		}
	}
	if isDirectoryListing {
		db.CreateIssueFromHistoryAndTemplate(item, db.DirectoryListingCode, 90)
	}
}
