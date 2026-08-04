package passive

import (
	"net/textproto"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestDeclaredHeaderNamesAreCanonical(t *testing.T) {
	for _, check := range getHeaderChecks() {
		for _, headerName := range check.Headers {
			canonical := textproto.CanonicalMIMEHeaderKey(headerName)
			if headerName != canonical {
				t.Errorf("check %q declares header %q, which is not in canonical MIME form (want %q)", check.IssueCode, headerName, canonical)
			}
		}
	}
}

func TestFingerprintHeaderChecksMatchRegardlessOfStoredCase(t *testing.T) {
	tests := []struct {
		name            string
		check           HeaderCheck
		headers         map[string][]string
		wantCode        db.IssueCode
		wantDescription string
	}{
		{
			name:            "asp.net version, canonicalised by DumpResponse",
			check:           xAspNetVersionHeaderCheck,
			headers:         map[string][]string{"X-Aspnet-Version": {"4.0.30319"}},
			wantCode:        db.XAspVersionHeaderCode,
			wantDescription: "Header 'X-Aspnet-Version' with value '4.0.30319' matches the condition 'exists' .\n",
		},
		{
			name:            "asp.net version, stored with the wire case",
			check:           xAspNetVersionHeaderCheck,
			headers:         map[string][]string{"X-AspNet-Version": {"4.0.30319"}},
			wantCode:        db.XAspVersionHeaderCode,
			wantDescription: "Header 'X-Aspnet-Version' with value '4.0.30319' matches the condition 'exists' .\n",
		},
		{
			name:            "asp.net mvc version, canonicalised by DumpResponse",
			check:           aspNetMvcHeaderCheck,
			headers:         map[string][]string{"X-Aspnetmvc-Version": {"5.2"}},
			wantCode:        db.AspNetMvcHeaderCode,
			wantDescription: "Header 'X-Aspnetmvc-Version' with value '5.2' matches the condition 'exists' .\n",
		},
		{
			name:            "asp.net mvc version, stored with the wire case",
			check:           aspNetMvcHeaderCheck,
			headers:         map[string][]string{"X-AspNetMvc-Version": {"5.2"}},
			wantCode:        db.AspNetMvcHeaderCode,
			wantDescription: "Header 'X-Aspnetmvc-Version' with value '5.2' matches the condition 'exists' .\n",
		},
		{
			name:            "x-xss-protection disabled, canonicalised by DumpResponse",
			check:           xXSSProtectionHeaderCheck,
			headers:         map[string][]string{"X-Xss-Protection": {"0"}},
			wantCode:        db.XXssProtectionHeaderCode,
			wantDescription: "Header 'X-Xss-Protection' with value '0' matches the condition 'not-equals' 1; mode=block.\n",
		},
		{
			name:            "x-xss-protection disabled, stored with the wire case",
			check:           xXSSProtectionHeaderCheck,
			headers:         map[string][]string{"X-XSS-Protection": {"0"}},
			wantCode:        db.XXssProtectionHeaderCode,
			wantDescription: "Header 'X-Xss-Protection' with value '0' matches the condition 'not-equals' 1; mode=block.\n",
		},
		{
			name:            "x-xss-protection disabled, stored lowercased",
			check:           xXSSProtectionHeaderCheck,
			headers:         map[string][]string{"x-xss-protection": {"0"}},
			wantCode:        db.XXssProtectionHeaderCode,
			wantDescription: "Header 'X-Xss-Protection' with value '0' matches the condition 'not-equals' 1; mode=block.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results := test.check.Check(test.headers)
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1: %+v", len(results), results)
			}
			if !results[0].Matched {
				t.Error("got unmatched result")
			}
			if results[0].IssueCode != test.wantCode {
				t.Errorf("got issue code %q, want %q", results[0].IssueCode, test.wantCode)
			}
			if results[0].Description != test.wantDescription {
				t.Errorf("got description %q, want %q", results[0].Description, test.wantDescription)
			}
		})
	}
}

func TestXXSSProtectionCorrectPolicyIsNotReported(t *testing.T) {
	results := xXSSProtectionHeaderCheck.Check(map[string][]string{"X-Xss-Protection": {"1; mode=block"}})
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0: %+v", len(results), results)
	}
}

func TestHeaderChecksOnStoredResponseHeaders(t *testing.T) {
	canonicalHeaders := map[string][]string{
		"Content-Type":              {"text/html; charset=utf-8"},
		"Server":                    {"Microsoft-IIS/10.0"},
		"X-Powered-By":              {"ASP.NET"},
		"X-Aspnet-Version":          {"4.0.30319"},
		"X-Aspnetmvc-Version":       {"5.2"},
		"X-Xss-Protection":          {"0"},
		"X-Frame-Options":           {"SAMEORIGIN"},
		"Cache-Control":             {"private"},
		"Strict-Transport-Security": {"max-age=31536000"},
	}
	wireCaseHeaders := map[string][]string{
		"Content-Type":              {"text/html; charset=utf-8"},
		"server":                    {"Microsoft-IIS/10.0"},
		"X-Powered-By":              {"ASP.NET"},
		"X-AspNet-Version":          {"4.0.30319"},
		"X-AspNetMvc-Version":       {"5.2"},
		"X-XSS-Protection":          {"0"},
		"X-Frame-Options":           {"SAMEORIGIN"},
		"Cache-Control":             {"private"},
		"Strict-Transport-Security": {"max-age=31536000"},
	}
	want := map[db.IssueCode]int{
		db.XPoweredByHeaderCode:   1,
		db.ServerHeaderCode:       1,
		db.CacheControlHeaderCode: 1,
		db.XAspVersionHeaderCode:  1,
		db.AspNetMvcHeaderCode:    1,
	}

	tests := []struct {
		name    string
		headers map[string][]string
	}{
		{name: "canonicalised by DumpResponse", headers: canonicalHeaders},
		{name: "stored with the wire case", headers: wireCaseHeaders},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make(map[db.IssueCode]int)
			for _, check := range getHeaderChecks() {
				for _, result := range check.Check(test.headers) {
					if result.Matched {
						got[result.IssueCode]++
					}
				}
			}

			for code, count := range want {
				if got[code] != count {
					t.Errorf("issue code %q: got %d results, want %d", code, got[code], count)
				}
			}
			for code, count := range got {
				if _, expected := want[code]; !expected {
					t.Errorf("unexpected issue code %q with %d results", code, count)
				}
			}
		})
	}
}
