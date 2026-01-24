package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// BackupFilePaths contains paths for backup files
var BackupFilePaths = []string{
	// Web server config backups
	"web.config.bak",
	"web.config.old",
	"web.config.backup",
	"web.config.orig",
	"web.config~",
	".htaccess.bak",
	".htaccess.old",
	".htaccess.backup",
	".htaccess~",
	"httpd.conf.bak",
	"nginx.conf.bak",
	"nginx.conf.old",

	// PHP config backups
	"config.php.bak",
	"config.php.old",
	"config.php~",
	"settings.php.bak",
	"settings.php.old",
	"wp-config.php.bak",
	"wp-config.php.old",
	"wp-config.php.backup",
	"configuration.php.bak",
	"configuration.php.old",

	// Database config backups
	"database.yml.bak",
	"database.yml.old",
	"application.yml.bak",
	"application.yml.old",
	"db.php.bak",

	// .NET config backups
	"appsettings.json.bak",
	"appsettings.json.old",
	"web.config.txt",
	"applicationHost.config.bak",

	// General config backups
	"config.json.bak",
	"config.json.old",
	"config.yaml.bak",
	"config.yml.bak",
	"settings.json.bak",
	".env.bak",
	".env.backup",
	".env.old",

	// Source file backups
	"index.php.bak",
	"index.php.old",
	"index.html.bak",
	"login.php.bak",
	"admin.php.bak",
}

// IsBackupFileValidationFunc validates backup file responses
func IsBackupFileValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: Backup files should never be HTML
	// Servers often return their homepage for non-existent paths
	if isHTMLResponse(history) {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Backup file detected: %s\n\n", history.URL))

	// Check content type - backup files usually served as octet-stream or text
	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "application/octet-stream") ||
		strings.Contains(contentType, "text/x-php") ||
		strings.Contains(contentType, "application/x-httpd-php") {
		confidence += 30
		details.WriteString("- Content-Type indicates non-executed file\n")
	}

	// Check for configuration indicators
	configIndicators := []struct {
		pattern     string
		description string
	}{
		{"DB_PASSWORD", "Database password"},
		{"DB_USERNAME", "Database username"},
		{"DB_HOST", "Database host"},
		{"API_KEY", "API key"},
		{"SECRET_KEY", "Secret key"},
		{"PRIVATE_KEY", "Private key"},
		{"AWS_SECRET", "AWS secret"},
		{"connection", "Connection string"},
		{"password", "Password field"},
		{"<?php", "PHP source code"},
		{"<configuration>", "XML configuration"},
		{"<connectionStrings>", ".NET connection strings"},
		{"RewriteRule", "Apache rewrite rules"},
		{"server {", "Nginx server block"},
	}

	foundIndicators := 0
	for _, indicator := range configIndicators {
		if strings.Contains(bodyStr, indicator.pattern) {
			foundIndicators++
			confidence += 15
			details.WriteString(fmt.Sprintf("- Contains: %s\n", indicator.description))
		}
	}

	if foundIndicators > 0 {
		details.WriteString(fmt.Sprintf("\nTotal sensitive indicators found: %d\n", foundIndicators))
	}

	// Base confidence for 200 response
	if history.StatusCode == 200 {
		confidence += 20
	}

	// Check file extension in URL
	url := strings.ToLower(history.URL)
	backupExtensions := []string{".bak", ".old", ".backup", ".orig", ".save", "~"}
	for _, ext := range backupExtensions {
		if strings.HasSuffix(url, ext) {
			confidence += 20
			details.WriteString(fmt.Sprintf("- URL ends with backup extension: %s\n", ext))
			break
		}
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverBackupFiles discovers backup files
func DiscoverBackupFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  BackupFilePaths,
			Concurrency:            10,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsBackupFileValidationFunc,
		IssueCode:      db.BackupFileDetectedCode,
	})
}
