package discovery

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzeCrossDomainPolicy(t *testing.T) {
	tests := []struct {
		name           string
		policy         *CrossDomainPolicy
		expectedIssues []string
		expectedLevel  string
	}{
		{
			name: "Allow all domains",
			policy: &CrossDomainPolicy{
				AllowAccess: []AllowAccess{{
					Domain: "*",
					Secure: "true",
				}},
			},
			expectedIssues: []string{"Policy allows access from any domain"},
			expectedLevel:  "High",
		},
		{
			name: "Allow all .com domains",
			policy: &CrossDomainPolicy{
				AllowAccess: []AllowAccess{{
					Domain: "*.com",
					Secure: "true",
				}},
			},
			expectedIssues: []string{"Policy allows access from all *.com domains"},
			expectedLevel:  "High",
		},
		{
			name: "Non-secure access",
			policy: &CrossDomainPolicy{
				AllowAccess: []AllowAccess{{
					Domain: "example.com",
					Secure: "false",
				}},
			},
			expectedIssues: []string{"Non-secure access allowed for domain: example.com"},
			expectedLevel:  "Low",
		},
		{
			name: "Wildcard subdomain",
			policy: &CrossDomainPolicy{
				AllowAccess: []AllowAccess{{
					Domain: "*.example.com",
					Secure: "true",
				}},
			},
			expectedIssues: []string{},
			expectedLevel:  "Info",
		},
		{
			name: "Allow all headers from specific domain",
			policy: &CrossDomainPolicy{
				AllowHeaders: []AllowHeader{{
					Domain:  "example.com",
					Headers: "*",
					Secure:  "true",
				}},
			},
			expectedIssues: []string{"All headers allowed from domain: example.com"},
			expectedLevel:  "Medium",
		},
		{
			name: "Sensitive headers allowed",
			policy: &CrossDomainPolicy{
				AllowHeaders: []AllowHeader{{
					Domain:  "example.com",
					Headers: "Authorization,X-Custom",
					Secure:  "true",
				}},
			},
			expectedIssues: []string{
				"Sensitive header authorization allowed from domain: example.com",
				"Sensitive header x-custom allowed from domain: example.com",
			},
			expectedLevel: "Medium",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues, level := analyzeCrossDomainPolicy(tc.policy)
			assert.ElementsMatch(t, tc.expectedIssues, issues)
			assert.Equal(t, tc.expectedLevel, level)
		})
	}
}

func TestIsFlashCrossDomainValidationFunc(t *testing.T) {
	tests := []struct {
		name             string
		history          *db.History
		expectValid      bool
		expectConfidence int
	}{
		{
			name: "Invalid status code",
			history: &db.History{
				StatusCode:          404,
				ResponseContentType: "text/xml",
				ResponseBody:        []byte("<cross-domain-policy></cross-domain-policy>"),
			},
			expectValid:      false,
			expectConfidence: 0,
		},
		{
			name: "Invalid content type",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				ResponseBody:        []byte("<cross-domain-policy></cross-domain-policy>"),
			},
			expectValid:      false,
			expectConfidence: 0,
		},
		{
			name: "Invalid XML content",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/xml",
				ResponseBody:        []byte("not xml content"),
			},
			expectValid:      false,
			expectConfidence: 0,
		},
		{
			name: "Valid secure policy",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/xml",
				URL:                 "https://example.com/crossdomain.xml",
				ResponseBody: []byte(`
					<?xml version="1.0"?>
					<cross-domain-policy>
						<allow-access-from domain="*.example.com" secure="true"/>
					</cross-domain-policy>
				`),
			},
			expectValid:      true,
			expectConfidence: 90,
		},
		{
			name: "High risk policy",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/xml",
				URL:                 "https://example.com/crossdomain.xml",
				ResponseBody: []byte(`
					<?xml version="1.0"?>
					<cross-domain-policy>
						<allow-access-from domain="*" secure="false"/>
						<allow-http-request-headers-from domain="*" headers="*"/>
					</cross-domain-policy>
				`),
			},
			expectValid:      true,
			expectConfidence: 90,
		},
		{
			name: "HTML content",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/html",
				URL:                 "https://example.com/crossdomain.xml",
				ResponseBody:        []byte("<html><body><h1>Hello, World!</h1></body></html>"),
			},
			expectValid:      false,
			expectConfidence: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			valid, details, confidence := IsFlashCrossDomainValidationFunc(tc.history)
			assert.Equal(t, tc.expectValid, valid)
			if valid {
				assert.NotEmpty(t, details)
				assert.Equal(t, tc.expectConfidence, confidence)
			}
		})
	}
}
