package passive

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/tokens"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
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
		db.CreateIssueFromHistoryAndTemplate(item, db.JavaSerializedObjectDetectedCode, "The page responds using the `application/x-java-serialized-object` content type.", 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
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
				db.CreateIssueFromHistoryAndTemplate(item, r.IssueCode, r.Description, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
			}
		}
	}
}

func ScanHistoryItem(item *db.History) {
	if strings.Contains(item.ResponseContentType, "text/html") {
		if viper.GetBool("passive.checks.js.enabled") {
			PassiveJavascriptScan(item)
		}
		DirectoryListingScan(item)
		UnencryptedPasswordFormDetectionScan(item)
	} else if strings.Contains(item.ResponseContentType, "javascript") || strings.Contains(item.ResponseContentType, "ecmascript") {
		if viper.GetBool("passive.checks.js.enabled") {
			passiveJavascriptSecretsScan(item)
			PassiveJavascriptScan(item)
		}
	}
	StorageBucketDetectionScan(item)
	DatabaseErrorScan(item)
	LeakedApiKeysScan(item)
	PrivateIPScan(item)
	JwtDetectionScan(item)
	EmailAddressScan(item)
	FileUploadScan(item)
	SessionTokenInURLScan(item)
	PrivateKeyScan(item)
	DBConnectionStringScan(item)
	PasswordInGetRequestScan(item)
	ContentTypesScan(item)
	WebSocketUsageScan(item)
	ServerSideIncludesUsageScan(item)
	WebAssemblyDetectionScan(item)

	if viper.GetBool("passive.checks.exceptions.enabled") {
		ExceptionsScan(item)
	}

	if viper.GetBool("passive.checks.missconfigurations.enabled") {
		MissconfigurationScan(item)
	}

	if viper.GetBool("passive.checks.headers.enabled") {
		ScanHistoryItemHeaders(item)
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
	bodyStr := string(item.ResponseBody)
	for _, match := range matches {

		if strings.Contains(bodyStr, match) {
			isDirectoryListing = true
		}
	}
	if isDirectoryListing {
		db.CreateIssueFromHistoryAndTemplate(item, db.DirectoryListingCode, "", 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func PrivateIPScan(item *db.History) {
	host, _ := lib.GetHostFromURL(item.URL)
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	matches := privateIPRegex.FindAllString(matchAgainst, -1)
	matches = lib.FilterOutString(matches, host)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered Internal IP addresses:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredIPs := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.PrivateIpsCode, discoveredIPs, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func DatabaseErrorScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}

	match := SearchDatabaseErrors(matchAgainst)
	if match != nil {
		errorDescription := fmt.Sprintf("Discovered database error: \n - Database type: %s\n - Error: %s", match.DatabaseName, match.MatchStr)

		db.CreateIssueFromHistoryAndTemplate(item, db.DatabaseErrorsCode, errorDescription, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}

}

func EmailAddressScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	matches := emailRegex.FindAllString(matchAgainst, -1)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered email addresses:")
		for _, match := range lib.GetUniqueItems(matches) {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredEmails := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.EmailAddressesCode, discoveredEmails, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func FileUploadScan(item *db.History) {
	// This is too simple, could also check the headers for content-type: multipart/form-data and other things
	matches := fileUploadRegex.FindAllString(string(item.ResponseBody), -1)
	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered file upload inputs:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		details := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.FileUploadDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func attemtToCrackJwtIfRequired(item *db.History, jwt *db.JsonWebToken) {
	if jwt != nil && !jwt.TestedEmbeddedWordlist && !jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Token seen for the first time, attempting to crack it with embedded wordlist")
		crackResult, err := tokens.CrackJWTAndCreateIssue(item, jwt)
		if err != nil {
			log.Err(err).Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Failed to crack JWT")
		} else if crackResult.Found {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Str("secret", crackResult.Secret).Msg("JWT cracked")
		} else {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("JWT secret could not be found")
		}
	} else if jwt != nil && jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Token already cracked")
		noTaskJob := uint(0)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("A JWT token that had been cracked previously has been discovered in a %s request to %s\n\n", item.Method, item.URL))
		sb.WriteString("Details:\n")
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", jwt.Algorithm))

		sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n\n", jwt.Secret))
		issue, err := db.CreateIssueFromHistoryAndTemplate(
			item,
			db.JwtWeakSigningSecretCode,
			sb.String(),
			100,
			"",
			item.WorkspaceID,
			item.TaskID,
			&noTaskJob,
		)
		if err != nil {
			log.Error().Err(err).Str("token", jwt.Token).Msg("Failed to create issue for already cracked JWT")
		} else {
			log.Info().Str("token", jwt.Token).Str("secret", jwt.Secret).Uint("issue_id", issue.ID).Msg("Created issue for already cracked JWT")
		}
	}
}

func JwtDetectionScan(item *db.History) {

	// Check ResponseBody
	matches := jwtRegex.FindAllString(string(item.ResponseBody), -1)
	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Detected potential JWTs in response body:")
		log.Info().Strs("matches", matches).Msg("Found JWTs")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
			jwt, err := db.Connection.GetOrCreateJWTFromTokenAndHistory(match, item.ID)
			if err != nil {
				log.Err(err).Msg("Failed to get or create JWT")
				continue
			}
			attemtToCrackJwtIfRequired(item, jwt)

		}
		details := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.JwtDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
	// Check RequestHeaders
	req, err := item.GetRequestHeadersAsMap()
	if err == nil {
		checkHeadersForJwt(item, req)
	}

	// Check ResponseHeaders
	res, err := item.GetResponseHeadersAsMap()
	if err == nil {
		checkHeadersForJwt(item, res)
	}
}

func checkHeadersForJwt(item *db.History, headers map[string][]string) {
	var sb strings.Builder

	for key, values := range headers {
		for _, value := range values {
			matches := jwtRegex.FindAllString(value, -1)
			if len(matches) > 0 {
				log.Info().Strs("matches", matches).Msg("Found JWTs in headers")

				sb.WriteString(fmt.Sprintf("Detected potential JWTs in %s header:", key))
				for _, match := range matches {
					sb.WriteString(fmt.Sprintf("\n - %s", match))
					jwt, err := db.Connection.GetOrCreateJWTFromTokenAndHistory(match, item.ID)
					if err != nil {
						log.Err(err).Msg("Failed to get or create JWT")
						continue
					}
					attemtToCrackJwtIfRequired(item, jwt)
				}

			}
		}
	}
	details := sb.String()
	if details != "" {
		db.CreateIssueFromHistoryAndTemplate(item, db.JwtDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
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
		db.CreateIssueFromHistoryAndTemplate(item, db.SessionTokenInUrlCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
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
		matches := keyMatch.Regex.FindAllString(string(item.ResponseBody), -1)
		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Discovered %s Private Key(s):", keyMatch.Type))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n\n%s", match))
			}
			discoveredKeys := sb.String()
			db.CreateIssueFromHistoryAndTemplate(item, db.PrivateKeysCode, discoveredKeys, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
		}
	}
}

func DBConnectionStringScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
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
			db.CreateIssueFromHistoryAndTemplate(item, db.DbConnectionStringsCode, discoveredStrings, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
		}
	}
}

func PasswordInGetRequestScan(item *db.History) {
	// NOTE: This is also cheked looking for forms in the page in: UnencryptedPasswordFormDetectionScan
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
		db.CreateIssueFromHistoryAndTemplate(item, db.PasswordInGetRequestCode, description, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func StorageBucketDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
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
		db.CreateIssueFromHistoryAndTemplate(item, db.StorageBucketDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func LeakedApiKeysScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	var sb strings.Builder
	matched := false

	for patternName, pattern := range apiKeysPatternsMap {
		matches := pattern.FindAllString(matchAgainst, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("\nDiscovered %s", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
		}
	}

	details := sb.String()

	if matched {
		db.CreateIssueFromHistoryAndTemplate(item, db.ExposedApiCredentialsCode, details, 80, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func WebSocketUsageScan(item *db.History) {
	headers, err := item.GetResponseHeadersAsMap()
	if err != nil {
		return
	}
	if item.StatusCode == 101 && lib.SliceContains(headers["Upgrade"], "websocket") {
		details := fmt.Sprintf("WebSockets in use detected at %s", item.URL)
		db.CreateIssueFromHistoryAndTemplate(item, db.WebsocketDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func ServerSideIncludesUsageScan(item *db.History) {
	extensions := []string{".shtml", ".stm", ".shtm"}
	parsedURL, err := url.Parse(item.URL)
	if err != nil {
		return
	}
	for _, ext := range extensions {
		if strings.HasSuffix(parsedURL.Path, ext) {
			details := fmt.Sprintf("Extension %s detected, meaning Server Side Includes (SSI) is probably detected by the web server", ext)
			db.CreateIssueFromHistoryAndTemplate(item, db.SsiDetectedCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
			return
		}
	}
}

func WebAssemblyDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	matches := webAssemblyURLRegex.FindAllString(matchAgainst, -1)

	var sb strings.Builder
	confidence := 80
	issueDetected := false

	if len(matches) > 0 {
		issueDetected = true
		sb.WriteString("WebAssembly detected in response body. Found the following WebAssembly URL(s):")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
	}

	parsedURL, err := url.Parse(item.URL)
	if err == nil && strings.HasSuffix(parsedURL.Path, ".wasm") {
		if !issueDetected {
			issueDetected = true
			sb.WriteString("WebAssembly module detected: URL ends with '.wasm' extension.")
		} else {
			sb.WriteString("\nAdditional detection: URL ends with '.wasm' extension.")
			confidence = 100
		}
	}

	if strings.EqualFold(item.ResponseContentType, "application/wasm") {
		if !issueDetected {
			issueDetected = true
			sb.WriteString("WebAssembly content detected: Response header 'Content-Type' is 'application/wasm'.")
		} else {
			sb.WriteString("\nAdditional detection: Response header 'Content-Type' is 'application/wasm'.")
			confidence = 100
		}
	}

	if issueDetected {
		details := sb.String()
		db.CreateIssueFromHistoryAndTemplate(item, db.WebassemblyDetectedCode, details, confidence, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}
