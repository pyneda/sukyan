package passive

import (
	"fmt"
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
	PrivateIPScan(item)
	EmailAddressScan(item)
	FileUploadScan(item)
	SessionTokenInURLScan(item)
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

func FileUploadScan(item *db.History) {
	// This is too simple, could also check the headers for content-type: multipart/form-data and other things
	matches := fileUploadRegex.FindAllString(item.ResponseBody, -1)
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

func SessionTokenInURLScan(item *db.History) {
	matches := sessionTokenRegex.FindAllStringSubmatch(item.URL, -1)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered session tokens in URL parameters:")
		for _, match := range matches {
			parameter := match[0]
			value := match[1]
			sb.WriteString(fmt.Sprintf("\n - Parameter: %s, Value: %s", parameter, value))
		}
		details := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.SessionTokenInURLCode, details, 90)
	}
}
