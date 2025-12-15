package integrations

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Version pattern replacement",
			input:    "jquery-(§§version§§)(.min)?\\.js",
			expected: "jquery-([0-9]+(?:\\.[0-9]+)*(?:[-_][a-zA-Z0-9.]+)*(?:\\+[a-zA-Z0-9.-]+)?)(.min)?\\.js",
		},
		{
			name:     "Large quantifier fix",
			input:    "pattern{0,9999}test",
			expected: "pattern{0,1000}test",
		},
		{
			name:     "Small quantifier preserved",
			input:    "pattern{0,50}test",
			expected: "pattern{0,50}test",
		},
		{
			name:     "Large quantifier fix type 2",
			input:    "pattern{1,5000}test",
			expected: "pattern{1,1000}test",
		},
		{
			name:     "Complex pattern with version and quantifiers",
			input:    "/\\*!? jQuery v(§§version§§){0,10000}",
			expected: "/\\*!? jQuery v([0-9]+(?:\\.[0-9]+)*(?:[-_][a-zA-Z0-9.]+)*(?:\\+[a-zA-Z0-9.-]+)?){0,1000}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixPattern(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidVulnerability(t *testing.T) {
	tests := []struct {
		name     string
		vuln     Vulnerability
		expected bool
	}{
		{
			name: "Valid vulnerability with below version",
			vuln: Vulnerability{
				Below: "1.6.3",
			},
			expected: true,
		},
		{
			name: "Valid vulnerability with atOrAbove version",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: true,
		},
		{
			name: "Invalid vulnerability with no version constraints",
			vuln: Vulnerability{
				Severity: "high",
			},
			expected: false,
		},
		{
			name: "Invalid vulnerability with invalid below version",
			vuln: Vulnerability{
				Below: "invalid-version",
			},
			expected: false,
		},
		{
			name: "Invalid vulnerability with invalid atOrAbove version",
			vuln: Vulnerability{
				AtOrAbove: "not-a-version",
				Below:     "1.6.3",
			},
			expected: false,
		},
		{
			name: "Valid vulnerability with only atOrAbove",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
			},
			expected: true,
		},
		{
			name: "Valid vulnerability with pre-release version",
			vuln: Vulnerability{
				Below: "1.0.0-beta1",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidVulnerability(tt.vuln)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVersionVulnerable(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		vuln     Vulnerability
		expected bool
	}{
		{
			name:    "Version vulnerable - below only",
			version: "1.5.0",
			vuln: Vulnerability{
				Below: "1.6.3",
			},
			expected: true,
		},
		{
			name:    "Version not vulnerable - above fix",
			version: "1.7.0",
			vuln: Vulnerability{
				Below: "1.6.3",
			},
			expected: false,
		},
		{
			name:    "Version vulnerable - in range",
			version: "1.5.0",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: true,
		},
		{
			name:    "Version not vulnerable - below range",
			version: "1.1.0",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: false,
		},
		{
			name:    "Version not vulnerable - above range",
			version: "1.7.0",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: false,
		},
		{
			name:    "Edge case - exact atOrAbove version",
			version: "1.2.0",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: true,
		},
		{
			name:    "Edge case - exact below version",
			version: "1.6.3",
			vuln: Vulnerability{
				AtOrAbove: "1.2.0",
				Below:     "1.6.3",
			},
			expected: false,
		},
		{
			name:    "Semantic version comparison - 1.10.0 vs 1.2.0",
			version: "1.10.0",
			vuln: Vulnerability{
				Below: "1.2.0",
			},
			expected: false, // 1.10.0 is NOT less than 1.2.0
		},
		{
			name:    "Semantic version comparison - 1.1.5 vs 1.2.0",
			version: "1.1.5",
			vuln: Vulnerability{
				Below: "1.2.0",
			},
			expected: true, // 1.1.5 is less than 1.2.0
		},
		{
			name:    "Pre-release version handling",
			version: "2.0.0-beta.1",
			vuln: Vulnerability{
				Below: "2.0.0",
			},
			expected: true,
		},
		{
			name:    "Invalid version string",
			version: "not-a-version",
			vuln: Vulnerability{
				Below: "1.6.3",
			},
			expected: false,
		},
		{
			name:    "Invalid vulnerability",
			version: "1.5.0",
			vuln:    Vulnerability{
				// No version constraints
			},
			expected: false,
		},
		{
			name:    "Complex version with metadata",
			version: "1.5.0+build.123",
			vuln: Vulnerability{
				Below: "1.6.0",
			},
			expected: true,
		},
		{
			name:    "Major version difference",
			version: "2.0.0",
			vuln: Vulnerability{
				Below: "1.9.9",
			},
			expected: false,
		},
		{
			name:    "Only atOrAbove constraint - vulnerable",
			version: "1.5.0",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
			},
			expected: true,
		},
		{
			name:    "Only atOrAbove constraint - not vulnerable",
			version: "0.9.0",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionVulnerable(tt.version, tt.vuln)
			assert.Equal(t, tt.expected, result, "Test case: %s", tt.name)
		})
	}
}

func TestLoadRetireJsRepo(t *testing.T) {
	repo, err := loadRetireJsRepo()
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	// Check that some known libraries are present
	assert.Contains(t, repo, "jquery")
	assert.Contains(t, repo, "angularjs")

	// Verify jQuery has expected structure
	jqueryEntry, exists := repo["jquery"]
	assert.True(t, exists)
	assert.NotEmpty(t, jqueryEntry.Vulnerabilities)
	assert.NotEmpty(t, jqueryEntry.Extractors.Filename)
	assert.NotEmpty(t, jqueryEntry.Extractors.Filecontent)
}

func TestNewRetireScanner(t *testing.T) {
	scanner := NewRetireScanner()
	assert.NotNil(t, scanner)
	assert.NotNil(t, scanner.repo)
	assert.Contains(t, scanner.repo, "jquery")
}

// Benchmark tests for performance
func BenchmarkFixPattern(b *testing.B) {
	pattern := "jquery-(§§version§§)(.min)?\\.js{0,9999}"
	for i := 0; i < b.N; i++ {
		fixPattern(pattern)
	}
}

func BenchmarkIsVersionVulnerable(b *testing.B) {
	vuln := Vulnerability{
		AtOrAbove: "1.2.0",
		Below:     "1.6.3",
	}
	version := "1.5.0"

	for i := 0; i < b.N; i++ {
		isVersionVulnerable(version, vuln)
	}
}

func BenchmarkExtractVersionFromMatch(b *testing.B) {
	pattern := "/\\*!? jQuery v(§§version§§)"
	content := "/*! jQuery v3.7.1 | (c) OpenJS Foundation and other contributors | jquery.org/license */"

	for i := 0; i < b.N; i++ {
		extractVersionFromMatch(pattern, content)
	}
}

func TestVersionComparisonEdgeCases(t *testing.T) {
	// These test cases specifically address the issues that could cause false positives
	tests := []struct {
		name     string
		version  string
		vuln     Vulnerability
		expected bool
		reason   string
	}{
		{
			name:    "String vs semantic comparison - 1.10.0 should not be vulnerable to <1.2.0",
			version: "1.10.0",
			vuln: Vulnerability{
				Below: "1.2.0",
			},
			expected: false,
			reason:   "1.10.0 is semantically greater than 1.2.0",
		},
		{
			name:    "String vs semantic comparison - 2.0.0 should not be vulnerable to <10.0.0",
			version: "2.0.0",
			vuln: Vulnerability{
				Below: "10.0.0",
			},
			expected: true,
			reason:   "2.0.0 is semantically less than 10.0.0",
		},
		{
			name:    "String vs semantic comparison - 11.0.0 should not be vulnerable to <2.0.0",
			version: "11.0.0",
			vuln: Vulnerability{
				Below: "2.0.0",
			},
			expected: false,
			reason:   "11.0.0 is semantically greater than 2.0.0",
		},
		{
			name:    "Version with many digits",
			version: "1.12.15",
			vuln: Vulnerability{
				Below: "1.9.0",
			},
			expected: false,
			reason:   "1.12.15 is semantically greater than 1.9.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionVulnerable(tt.version, tt.vuln)
			assert.Equal(t, tt.expected, result, "Test case: %s - %s", tt.name, tt.reason)
		})
	}
}

// Integration tests to demonstrate false positive prevention
func TestFalsePositivePrevention(t *testing.T) {
	scanner := NewRetireScanner()

	// Test with actual jQuery vulnerabilities from the repository
	jqueryEntry, exists := scanner.repo["jquery"]
	assert.True(t, exists, "jQuery should exist in repository")

	// Find a vulnerability that uses version ranges
	var testVuln Vulnerability
	for _, vuln := range jqueryEntry.Vulnerabilities {
		if vuln.Below != "" && vuln.AtOrAbove != "" {
			testVuln = vuln
			break
		}
	}

	if testVuln.Below != "" {
		t.Run("Real jQuery vulnerability test", func(t *testing.T) {
			// Test that a version clearly above the range is not vulnerable
			result := isVersionVulnerable("10.0.0", testVuln)
			assert.False(t, result, "Version 10.0.0 should not be vulnerable to %+v", testVuln)

			// Test that an invalid version string doesn't cause issues
			result = isVersionVulnerable("not-a-version", testVuln)
			assert.False(t, result, "Invalid version should not be considered vulnerable")
		})
	}
}

func TestPatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		url      string
		expected bool
	}{
		{
			name:     "jQuery filename pattern match",
			pattern:  "jquery-(§§version§§)(.min)?\\.js",
			url:      "https://example.com/js/jquery-1.12.4.min.js",
			expected: true,
		},
		{
			name:     "jQuery filename pattern no match",
			pattern:  "jquery-(§§version§§)(.min)?\\.js",
			url:      "https://example.com/js/bootstrap-3.3.7.min.js",
			expected: false,
		},
		{
			name:     "Version pattern with complex version",
			pattern:  "library-(§§version§§).js",
			url:      "https://cdn.com/library-1.2.3-beta.1+build.456.js",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixedPattern := fixPattern(tt.pattern)
			match, err := regexp.MatchString(fixedPattern, tt.url)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// Test to ensure we handle edge cases in the JSON data properly
func TestRepositoryDataIntegrity(t *testing.T) {
	scanner := NewRetireScanner()

	invalidVulnCount := 0
	totalVulnCount := 0

	for libraryName, entry := range scanner.repo {
		for _, vuln := range entry.Vulnerabilities {
			totalVulnCount++
			if !isValidVulnerability(vuln) {
				invalidVulnCount++
				t.Logf("Invalid vulnerability in %s: %+v", libraryName, vuln)
			}
		}
	}

	t.Logf("Total vulnerabilities: %d", totalVulnCount)
	t.Logf("Invalid vulnerabilities: %d", invalidVulnCount)

	// We expect some invalid vulnerabilities in the data, but not too many
	invalidRatio := float64(invalidVulnCount) / float64(totalVulnCount)
	assert.Less(t, invalidRatio, 0.5, "Too many invalid vulnerabilities in repository")
}

func TestExtractVersionFromMatch(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		content  string
		expected string
	}{
		{
			name:     "jQuery version extraction from comment",
			pattern:  "/\\*!? jQuery v(§§version§§)",
			content:  "/*! jQuery v3.7.1 | (c) OpenJS Foundation and other contributors | jquery.org/license */",
			expected: "3.7.1",
		},
		{
			name:     "jQuery version extraction from filename",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			content:  "https://example.com/js/jquery-1.12.4.min.js",
			expected: "1.12.4",
		},
		{
			name:     "Version with pre-release",
			pattern:  "/\\*!? Library v(§§version§§)",
			content:  "/*! Library v2.0.0-beta.1 */",
			expected: "2.0.0-beta.1",
		},
		{
			name:     "No version match",
			pattern:  "/\\*!? jQuery v(§§version§§)",
			content:  "/* Some other comment */",
			expected: "",
		},
		{
			name:     "Multiple versions - returns first",
			pattern:  "version: (§§version§§)",
			content:  "version: 1.2.3, backup version: 1.2.2",
			expected: "1.2.3",
		},
		{
			name:     "Complex pattern with large quantifiers",
			pattern:  "jquery.{0,9999}version.*?(§§version§§)",
			content:  "jquery large content here version: 1.5.0",
			expected: "1.5.0",
		},
		{
			name:     "Version with build metadata",
			pattern:  "lib-(§§version§§)\\.js",
			content:  "lib-1.2.3+build.456.js",
			expected: "1.2.3+build.456",
		},
		{
			name:     "Min suffix stripped from version",
			pattern:  "lib-(§§version§§)(\\.min)?\\.js",
			content:  "lib-1.2.3.min.js",
			expected: "1.2.3",
		},
		{
			name:     "Dash min suffix stripped from version",
			pattern:  "lib-(§§version§§)(-min)?\\.js",
			content:  "lib-1.2.3-min.js",
			expected: "1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromMatch(tt.pattern, tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractVersionFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected string
	}{
		{
			name:     "Simple filename",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "jquery-1.8.1.js",
			expected: "1.8.1",
		},
		{
			name:     "Linux path",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "/usr/file/jquery-1.8.1.js",
			expected: "1.8.1",
		},
		{
			name:     "Windows path",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "\\usr\\file\\jquery-1.8.1.js",
			expected: "1.8.1",
		},
		{
			name:     "URL path",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "https://example.com/js/jquery-1.12.4.min.js",
			expected: "1.12.4",
		},
		{
			name:     "No match - different library",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "/usr/file/bootstrap-3.3.7.js",
			expected: "",
		},
		{
			name:     "Min suffix stripped",
			pattern:  "jquery-(§§version§§)(\\.min)?\\.js",
			path:     "https://ajax.googleapis.com/lib/jquery-3.5.0.min.js",
			expected: "3.5.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromFilename(tt.pattern, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractVersionFromReplace(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		content  string
		expected string
	}{
		{
			name:     "Simple replacement",
			pattern:  "/version\\s*=\\s*['\"]([0-9.]+)['\"]/version=$1/",
			content:  "version = '1.2.3'",
			expected: "version=1.2.3",
		},
		{
			name:     "No match",
			pattern:  "/version\\s*=\\s*['\"]([0-9.]+)['\"]/version=$1/",
			content:  "no version here",
			expected: "",
		},
		{
			name:     "Invalid pattern format",
			pattern:  "not a valid pattern",
			content:  "some content",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromReplace(tt.pattern, tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Windows line endings",
			input:    "line1\r\nline2\r\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "Old Mac line endings",
			input:    "line1\rline2\rline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "Unix line endings unchanged",
			input:    "line1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "Mixed line endings",
			input:    "line1\r\nline2\rline3\nline4",
			expected: "line1\nline2\nline3\nline4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeContent([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestExcludesSupport(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		vuln     Vulnerability
		expected bool
	}{
		{
			name:    "Version in excludes list",
			version: "1.12.4-aem",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
				Below:     "3.0.0",
				Excludes:  []string{"1.12.4-aem"},
			},
			expected: false,
		},
		{
			name:    "Similar version not in excludes",
			version: "1.12.4",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
				Below:     "3.0.0",
				Excludes:  []string{"1.12.4-aem"},
			},
			expected: true,
		},
		{
			name:    "Multiple excludes - version matches one",
			version: "2.0.0-patched",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
				Below:     "3.0.0",
				Excludes:  []string{"1.12.4-aem", "2.0.0-patched", "2.5.0-security"},
			},
			expected: false,
		},
		{
			name:    "Empty excludes list",
			version: "1.5.0",
			vuln: Vulnerability{
				AtOrAbove: "1.0.0",
				Below:     "2.0.0",
				Excludes:  []string{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionVulnerable(tt.version, tt.vuln)
			assert.Equal(t, tt.expected, result, "Test case: %s", tt.name)
		})
	}
}
