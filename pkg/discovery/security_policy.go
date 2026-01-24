package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// SecurityPolicyPaths contains paths for security policy files
var SecurityPolicyPaths = []string{
	".well-known/security.txt",
	"security.txt",
	".well-known/mta-sts.txt",
	".well-known/dnt-policy.txt",
	".well-known/gpc.json",
}

// IsSecurityTxtValidationFunc validates security.txt files
func IsSecurityTxtValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: security.txt should be plain text, not HTML
	if isHTMLResponse(history) {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Security.txt file detected: %s\n\n", history.URL))

	// Check content type
	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "text/plain") {
		confidence += 20
		details.WriteString("- Valid content type (text/plain)\n")
	}

	// Check for required fields according to RFC 9116
	requiredFields := map[string]string{
		"Contact:": "Contact information",
		"Expires:": "Policy expiration",
	}

	optionalFields := map[string]string{
		"Encryption:":          "PGP key for encrypted communication",
		"Acknowledgments:":     "Acknowledgments page",
		"Acknowledgements:":    "Acknowledgments page (alt spelling)",
		"Policy:":              "Security policy link",
		"Hiring:":              "Security job opportunities",
		"Preferred-Languages:": "Preferred languages",
		"Canonical:":           "Canonical URL for this file",
	}

	for field, description := range requiredFields {
		if strings.Contains(bodyStr, field) {
			confidence += 25
			details.WriteString(fmt.Sprintf("- Contains required field: %s\n", description))
		}
	}

	for field, description := range optionalFields {
		if strings.Contains(bodyStr, field) {
			confidence += 10
			details.WriteString(fmt.Sprintf("- Contains optional field: %s\n", description))
		}
	}

	// Check for PGP signature
	if strings.Contains(bodyStr, "-----BEGIN PGP SIGNED MESSAGE-----") ||
		strings.Contains(bodyStr, "-----BEGIN PGP SIGNATURE-----") {
		confidence += 15
		details.WriteString("- File is PGP signed\n")
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsMtaStsValidationFunc validates MTA-STS policy files
func IsMtaStsValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: MTA-STS should be plain text, not HTML
	if isHTMLResponse(history) {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("MTA-STS policy file detected: %s\n\n", history.URL))

	// Check for required MTA-STS fields
	if strings.Contains(bodyStr, "version:") {
		confidence += 30
		details.WriteString("- Contains version field\n")
	}

	if strings.Contains(bodyStr, "mode:") {
		confidence += 30
		details.WriteString("- Contains mode field\n")

		// Extract mode value
		if strings.Contains(bodyStr, "mode: enforce") {
			details.WriteString("  Mode: enforce (TLS required)\n")
		} else if strings.Contains(bodyStr, "mode: testing") {
			details.WriteString("  Mode: testing (TLS optional, report only)\n")
		} else if strings.Contains(bodyStr, "mode: none") {
			details.WriteString("  Mode: none (no TLS requirement)\n")
		}
	}

	if strings.Contains(bodyStr, "max_age:") {
		confidence += 20
		details.WriteString("- Contains max_age field\n")
	}

	if strings.Contains(bodyStr, "mx:") {
		confidence += 20
		details.WriteString("- Contains MX host specifications\n")
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsDntPolicyValidationFunc validates DNT policy files
func IsDntPolicyValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: DNT policy should be plain text or JSON, not HTML
	if isHTMLResponse(history) {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("DNT policy file detected: %s\n\n", history.URL))

	// Check content type - DNT policy must be proper format
	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "text/plain") {
		confidence += 20
		details.WriteString("- Valid content type (text/plain)\n")
	} else if strings.Contains(contentType, "application/json") {
		confidence += 20
		details.WriteString("- Valid content type (application/json)\n")
	}

	// For .txt files, check for specific DNT policy format
	// For GPC .json files, check for JSON structure
	if strings.HasSuffix(history.URL, ".json") {
		// GPC (Global Privacy Control) JSON format
		if strings.Contains(bodyStr, "gpc") || strings.Contains(bodyStr, "\"version\"") {
			confidence += 30
			details.WriteString("- Contains GPC-related content\n")
		}
	} else {
		// DNT policy text format - needs specific privacy-related terms
		dntIndicators := []struct {
			term   string
			weight int
		}{
			{"do not track", 25},
			{"tracking", 15},
			{"DNT", 20},
			{"privacy policy", 15},
		}

		foundIndicators := 0
		for _, indicator := range dntIndicators {
			if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(indicator.term)) {
				confidence += indicator.weight
				foundIndicators++
				details.WriteString(fmt.Sprintf("- Contains DNT-related term: %s\n", indicator.term))
			}
		}

		// Need at least one specific DNT indicator
		if foundIndicators == 0 {
			return false, "", 0
		}
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverSecurityPolicy discovers security policy files
func DiscoverSecurityPolicy(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	// Check for security.txt
	securityTxtResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/security.txt", "security.txt"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
			StopAfterValid:         true,
		},
		ValidationFunc: IsSecurityTxtValidationFunc,
		IssueCode:      db.SecurityTxtDetectedCode,
	})

	if err != nil {
		return securityTxtResults, err
	}

	// Check for mta-sts.txt
	mtaStsResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/mta-sts.txt"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsMtaStsValidationFunc,
		IssueCode:      db.MtaStsDetectedCode,
	})

	if err != nil {
		return securityTxtResults, err
	}

	// Check for dnt-policy.txt
	dntResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/dnt-policy.txt", ".well-known/gpc.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsDntPolicyValidationFunc,
		IssueCode:      db.DntPolicyDetectedCode,
	})

	if err != nil {
		return securityTxtResults, err
	}

	// Combine results
	combined := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: append(append(securityTxtResults.Responses, mtaStsResults.Responses...), dntResults.Responses...),
			Errors:    append(append(securityTxtResults.Errors, mtaStsResults.Errors...), dntResults.Errors...),
		},
		Issues: append(append(securityTxtResults.Issues, mtaStsResults.Issues...), dntResults.Issues...),
		Errors: append(append(securityTxtResults.Errors, mtaStsResults.Errors...), dntResults.Errors...),
	}

	return combined, nil
}
