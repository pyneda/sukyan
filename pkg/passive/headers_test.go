package passive

import (
	"testing"
)

func TestHeaderCheckMatcher(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		matcher     HeaderCheckMatcher
		expectMatch bool
	}{
		{
			name:        "Exists match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: Exists},
			expectMatch: true,
		},
		{
			name:        "Exists no match",
			headerValue: "",
			matcher:     HeaderCheckMatcher{MatcherType: Exists},
			expectMatch: false,
		},
		{
			name:        "Regex match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: Regex, Value: "nginx.*"},
			expectMatch: true,
		},
		{
			name:        "Regex no match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: Regex, Value: "nginx.*"},
			expectMatch: false,
		},
		{
			name:        "Contains match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: Contains, Value: "nginx"},
			expectMatch: true,
		},
		{
			name:        "Contains no match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: Contains, Value: "nginx"},
			expectMatch: false,
		},
		{
			name:        "NotContains match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: NotContains, Value: "nginx"},
			expectMatch: true,
		},
		{
			name:        "NotContains no match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: NotContains, Value: "nginx"},
			expectMatch: false,
		},
		{
			name:        "Equals match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: Equals, Value: "nginx/1.14.0"},
			expectMatch: true,
		},
		{
			name:        "Equals no match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: Equals, Value: "nginx/1.14.0"},
			expectMatch: false,
		},
		{
			name:        "NotEquals match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: NotEquals, Value: "nginx/1.14.0"},
			expectMatch: true,
		},
		{
			name:        "NotEquals no match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: NotEquals, Value: "nginx/1.14.0"},
			expectMatch: false,
		},
		{
			name:        "StartsWith match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: StartsWith, Value: "nginx"},
			expectMatch: true,
		},
		{
			name:        "StartsWith no match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: StartsWith, Value: "nginx"},
			expectMatch: false,
		},
		{
			name:        "EndsWith match",
			headerValue: "nginx/1.14.0",
			matcher:     HeaderCheckMatcher{MatcherType: EndsWith, Value: "1.14.0"},
			expectMatch: true,
		},
		{
			name:        "EndsWith no match",
			headerValue: "apache/2.2",
			matcher:     HeaderCheckMatcher{MatcherType: EndsWith, Value: "1.14.0"},
			expectMatch: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := test.matcher.Match(test.headerValue)
			if match != test.expectMatch {
				t.Errorf("%s failed: got %v, expect %v", test.name, match, test.expectMatch)
			}
		})
	}
}

func TestHeaderCheck(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string][]string
		check       HeaderCheck
		expectCount int
	}{
		{
			name:        "Exists check",
			headers:     map[string][]string{"Server": []string{"nginx/1.14.0"}},
			check:       HeaderCheck{Headers: []string{"Server"}, Matchers: []HeaderCheckMatcher{{MatcherType: Exists}}, IssueCode: "server-header"},
			expectCount: 1,
		},
		{
			name:        "Equals check",
			headers:     map[string][]string{"Server": []string{"nginx/1.14.0"}},
			check:       HeaderCheck{Headers: []string{"Server"}, Matchers: []HeaderCheckMatcher{{MatcherType: Equals, Value: "nginx/1.14.0"}}, IssueCode: "server-header"},
			expectCount: 1,
		},
		{
			name:        "NotEquals check",
			headers:     map[string][]string{"Server": []string{"nginx/1.14.0"}},
			check:       HeaderCheck{Headers: []string{"Server"}, Matchers: []HeaderCheckMatcher{{MatcherType: NotEquals, Value: "apache"}}, IssueCode: "server-header"},
			expectCount: 1,
		},
		{
			name:        "NotExists check",
			headers:     map[string][]string{"Server": []string{"nginx/1.14.0"}},
			check:       HeaderCheck{Headers: []string{"UnknownHeader"}, Matchers: []HeaderCheckMatcher{{MatcherType: NotExists}}, IssueCode: "missing-header"},
			expectCount: 1,
		},
		{
			name:        "NotExists but header exists check",
			headers:     map[string][]string{"Server": []string{"nginx/1.14.0"}},
			check:       HeaderCheck{Headers: []string{"Server"}, Matchers: []HeaderCheckMatcher{{MatcherType: NotExists}}, IssueCode: "unexpected-header"},
			expectCount: 0,
		},
		{
			name:        "NotExists check with empty value",
			headers:     map[string][]string{"Server": []string{}},
			check:       HeaderCheck{Headers: []string{"Server"}, Matchers: []HeaderCheckMatcher{{MatcherType: NotExists}}, IssueCode: "missing-header-value"},
			expectCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchResults := test.check.Check(test.headers)
			if len(matchResults) != test.expectCount {
				t.Errorf("%s failed: got %v matches, expect %v matches", test.name, len(matchResults), test.expectCount)
			}
			for _, result := range matchResults {
				if !result.Matched {
					t.Errorf("%s failed: got unmatched result", test.name)
				}
				if result.IssueCode != test.check.IssueCode {
					t.Errorf("%s failed: got %v, expect %v", test.name, result.IssueCode, test.check.IssueCode)
				}
			}
		})
	}
}
