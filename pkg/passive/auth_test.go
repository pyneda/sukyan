package passive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractRealm(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "Basic with quoted realm",
			header:   `Basic realm="Login Required"`,
			expected: "Login Required",
		},
		{
			name:     "Basic with single quoted realm",
			header:   `Basic realm='Private Area'`,
			expected: "Private Area",
		},
		{
			name:     "Basic with unquoted realm",
			header:   `Basic realm=LoginRequired`,
			expected: "LoginRequired",
		},
		{
			name:     "Basic with no realm",
			header:   `Basic`,
			expected: "",
		},
		{
			name:     "With additional parameters",
			header:   `Basic realm="Secure Zone", charset="UTF-8"`,
			expected: "Secure Zone",
		},
		{
			name:     "With spaces around equals",
			header:   `Basic realm = "Admin Area"`,
			expected: "Admin Area",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := extractRealm(test.header)
			assert.Equal(t, test.expected, result, "The extracted realm should match the expected value")
		})
	}
}

func TestExtractAuthType(t *testing.T) {
	testCases := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "Basic auth simple",
			header:   "Basic realm=\"test\"",
			expected: "Basic",
		},
		{
			name:     "Basic auth with space",
			header:   "Basic  realm=\"test\"",
			expected: "Basic",
		},
		{
			name:     "Digest auth",
			header:   "Digest realm=\"testrealm@host.com\", qop=\"auth,auth-int\", nonce=\"dcd98b7102dd2f0e8b11d0f600bfb0c093\"",
			expected: "Digest",
		},
		{
			name:     "Bearer auth",
			header:   "Bearer realm=\"example\", error=\"invalid_token\"",
			expected: "Bearer",
		},
		{
			name:     "NTLM auth empty",
			header:   "NTLM",
			expected: "NTLM",
		},
		{
			name:     "NTLM auth with data",
			header:   "NTLM TlRMTVNTUAABAAAAB4IIogAAAAAAAAAAAAAAAAAAAAAGAbEdAAAADw==",
			expected: "NTLM",
		},
		{
			name:     "Negotiate auth",
			header:   "Negotiate",
			expected: "Negotiate",
		},
		{
			name:     "Mutual auth",
			header:   "Mutual realm=\"example.org\", algorithm=iso.9798.3.4.1.1.1",
			expected: "Mutual",
		},
		{
			name:     "Custom auth",
			header:   "CustomAuth method=token, id=12345",
			expected: "CustomAuth",
		},
		{
			name:     "Malformed auth header",
			header:   ":",
			expected: ":",
		},
		{
			name:     "Empty auth header",
			header:   "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractAuthType(tc.header)
			if result != tc.expected {
				t.Errorf("extractAuthType(%q) = %q, want %q", tc.header, result, tc.expected)
			}
		})
	}
}
