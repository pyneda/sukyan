package passive

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func assertMatchResults(t *testing.T, got []MatchResult, want []MatchResult) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].IssueCode != want[i].IssueCode {
			t.Errorf("result %d: got issue code %q, want %q", i, got[i].IssueCode, want[i].IssueCode)
		}
		if !got[i].Matched {
			t.Errorf("result %d: got unmatched result", i)
		}
		if got[i].Description != want[i].Description {
			t.Errorf("result %d: got description %q, want %q", i, got[i].Description, want[i].Description)
		}
	}
}

func TestXFrameOptionsHeaderCheckHonoursAndCondition(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]string
		want    []MatchResult
	}{
		{
			name:    "DENY is a correct policy and must not be reported",
			headers: map[string][]string{"X-Frame-Options": {"DENY"}},
			want:    nil,
		},
		{
			name:    "SAMEORIGIN is a correct policy and must not be reported",
			headers: map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}},
			want:    nil,
		},
		{
			name:    "ALLOWALL is reported once",
			headers: map[string][]string{"X-Frame-Options": {"ALLOWALL"}},
			want: []MatchResult{
				{
					IssueCode: db.XFrameOptionsHeaderCode,
					Description: "Header 'X-Frame-Options' with value 'ALLOWALL' matches the condition 'not-equals' DENY.\n" +
						"Header 'X-Frame-Options' with value 'ALLOWALL' matches the condition 'not-equals' SAMEORIGIN.\n",
				},
			},
		},
		{
			name:    "ALLOW-FROM is reported once",
			headers: map[string][]string{"X-Frame-Options": {"ALLOW-FROM https://evil.example"}},
			want: []MatchResult{
				{
					IssueCode: db.XFrameOptionsHeaderCode,
					Description: "Header 'X-Frame-Options' with value 'ALLOW-FROM https://evil.example' matches the condition 'not-equals' DENY.\n" +
						"Header 'X-Frame-Options' with value 'ALLOW-FROM https://evil.example' matches the condition 'not-equals' SAMEORIGIN.\n",
				},
			},
		},
		{
			name:    "only the bad value of a repeated header is reported",
			headers: map[string][]string{"X-Frame-Options": {"DENY", "ALLOWALL"}},
			want: []MatchResult{
				{
					IssueCode: db.XFrameOptionsHeaderCode,
					Description: "Header 'X-Frame-Options' with value 'ALLOWALL' matches the condition 'not-equals' DENY.\n" +
						"Header 'X-Frame-Options' with value 'ALLOWALL' matches the condition 'not-equals' SAMEORIGIN.\n",
				},
			},
		},
		{
			name:    "absent header is not reported",
			headers: map[string][]string{"Server": {"nginx/1.14.0"}},
			want:    nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := xFrameOptionsHeaderCheck
			assertMatchResults(t, check.Check(test.headers), test.want)
		})
	}
}

func TestHeaderChecksOutputUnchanged(t *testing.T) {
	tests := []struct {
		name    string
		check   HeaderCheck
		headers map[string][]string
		want    []MatchResult
	}{
		{
			name:    "x-powered-by present",
			check:   xPoweredByHeaderCheck,
			headers: map[string][]string{"X-Powered-By": {"PHP/8.1.2"}},
			want: []MatchResult{
				{IssueCode: db.XPoweredByHeaderCode, Description: "Header 'X-Powered-By' with value 'PHP/8.1.2' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "x-powered-by absent",
			check:   xPoweredByHeaderCheck,
			headers: map[string][]string{"Server": {"nginx/1.14.0"}},
			want:    nil,
		},
		{
			name:    "x-aspnet-version present",
			check:   xAspNetVersionHeaderCheck,
			headers: map[string][]string{"X-Aspnet-Version": {"4.0.30319"}},
			want: []MatchResult{
				{IssueCode: db.XAspVersionHeaderCode, Description: "Header 'X-Aspnet-Version' with value '4.0.30319' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "server header jetty emits both the specific and the generic code",
			check:   serverHeaderCheck,
			headers: map[string][]string{"Server": {"Jetty.9.4.1"}},
			want: []MatchResult{
				{IssueCode: db.JettyServerHeaderCode, Description: "Header 'Server' with value 'Jetty.9.4.1' matches the condition 'regex' Jetty\\.([\\d\\.]+).\n"},
				{IssueCode: db.ServerHeaderCode, Description: "Header 'Server' with value 'Jetty.9.4.1' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "server header java emits both the specific and the generic code",
			check:   serverHeaderCheck,
			headers: map[string][]string{"Server": {"java/1.8.0_292"}},
			want: []MatchResult{
				{IssueCode: db.JavaServerHeaderCode, Description: "Header 'Server' with value 'java/1.8.0_292' matches the condition 'regex' java\\/([\\d\\.\\_]+).\n"},
				{IssueCode: db.ServerHeaderCode, Description: "Header 'Server' with value 'java/1.8.0_292' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "server header plain value emits only the generic code",
			check:   serverHeaderCheck,
			headers: map[string][]string{"Server": {"nginx/1.14.0"}},
			want: []MatchResult{
				{IssueCode: db.ServerHeaderCode, Description: "Header 'Server' with value 'nginx/1.14.0' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "missing content-type reported when absent",
			check:   missingContentTypeHeaderCheck,
			headers: map[string][]string{"Server": {"nginx/1.14.0"}},
			want: []MatchResult{
				{IssueCode: db.MissingContentTypeHeaderCode, Description: "Header 'Content-Type' does not exist as expected.\n"},
			},
		},
		{
			name:    "missing content-type not reported when present",
			check:   missingContentTypeHeaderCheck,
			headers: map[string][]string{"Content-Type": {"text/html; charset=UTF-8"}},
			want:    nil,
		},
		{
			name:    "cache-control no-store",
			check:   cacheControlHeaderCheck,
			headers: map[string][]string{"Cache-Control": {"no-store"}},
			want: []MatchResult{
				{IssueCode: db.CacheControlHeaderCode, Description: "Header 'Cache-Control' with value 'no-store' matches the condition 'contains' no-store.\n"},
			},
		},
		{
			name:    "cache-control private",
			check:   cacheControlHeaderCheck,
			headers: map[string][]string{"Cache-Control": {"private"}},
			want: []MatchResult{
				{IssueCode: db.CacheControlHeaderCode, Description: "Header 'Cache-Control' with value 'private' matches the condition 'contains' private.\n"},
			},
		},
		{
			name:    "cache-control matching both matchers still emits both results",
			check:   cacheControlHeaderCheck,
			headers: map[string][]string{"Cache-Control": {"private, no-store"}},
			want: []MatchResult{
				{IssueCode: db.CacheControlHeaderCode, Description: "Header 'Cache-Control' with value 'private, no-store' matches the condition 'contains' no-store.\n"},
				{IssueCode: db.CacheControlHeaderCode, Description: "Header 'Cache-Control' with value 'private, no-store' matches the condition 'contains' private.\n"},
			},
		},
		{
			name:    "cache-control public",
			check:   cacheControlHeaderCheck,
			headers: map[string][]string{"Cache-Control": {"public, max-age=3600"}},
			want:    nil,
		},
		{
			name:    "hsts matching the regex",
			check:   strictTransportSecurityHeaderCheck,
			headers: map[string][]string{"Strict-Transport-Security": {"max-age=31536000; includeSubDomains; preload"}},
			want: []MatchResult{
				{IssueCode: db.StrictTransportSecurityHeaderCode, Description: "Header 'Strict-Transport-Security' with value 'max-age=31536000; includeSubDomains; preload' matches the condition 'regex' ^max-age=\\d+; includeSubDomains; preload$.\n"},
			},
		},
		{
			name:    "hsts not matching the regex",
			check:   strictTransportSecurityHeaderCheck,
			headers: map[string][]string{"Strict-Transport-Security": {"max-age=1"}},
			want:    nil,
		},
		{
			name:    "x-xss-protection disabled",
			check:   xXSSProtectionHeaderCheck,
			headers: map[string][]string{"X-Xss-Protection": {"0"}},
			want: []MatchResult{
				{IssueCode: db.XXssProtectionHeaderCode, Description: "Header 'X-Xss-Protection' with value '0' matches the condition 'not-equals' 1; mode=block.\n"},
			},
		},
		{
			name:    "x-xss-protection enabled",
			check:   xXSSProtectionHeaderCheck,
			headers: map[string][]string{"X-Xss-Protection": {"1; mode=block"}},
			want:    nil,
		},
		{
			name:    "asp.net mvc version present",
			check:   aspNetMvcHeaderCheck,
			headers: map[string][]string{"X-Aspnetmvc-Version": {"5.2"}},
			want: []MatchResult{
				{IssueCode: db.AspNetMvcHeaderCode, Description: "Header 'X-Aspnetmvc-Version' with value '5.2' matches the condition 'exists' .\n"},
			},
		},
		{
			name:    "esi surrogate control",
			check:   esiDetectionHeaderCheck,
			headers: map[string][]string{"Surrogate-Control": {"content=\"ESI/1.0\""}},
			want: []MatchResult{
				{IssueCode: db.EsiDetectedCode, Description: "Header 'Surrogate-Control' with value 'content=\"ESI/1.0\"' matches the condition 'equals' content=\"ESI/1.0\".\n"},
			},
		},
		{
			name:    "esi surrogate control with an unrelated value",
			check:   esiDetectionHeaderCheck,
			headers: map[string][]string{"Surrogate-Control": {"max-age=60"}},
			want:    nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := test.check
			assertMatchResults(t, check.Check(test.headers), test.want)
		})
	}
}

func TestHeaderCheckMatchConditionSemantics(t *testing.T) {
	andWithCustomCodes := HeaderCheck{
		Headers: []string{"X-Test"},
		Matchers: []HeaderCheckMatcher{
			{MatcherType: Contains, Value: "alpha", CustomIssueCode: db.JettyServerHeaderCode},
			{MatcherType: Contains, Value: "beta", CustomIssueCode: db.JavaServerHeaderCode},
		},
		MatchCondition: And,
		IssueCode:      db.ServerHeaderCode,
	}
	orWithCustomCodes := andWithCustomCodes
	orWithCustomCodes.MatchCondition = Or

	tests := []struct {
		name    string
		check   HeaderCheck
		headers map[string][]string
		want    []MatchResult
	}{
		{
			name:    "and requires every matcher",
			check:   andWithCustomCodes,
			headers: map[string][]string{"X-Test": {"alpha"}},
			want:    nil,
		},
		{
			name:    "and reports once with the first custom issue code",
			check:   andWithCustomCodes,
			headers: map[string][]string{"X-Test": {"alpha beta"}},
			want: []MatchResult{
				{
					IssueCode: db.JettyServerHeaderCode,
					Description: "Header 'X-Test' with value 'alpha beta' matches the condition 'contains' alpha.\n" +
						"Header 'X-Test' with value 'alpha beta' matches the condition 'contains' beta.\n",
				},
			},
		},
		{
			name:    "or reports every matcher independently",
			check:   orWithCustomCodes,
			headers: map[string][]string{"X-Test": {"alpha beta"}},
			want: []MatchResult{
				{IssueCode: db.JettyServerHeaderCode, Description: "Header 'X-Test' with value 'alpha beta' matches the condition 'contains' alpha.\n"},
				{IssueCode: db.JavaServerHeaderCode, Description: "Header 'X-Test' with value 'alpha beta' matches the condition 'contains' beta.\n"},
			},
		},
		{
			name:    "unset condition keeps the or behaviour",
			check:   HeaderCheck{Headers: []string{"X-Test"}, Matchers: andWithCustomCodes.Matchers, IssueCode: db.ServerHeaderCode},
			headers: map[string][]string{"X-Test": {"alpha beta"}},
			want: []MatchResult{
				{IssueCode: db.JettyServerHeaderCode, Description: "Header 'X-Test' with value 'alpha beta' matches the condition 'contains' alpha.\n"},
				{IssueCode: db.JavaServerHeaderCode, Description: "Header 'X-Test' with value 'alpha beta' matches the condition 'contains' beta.\n"},
			},
		},
		{
			name:    "and falls back to the check issue code",
			check:   HeaderCheck{Headers: []string{"X-Test"}, Matchers: []HeaderCheckMatcher{{MatcherType: Contains, Value: "alpha"}}, MatchCondition: And, IssueCode: db.ServerHeaderCode},
			headers: map[string][]string{"X-Test": {"alpha"}},
			want: []MatchResult{
				{IssueCode: db.ServerHeaderCode, Description: "Header 'X-Test' with value 'alpha' matches the condition 'contains' alpha.\n"},
			},
		},
		{
			name:    "and with no matchers reports nothing",
			check:   HeaderCheck{Headers: []string{"X-Test"}, MatchCondition: And, IssueCode: db.ServerHeaderCode},
			headers: map[string][]string{"X-Test": {"alpha"}},
			want:    nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := test.check
			assertMatchResults(t, check.Check(test.headers), test.want)
		})
	}
}
