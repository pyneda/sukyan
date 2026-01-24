package discovery

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestIsAndroidAssetLinksValidationFunc(t *testing.T) {
	tests := []struct {
		name       string
		history    *db.History
		body       string
		shouldPass bool
	}{
		{
			name: "Valid assetlinks.json",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/assetlinks.json",
			},
			body: `[{
				"relation": ["delegate_permission/common.handle_all_urls"],
				"target": {
					"namespace": "android_app",
					"package_name": "com.example.app",
					"sha256_cert_fingerprints": ["AB:CD:EF:12:34:56"]
				}
			}]`,
			shouldPass: true,
		},
		{
			name: "Invalid JSON",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/assetlinks.json",
			},
			body:       `not valid json`,
			shouldPass: false,
		},
		{
			name: "Non-200 status code",
			history: &db.History{
				StatusCode:          404,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/assetlinks.json",
			},
			body:       `[]`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the ResponseBody method by setting RawResponse
			tt.history.RawResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n" + tt.body)

			passed, details, confidence := IsAndroidAssetLinksValidationFunc(tt.history, nil)

			if passed != tt.shouldPass {
				t.Errorf("Expected pass=%v, got pass=%v (confidence=%d, details=%s)",
					tt.shouldPass, passed, confidence, details)
			}

			if passed && confidence < 50 {
				t.Errorf("Expected confidence >= 50 for passing validation, got %d", confidence)
			}
		})
	}
}

func TestIsAppleAppSiteAssociationValidationFunc(t *testing.T) {
	tests := []struct {
		name       string
		history    *db.History
		body       string
		shouldPass bool
	}{
		{
			name: "Valid AASA with applinks",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/apple-app-site-association",
			},
			body: `{
				"applinks": {
					"apps": [],
					"details": [
						{
							"appID": "ABCDE12345.com.example.app",
							"paths": ["*"]
						}
					]
				}
			}`,
			shouldPass: true,
		},
		{
			name: "Valid AASA with webcredentials",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/apple-app-site-association",
			},
			body: `{
				"webcredentials": {
					"apps": ["ABCDE12345.com.example.app"]
				}
			}`,
			shouldPass: true,
		},
		{
			name: "Invalid JSON",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/.well-known/apple-app-site-association",
			},
			body:       `not valid json`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.history.RawResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n" + tt.body)

			passed, details, confidence := IsAppleAppSiteAssociationValidationFunc(tt.history, nil)

			if passed != tt.shouldPass {
				t.Errorf("Expected pass=%v, got pass=%v (confidence=%d, details=%s)",
					tt.shouldPass, passed, confidence, details)
			}

			if passed {
				if !strings.Contains(details, "Apple App Site Association") {
					t.Errorf("Details should mention Apple App Site Association")
				}
			}
		})
	}
}
