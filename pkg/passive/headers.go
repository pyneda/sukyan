package passive

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"regexp"
	"strings"
)

type MatcherType string
type MatcherCondition string

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

type HeaderCheck struct {
	Headers        []string
	Matchers       []HeaderCheckMatcher
	MatchCondition MatcherCondition
	IssueCode      db.IssueCode
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

func (c *HeaderCheck) Check(headers map[string][]string) (bool, db.IssueCode, string) {
	var sb strings.Builder
	var matchFound bool
	var issueCode db.IssueCode

	for _, headerName := range c.Headers {
		headerValues, exists := headers[headerName]

		if !exists {
			// sb.WriteString(fmt.Sprintf("Header '%s' does not exist.\n", headerName))
			continue
		}

		for _, matcher := range c.Matchers {
			for _, headerValue := range headerValues {
				match := matcher.Match(headerValue)
				if match {
					sb.WriteString(fmt.Sprintf("Header '%s' with value '%s' matches the condition '%s' %s.\n", headerName, headerValue, matcher.MatcherType, matcher.Value))
					if matcher.CustomIssueCode != "" {
						issueCode = matcher.CustomIssueCode
					} else {
						issueCode = c.IssueCode
					}
					if c.MatchCondition == Or {
						return true, issueCode, sb.String()
					}
					matchFound = true
				} else {
					if c.MatchCondition == And {
						return false, issueCode, sb.String()
					}
				}
			}
		}
	}

	if matchFound && c.MatchCondition == And {
		return true, issueCode, sb.String()
	} else if !matchFound && c.MatchCondition == Or {
		return false, issueCode, sb.String()
	} else {
		return false, issueCode, sb.String()
	}
}

var headerMatchAny = HeaderCheckMatcher{
	MatcherType: Exists,
}
