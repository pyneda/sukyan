package discovery

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestIsBackupFileValidationFunc(t *testing.T) {
	tests := []struct {
		name        string
		history     *db.History
		body        string
		shouldPass  bool
		description string
	}{
		{
			name: "PHP config backup with credentials",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/plain",
				URL:                 "https://example.com/config.php.bak",
			},
			body: `<?php
$db_host = 'localhost';
$db_user = 'admin';
$DB_PASSWORD = 'secret123';
$API_KEY = 'abcd1234';
`,
			shouldPass:  true,
			description: "PHP backup with sensitive indicators",
		},
		{
			name: "web.config backup",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/octet-stream",
				URL:                 "https://example.com/web.config.bak",
			},
			body: `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <connectionStrings>
    <add name="DefaultConnection" connectionString="Server=db;Database=app;password=secret" />
  </connectionStrings>
</configuration>`,
			shouldPass:  true,
			description: ".NET config backup",
		},
		{
			name: "htaccess backup with rewrite rules",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/plain",
				URL:                 "https://example.com/.htaccess.bak",
			},
			body: `RewriteEngine On
RewriteRule ^api/(.*)$ api.php?route=$1 [L,QSA]
`,
			shouldPass:  true,
			description: "htaccess backup with rules",
		},
		{
			name: "HTML error page (false positive)",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/html",
				URL:                 "https://example.com/config.bak",
			},
			body: `<!DOCTYPE html>
<html>
<head><title>Not Found</title></head>
<body>The requested file was not found.</body>
</html>`,
			shouldPass:  false,
			description: "HTML error page should be rejected",
		},
		{
			name: "Non-200 status code",
			history: &db.History{
				StatusCode:          404,
				ResponseContentType: "text/plain",
				URL:                 "https://example.com/config.php.bak",
			},
			body:        ``,
			shouldPass:  false,
			description: "404 should not pass",
		},
		{
			name: "Empty backup file with backup extension",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/octet-stream",
				URL:                 "https://example.com/settings.json.bak",
			},
			body:        `{}`,
			shouldPass:  true,
			description: "Empty JSON with backup extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.history.RawResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: " + tt.history.ResponseContentType + "\r\n\r\n" + tt.body)

			passed, details, confidence := IsBackupFileValidationFunc(tt.history, nil)

			if passed != tt.shouldPass {
				t.Errorf("%s: expected pass=%v, got pass=%v (confidence=%d)\nDetails: %s",
					tt.description, tt.shouldPass, passed, confidence, details)
			}
		})
	}
}

func TestBackupFilePaths(t *testing.T) {
	// Verify the paths list contains expected backup patterns
	expectedPatterns := []string{
		"web.config.bak",
		".htaccess.bak",
		"config.php.bak",
		"wp-config.php.bak",
		"database.yml.bak",
		"appsettings.json.bak",
		".env.bak",
	}

	pathSet := make(map[string]bool)
	for _, path := range BackupFilePaths {
		pathSet[path] = true
	}

	for _, expected := range expectedPatterns {
		if !pathSet[expected] {
			t.Errorf("Expected backup path %s not found in BackupFilePaths", expected)
		}
	}
}
