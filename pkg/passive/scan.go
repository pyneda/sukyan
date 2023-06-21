package passive

import (
	"fmt"
	wappalyzer "github.com/projectdiscovery/wappalyzergo"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"net/url"
	"regexp"
	"strings"
)

func ContentTypesScan(item *db.History) {
	// TODO: Implementation
	contentType := item.ResponseContentType
	// if strings.Contains(contentType, "text/html") {
	// } else if strings.Contains(contentType, "javascript") {
	// } else if strings.Contains(contentType, "application/json") {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked JSON response")
	// } else if strings.Contains(contentType, "application/ld+json") {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked JSON-LD response")
	// } else if strings.Contains(contentType, "application/xml") {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked application/xml response")
	// } else if strings.Contains(contentType, "text/xml") {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked text/xml response")
	// } else if strings.Contains(contentType, "text/csv") {
	// 	log.Warn().Str("url", ctx.Request.URL().String()).Msg("Hijacked CSV response")
	// } else if strings.Contains(contentType, "text/css") {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked CSS response")
	// } else if strings.Contains(contentType, "application/x-java-serialized-object") {
	if strings.Contains(contentType, "application/x-java-serialized-object") {
		log.Warn().Str("url", item.URL).Msg("Hijacked java serialized object response")
		db.CreateIssueFromHistoryAndTemplate(item, db.JavaSerializedObjectCode, "The page responds using the `application/x-java-serialized-object` content type.", 90)
	}
	// } else {
	// 	log.Info().Str("url", ctx.Request.URL().String()).Str("contentType", contentType).Msg("Hijacked non common response")

	// }
}

func ScanHistoryItemHeaders(item *db.History) {
	checks := getHeaderChecks()
	headers, _ := item.GetResponseHeadersAsMap()

	for _, check := range checks {
		result := check.Check(headers)
		for _, r := range result {
			if r.Matched {
				db.CreateIssueFromHistoryAndTemplate(item, r.IssueCode, r.Description, 90)
			}
		}
	}
}

func ScanHistoryItem(item *db.History) {
	if viper.GetBool("passive.wappalyzer") {
		headers, _ := item.GetResponseHeadersAsMap()
		wappalyzerClient, _ := wappalyzer.New()
		fingerprints := wappalyzerClient.Fingerprint(headers, []byte(item.ResponseBody))
		log.Info().Interface("fingerprints", fingerprints).Msg("Fingerprints found")
	}

	if strings.Contains(item.ResponseContentType, "text/html") {
		if viper.GetBool("passive.js.enabled") {
			PassiveJavascriptScan(item)
		}
		DirectoryListingScan(item)
	} else if strings.Contains(item.ResponseContentType, "javascript") {
		if viper.GetBool("passive.js.enabled") {
			PassiveJavascriptScan(item)
		}
	}
	StorageBucketDetectionScan(item)
	PrivateIPScan(item)
	EmailAddressScan(item)
	FileUploadScan(item)
	SessionTokenInURLScan(item)
	PrivateKeyScan(item)
	DBConnectionStringScan(item)
	PasswordInGetRequestScan(item)
	ContentTypesScan(item)
	if viper.GetBool("passive.headers.checks.enabled") {
		ScanHistoryItemHeaders(item)
	}
}

func PassiveJavascriptScan(item *db.History) {
	jsSources := FindJsSources(item.ResponseBody)
	jsSinks := FindJsSinks(item.ResponseBody)
	jquerySinks := FindJquerySinks(item.ResponseBody)
	// log.Info().Str("url", item.URL).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked HTML response")
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

func PrivateKeyScan(item *db.History) {
	type KeyMatch struct {
		Type  string
		Regex *regexp.Regexp
	}
	keyMatches := []KeyMatch{
		{"RSA", rsaPrivateKeyRegex},
		{"DSA", dsaPrivateKeyRegex},
		{"EC", ecPrivateKeyRegex},
		{"OpenSSH", opensshPrivateKeyRegex},
		{"PEM", pemPrivateKeyRegex},
	}

	for _, keyMatch := range keyMatches {
		matches := keyMatch.Regex.FindAllString(item.ResponseBody, -1)
		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Discovered %s Private Key(s):", keyMatch.Type))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n\n%s", match))
			}
			discoveredKeys := sb.String()
			db.CreateIssueFromHistoryAndTemplate(item, db.PrivateKeysCode, discoveredKeys, 90)
		}
	}
}

func DBConnectionStringScan(item *db.History) {
	matchAgainst := item.RawResponse
	if matchAgainst == "" {
		matchAgainst = item.ResponseBody
	}

	connectionStringRegexes := []*regexp.Regexp{
		mongoDBConnectionStringRegex,
		postgreSQLConnectionStringRegex,
		postGISConnectionStringRegex,
		mySQLConnectionStringRegex,
		msSQLConnectionStringRegex,
		oracleConnectionStringRegex,
		sqliteConnectionStringRegex,
		redisConnectionStringRegex,
		rabbitMQConnectionStringRegex,
		cassandraConnectionStringRegex,
		neo4jConnectionStringRegex,
		couchDBConnectionStringRegex,
		influxDBConnectionStringRegex,
		memcachedConnectionStringRegex,
	}

	for _, regex := range connectionStringRegexes {
		matches := regex.FindAllString(matchAgainst, -1)

		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString("Discovered database connection strings:")
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
			discoveredStrings := sb.String()
			db.CreateIssueFromHistoryAndTemplate(item, db.DBConnectionStringsCode, discoveredStrings, 90)
		}
	}
}

func PasswordInGetRequestScan(item *db.History) {
	if item.Method != "GET" {
		return
	}
	commonParameters := []string{
		"password",
		"pass",
		"pwd",
		"user_pass",
		"passwd",
		"passcode",
		"pin",
	}

	u, err := url.Parse(item.URL)
	if err != nil {
		return
	}
	query := u.Query()

	var passwordParams []string
	for _, match := range commonParameters {
		if value, ok := query[match]; ok {
			passwordParams = append(passwordParams, fmt.Sprintf("Parameter: %s, Value: %s", match, value[0]))
		}
	}

	if len(passwordParams) > 0 {
		description := "Detected password in URL: " + strings.Join(passwordParams, "\n  - ")
		db.CreateIssueFromHistoryAndTemplate(item, db.PasswordInGetRequestCode, description, 90)
	}
}

func StorageBucketDetectionScan(item *db.History) {
	matchAgainst := item.RawResponse
	if matchAgainst == "" {
		matchAgainst = item.ResponseBody
	}
	var sb strings.Builder
	matched := false

	// Detect buckets in URLs.
	for patternName, pattern := range bucketsURlsPatternsMap {
		matches := pattern.FindAllString(matchAgainst, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("Discovered %s bucket URLs:", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}

		}
	}

	// Detect bucket errors in body.
	for patternName, pattern := range bucketBodyPatternsMap {
		matches := pattern.FindAllString(matchAgainst, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("\nDiscovered %s bucket errors:", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
		}
	}

	details := sb.String()

	if matched {
		db.CreateIssueFromHistoryAndTemplate(item, db.StorageBucketDetectedCode, details, 90)
	}
}
