package discovery

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type CrossDomainPolicy struct {
	XMLName      xml.Name      `xml:"cross-domain-policy"`
	AllowAccess  []AllowAccess `xml:"allow-access-from"`
	AllowHeaders []AllowHeader `xml:"allow-http-request-headers-from"`
}

type AllowAccess struct {
	Domain string `xml:"domain,attr"`
	Secure string `xml:"secure,attr"`
}

type AllowHeader struct {
	Domain  string `xml:"domain,attr"`
	Headers string `xml:"headers,attr"`
	Secure  string `xml:"secure,attr"`
}

func getSecureValue(secure string) string {
	if secure == "" {
		return "not specified"
	}
	return secure
}

func analyzeCrossDomainPolicy(policy *CrossDomainPolicy) ([]string, string) {
	var issues []string
	severity := "Info"

	for _, access := range policy.AllowAccess {
		if access.Domain == "*" {
			issues = append(issues, "Policy allows access from any domain")
			severity = "High"
		} else if access.Domain == "*.com" || access.Domain == "*.org" || access.Domain == "*.net" {
			issues = append(issues, fmt.Sprintf("Policy allows access from all %s domains", access.Domain))
			severity = "High"
		} else if strings.HasPrefix(access.Domain, "*.") && !strings.Contains(access.Domain[2:], ".") {
			issues = append(issues, fmt.Sprintf("Broad wildcard domain pattern detected: %s", access.Domain))
			if severity != "High" {
				severity = "Medium"
			}
		}

		if access.Secure == "false" {
			issues = append(issues, fmt.Sprintf("Non-secure access allowed for domain: %s", access.Domain))
			if severity == "Info" {
				severity = "Low"
			}
		}
	}

	for _, header := range policy.AllowHeaders {
		if header.Domain == "*" {
			issues = append(issues, "Policy allows headers from any domain")
			severity = "High"
		}

		if header.Headers == "*" {
			issues = append(issues, fmt.Sprintf("All headers allowed from domain: %s", header.Domain))
			if severity != "High" {
				severity = "Medium"
			}
		}

		sensitiveHeaders := []string{"Authorization", "Cookie", "X-", "Proxy-"}
		headersList := strings.Split(strings.ToLower(header.Headers), ",")
		for _, h := range headersList {
			h = strings.TrimSpace(h)
			for _, sensitive := range sensitiveHeaders {
				if strings.HasPrefix(strings.ToLower(h), strings.ToLower(sensitive)) {
					issues = append(issues, fmt.Sprintf("Sensitive header %s allowed from domain: %s", h, header.Domain))
					if severity != "High" {
						severity = "Medium"
					}
				}
			}
		}

		if header.Secure == "false" {
			issues = append(issues, fmt.Sprintf("Non-secure headers allowed from domain: %s", header.Domain))
			if severity == "Info" {
				severity = "Low"
			}
		}
	}

	return issues, severity
}

func IsFlashCrossDomainValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	contentType := strings.ToLower(history.ResponseContentType)
	if !strings.Contains(contentType, "xml") && !strings.Contains(contentType, "text/plain") {
		return false, "", 0
	}
	body, _ := history.ResponseBody()

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<cross-domain-policy") {
		return false, "", 0
	}

	var policy CrossDomainPolicy
	if err := xml.Unmarshal(body, &policy); err != nil {
		log.Warn().Str("url", history.URL).Err(err).Msg("Failed to unmarshal cross-domain policy XML")
		return false, "", 0
	}

	issues, severity := analyzeCrossDomainPolicy(&policy)
	details := fmt.Sprintf("Flash crossdomain.xml policy found at: %s\n\n", history.URL)

	if len(issues) > 0 {
		details += "Policy Analysis:\n"
		for _, issue := range issues {
			details += fmt.Sprintf("• %s\n", issue)
		}
	} else {
		details += "The policy follows security best practices.\n"
	}

	details += "\nPolicy Configuration:\n"
	for _, access := range policy.AllowAccess {
		details += fmt.Sprintf("• Domain access: %s (secure: %s)\n",
			access.Domain, getSecureValue(access.Secure))
	}

	for _, header := range policy.AllowHeaders {
		details += fmt.Sprintf("• Header access from %s: %s (secure: %s)\n",
			header.Domain, header.Headers, getSecureValue(header.Secure))
	}

	details += fmt.Sprintf("\n\nSeverity: %s\n", severity)

	// TODO: ValidationFunc should be refactored to support returning additional metadata like severity overrides.
	// Just logging the severity for now.

	log.Info().
		Str("url", history.URL).
		Str("severity", severity).
		Msg("Flash cross-domain policy analysis severity override")

	confidence := 90
	return true, details, confidence
}

func DiscoverFlashCrossDomainPolicy(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       []string{"crossdomain.xml"},
			Concurrency: 1,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/xml,application/xml,text/plain",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsFlashCrossDomainValidationFunc,
		IssueCode:      db.FlashCrossdomainPolicyCode,
	})
}
