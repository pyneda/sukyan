package passive

import (
	"regexp"
	"testing"
)

func TestSessionTokenRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		// Test cases with inputs that should match the regex
		{"https://example.com/?session_token=abc123", true},
		{"https://example.com/?auth_token=xyz987", true},
		{"https://example.com/?api_key=123456", true},
		{"https://example.com/?id_token=789xyz", true},
		{"https://example.com/?token=789xyz", true},
		{"https://example.com/?session_token=abc123&page=1", true},
		{"https://example.com/?session_token=abc123&access_token=xyz987", true},
		{"https://example.com/?session_cookie=xyz987", true},
		{"https://example.com/?tokenid=123456", true},
		{"https://example.com/?access_token=abcd", true},
		{"https://example.com/?session_tokenid=abc123", true},
		{"https://example.com/?jwt=xyz.xyz", true},
		{"https://example.com/?first=1&second=2&token=xyz.xyz", true},
		{"https://example.com/?authentication_token=abc123", true},
		{"https://example.com/?auth_key=abc123", true},
		{"https://example.com/?auth-code=abc123", true},
		{"https://example.com/?authcode=abc123", true},
		{"https://example.com/?session-key=abc123", true},
		{"https://example.com/?sessionkey=abc123", true},
		{"https://example.com/?auth_KEY=abc123", true},
		{"https://example.com/?page=1&session_token=abc123", true},
		{"https://example.com/?pagesize=10&session_token=abc123", true},
		// Test cases with inputs that should not match the regex
		{"https://example.com/?not_token=123456", false},
		{"https://example.com/", false},
		{"https://example.com/?page=1&pagesize=10", false},
		{"https://example.com/?csrf_token=asdfasf", false},
		{"https://example.com/?session_token", false},
		{"https://example.com/?session_token=", false},
		{"https://example.com/?=abc123", false},
	}

	for _, tc := range testCases {
		match := sessionTokenRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestEmailRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"example@example.com", true},
		{"abc123@gmail.com", true},
		{"first.last@domain.io", true},
		{"special_chars+%.-@example.co.uk", true},
		{"invalid_email.com", false},
		{"missing@sign", false},
		{"@noLocalPart.com", false},
		{"missingDomain@.com", false},
	}

	for _, tc := range testCases {
		match := emailRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestPrivateIPRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},

		{"172.32.0.1", false},
		{"192.169.1.1", false},
		{"256.0.0.1", false},
		{"10.0.0.256", false},
		{"192.168.1.500", false},
	}

	for _, tc := range testCases {
		match := privateIPRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestFileUploadRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"<input type='file'>", true},
		{"<input type=\"file\">", true},
		{"<input type=FILE>", true},
		{"<input type='file' id='upload'>", true},
		{"<input type='file' id='upload'/>", true},

		{"<input type='text'>", false},
		{"<input type=\"submit\">", false},
		{"<input>", false},
		{"<input type='file", false},
	}

	for _, tc := range testCases {
		match := fileUploadRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestPrivateKeyRegexes(t *testing.T) {
	testCases := []struct {
		regex    *regexp.Regexp
		input    string
		expected bool
	}{
		{rsaPrivateKeyRegex, "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKqVkA==\n-----END RSA PRIVATE KEY-----", true},
		{dsaPrivateKeyRegex, "-----BEGIN DSA PRIVATE KEY-----\nMIIBvAIBAAKBgQCqVkA==\n-----END DSA PRIVATE KEY-----", true},
		{ecPrivateKeyRegex, "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIKqVkA==\n-----END EC PRIVATE KEY-----", true},
		{opensshPrivateKeyRegex, "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ==\n-----END OPENSSH PRIVATE KEY-----", true},
		{pemPrivateKeyRegex, "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEAqpWQA==\n-----END PRIVATE KEY-----", true},

		// Negative cases
		{rsaPrivateKeyRegex, "-----BEGIN RSA PUBLIC KEY-----\nMIIBOgIBAAJBAKqVkA==\n-----END RSA PUBLIC KEY-----", false},
	}

	for _, tc := range testCases {
		match := tc.regex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}
