package passive

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"regexp"
	"strings"
)

type MatcherType string
type MatcherCondition string

type MatchResult struct {
	IssueCode   db.IssueCode
	Matched     bool
	Description string
}

const (
	Exists      MatcherType      = "exists"
	Regex       MatcherType      = "regex"
	Contains    MatcherType      = "contains"
	NotContains MatcherType      = "not-contains"
	Equals      MatcherType      = "equals"
	NotEquals   MatcherType      = "not-equals"
	StartsWith  MatcherType      = "starts-with"
	EndsWith    MatcherType      = "ends-with"
	And         MatcherCondition = "and"
	Or          MatcherCondition = "or"
)

type HeaderCheckMatcher struct {
	MatcherType     MatcherType
	Value           string
	CustomIssueCode db.IssueCode
}

func (m *HeaderCheckMatcher) CheckMatcher(headerName string, headerValues []string) []MatchResult {
	var matchResults []MatchResult

	for _, headerValue := range headerValues {
		match := m.Match(headerValue)
		if match {
			description := fmt.Sprintf("Header '%s' with value '%s' matches the condition '%s' %s.\n", headerName, headerValue, m.MatcherType, m.Value)
			matchResults = append(matchResults, MatchResult{IssueCode: m.CustomIssueCode, Matched: true, Description: description})
		}
	}

	return matchResults
}
func (m *HeaderCheckMatcher) Match(headerValue string) bool {
	switch m.MatcherType {
	case Exists:
		return headerValue != ""
	case Regex:
		matched, _ := regexp.MatchString(m.Value, headerValue)
		return matched
	case Contains:
		return strings.Contains(headerValue, m.Value)
	case NotContains:
		return !strings.Contains(headerValue, m.Value)
	case Equals:
		return headerValue == m.Value
	case NotEquals:
		return headerValue != m.Value
	case StartsWith:
		return strings.HasPrefix(headerValue, m.Value)
	case EndsWith:
		return strings.HasSuffix(headerValue, m.Value)
	default:
		return false
	}
}

type HeaderCheck struct {
	Headers        []string
	Matchers       []HeaderCheckMatcher
	MatchCondition MatcherCondition
	IssueCode      db.IssueCode
}

func (c *HeaderCheck) Check(headers map[string][]string) []MatchResult {
	var matchResults []MatchResult

	for _, headerName := range c.Headers {
		headerValues, exists := headers[headerName]

		if !exists {
			continue
		}

		results := c.CheckHeader(headerName, headerValues)
		matchResults = append(matchResults, results...)
	}

	return matchResults
}

func (c *HeaderCheck) CheckHeader(headerName string, headerValues []string) []MatchResult {
	var matchResults []MatchResult
	for _, matcher := range c.Matchers {
		results := matcher.CheckMatcher(headerName, headerValues)
		for _, result := range results {
			if result.Matched {
				if result.IssueCode == "" {
					result.IssueCode = c.IssueCode
				}
				matchResults = append(matchResults, result)
			}
		}
	}
	return matchResults
}

var headerMatchAny = HeaderCheckMatcher{
	MatcherType: Exists,
}
