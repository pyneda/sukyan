package passive

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"strings"
	"regexp"
	"fmt"

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
	PrivateIPScan(item)
	EmailAddressScan(item)
	FileUploadScan(item)

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
		db.CreateIssueFromHistoryAndTemplate(item, db.DirectoryListingCode, "", 90)
	}
}


var privateIPRegex = regexp.MustCompile(`\b((10\.\d{1,3}\.\d{1,3}\.\d{1,3})|(172\.(1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3})|(192\.168\.\d{1,3}\.\d{1,3})|(127\.0\.0\.1))\b`)

func PrivateIPScan(item *db.History) {
	matchAgainst := item.RawResponse
	if matchAgainst == "" {
		matchAgainst = item.ResponseBody
	}
	matches := privateIPRegex.FindAllString(matchAgainst, -1)
	
	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered Internal IP addresses:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredIPs := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.PrivateIPsCode, discoveredIPs, 90)
	}
}


var emailRegex = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

func EmailAddressScan(item *db.History) {
	matchAgainst := item.RawResponse
	if matchAgainst == "" {
		matchAgainst = item.ResponseBody
	}
	matches := emailRegex.FindAllString(matchAgainst, -1)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered email addresses:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredEmails := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.EmailAddressesCode, discoveredEmails, 90)
	}
}


var fileUploadRegex = regexp.MustCompile(`(?i)<input[^>]*type=["']?file["']?`)

func FileUploadScan(item *db.History) {
	// This is too simple, could also check the headers for content-type: multipart/form-data and other things
	matches := emailRegex.FindAllString(matchAgainst, -1)
	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered file upload inputs:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		details := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.FileUploadDetectedCode, details, 90)
	}
}
