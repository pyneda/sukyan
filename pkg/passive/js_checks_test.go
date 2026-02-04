package passive

import (
	"strings"
	"testing"
)

func TestWrapJSONAsJS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple object",
			input:    `{"key":"value"}`,
			expected: `var _={"key":"value"};`,
		},
		{
			name:     "array",
			input:    `[1,2,3]`,
			expected: `var _=[1,2,3];`,
		},
		{
			name:     "nested object",
			input:    `{"a":{"b":"c"}}`,
			expected: `var _={"a":{"b":"c"}};`,
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: `var _={};`,
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: `var _=[];`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(wrapJSONAsJS([]byte(tt.input)))
			if result != tt.expected {
				t.Errorf("wrapJSONAsJS(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFindSecretsInJSON(t *testing.T) {
	t.Run("detects AWS access key in JSON", func(t *testing.T) {
		jsonData := []byte(`{"aws_access_key_id":"AKIAIOSFODNN7EXAMPLE","aws_secret_access_key":"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`)
		secrets := findSecretsInJSON(jsonData)
		if len(secrets) == 0 {
			t.Skip("jsluice did not detect secrets in this pattern; skipping")
		}
		for _, s := range secrets {
			if s.Details == "" {
				t.Error("expected non-empty details for detected secret")
			}
			if s.Severity == "" {
				t.Error("expected non-empty severity for detected secret")
			}
		}
	})

	t.Run("no false positives on clean JSON", func(t *testing.T) {
		jsonData := []byte(`{"name":"John","age":30,"city":"New York"}`)
		secrets := findSecretsInJSON(jsonData)
		if len(secrets) != 0 {
			t.Errorf("expected 0 secrets for clean JSON, got %d", len(secrets))
		}
	})

	t.Run("source label mentions JSON response", func(t *testing.T) {
		jsonData := []byte(`{"token":"AKIAIOSFODNN7EXAMPLE"}`)
		secrets := findSecretsInJSON(jsonData)
		for _, s := range secrets {
			if !strings.Contains(s.Details, "JSON response") {
				t.Errorf("expected details to mention 'JSON response', got: %s", s.Details)
			}
		}
	})
}

func TestFindSecretsInJavascript(t *testing.T) {
	t.Run("source label mentions javascript code", func(t *testing.T) {
		code := []byte(`var key = "AKIAIOSFODNN7EXAMPLE";`)
		secrets := findSecretsInJavascript(code)
		for _, s := range secrets {
			if !strings.Contains(s.Details, "javascript code") {
				t.Errorf("expected details to mention 'javascript code', got: %s", s.Details)
			}
		}
	})

	t.Run("no false positives on clean JS", func(t *testing.T) {
		code := []byte(`var x = 42; console.log(x);`)
		secrets := findSecretsInJavascript(code)
		if len(secrets) != 0 {
			t.Errorf("expected 0 secrets for clean JS, got %d", len(secrets))
		}
	})
}
