package discovery

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// WellKnownMetadataPaths contains paths for various well-known metadata files
var WellKnownMetadataPaths = []string{
	".well-known/host-meta",
	".well-known/host-meta.json",
	".well-known/keybase.txt",
	".well-known/sbom",
	".well-known/csaf/provider-metadata.json",
	".well-known/change-password",
	".well-known/webfinger",
	".well-known/nodeinfo",
}

// IsHostMetaValidationFunc validates host-meta files
func IsHostMetaValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	trimmedBody := strings.TrimSpace(bodyStr)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Host-Meta file detected: %s\n\n", history.URL))

	// Check for JSON format
	if strings.Contains(history.ResponseContentType, "application/json") ||
		strings.HasSuffix(history.URL, ".json") {
		// CRITICAL: If expecting JSON, reject HTML responses
		if isHTMLResponse(history) {
			return false, "", 0
		}

		var hostMeta map[string]interface{}
		if err := json.Unmarshal(body, &hostMeta); err == nil {
			// Must have host-meta specific fields
			if links, ok := hostMeta["links"].([]interface{}); ok {
				confidence += 60
				details.WriteString("- Valid JSON host-meta format\n")
				details.WriteString(fmt.Sprintf("- Links defined: %d\n", len(links)))

				for _, link := range links {
					if linkMap, ok := link.(map[string]interface{}); ok {
						if rel, ok := linkMap["rel"].(string); ok {
							details.WriteString(fmt.Sprintf("  - Relation: %s\n", rel))
						}
					}
				}
			} else {
				// JSON without links field is not a valid host-meta
				return false, "", 0
			}
		} else {
			// Invalid JSON
			return false, "", 0
		}
	} else if strings.Contains(history.ResponseContentType, "application/xrd+xml") ||
		strings.Contains(history.ResponseContentType, "application/xml") ||
		strings.HasPrefix(trimmedBody, "<?xml") ||
		strings.HasPrefix(trimmedBody, "<XRD") {
		// XRD (XML) format - but reject generic HTML
		if strings.Contains(strings.ToLower(trimmedBody), "<!doctype html") ||
			(strings.Contains(trimmedBody, "<html") && !strings.Contains(trimmedBody, "<XRD")) {
			return false, "", 0
		}

		// Must start with XML or XRD element
		if !strings.HasPrefix(trimmedBody, "<?xml") && !strings.HasPrefix(trimmedBody, "<XRD") {
			return false, "", 0
		}

		confidence += 40
		details.WriteString("- XRD/XML host-meta format detected\n")

		// Basic XML validation
		decoder := xml.NewDecoder(strings.NewReader(bodyStr))
		if err := decoder.Decode(new(interface{})); err == nil {
			confidence += 20
			details.WriteString("- Valid XML structure\n")
		}

		// Check for common host-meta elements
		if strings.Contains(bodyStr, "<Link") {
			confidence += 20
			details.WriteString("- Contains Link elements\n")
		} else {
			// XRD without Link elements is not useful
			confidence -= 20
		}
	} else {
		// Unknown content type - check if it looks like host-meta at all
		if isHTMLResponse(history) {
			return false, "", 0
		}

		// Might be XRD served with wrong content type
		if strings.HasPrefix(trimmedBody, "<?xml") || strings.HasPrefix(trimmedBody, "<XRD") {
			if strings.Contains(bodyStr, "<Link") {
				confidence += 50
				details.WriteString("- XRD format detected (incorrect content-type)\n")
			} else {
				return false, "", 0
			}
		} else {
			return false, "", 0
		}
	}

	// Check for WebFinger reference - adds confidence but not required
	if strings.Contains(bodyStr, "webfinger") || strings.Contains(bodyStr, "lrdd") {
		confidence += 15
		details.WriteString("- References WebFinger/LRDD\n")
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsKeybaseTxtValidationFunc validates keybase.txt files
func IsKeybaseTxtValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: Keybase.txt should be plain text, not HTML
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

	details.WriteString(fmt.Sprintf("Keybase proof file detected: %s\n\n", history.URL))

	// Check content type - should be text/plain
	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 20
		details.WriteString("- Valid content type (text/plain)\n")
	}

	// Check for Keybase-specific patterns with high specificity
	keybaseIndicators := []struct {
		pattern     string
		description string
		weight      int
		required    bool
	}{
		{"BEGIN KEYBASE SALTPACK", "Keybase saltpack message", 50, false},
		{"keybase.io", "Keybase domain reference", 30, false},
		{"https://keybase.io/", "Keybase URL", 35, false},
		{"sig_id", "Signature ID field", 25, false},
		{"public_key", "Public key reference", 20, false},
	}

	foundIndicators := 0
	for _, indicator := range keybaseIndicators {
		if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(indicator.pattern)) {
			confidence += indicator.weight
			foundIndicators++
			details.WriteString(fmt.Sprintf("- Contains: %s\n", indicator.description))
		}
	}

	// Need at least one strong Keybase-specific indicator
	if foundIndicators == 0 {
		return false, "", 0
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsSbomValidationFunc validates SBOM files
func IsSbomValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	// CRITICAL: SBOM should be JSON or XML, not HTML
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

	details.WriteString(fmt.Sprintf("Software Bill of Materials (SBOM) detected: %s\n\n", history.URL))

	// Try to parse as JSON first - most common format
	var sbomData map[string]interface{}
	if err := json.Unmarshal(body, &sbomData); err == nil {
		// Valid JSON - check for SBOM-specific structure
		foundSbomFields := 0

		// CycloneDX specific fields
		if _, ok := sbomData["bomFormat"]; ok {
			confidence += 30
			foundSbomFields++
			details.WriteString("- CycloneDX bomFormat field detected\n")
		}
		if _, ok := sbomData["specVersion"]; ok {
			confidence += 20
			foundSbomFields++
			details.WriteString("- SBOM specVersion field detected\n")
		}
		if components, ok := sbomData["components"].([]interface{}); ok {
			confidence += 25
			foundSbomFields++
			details.WriteString(fmt.Sprintf("- Components listed: %d\n", len(components)))
		}

		// SPDX specific fields
		if _, ok := sbomData["spdxVersion"]; ok {
			confidence += 30
			foundSbomFields++
			details.WriteString("- SPDX version field detected\n")
		}
		if _, ok := sbomData["packages"].([]interface{}); ok {
			confidence += 25
			foundSbomFields++
			details.WriteString("- SPDX packages field detected\n")
		}

		// Need at least one SBOM-specific field
		if foundSbomFields == 0 {
			return false, "", 0
		}

		details.WriteString("- Valid JSON structure\n")
	} else {
		// Not valid JSON - check for SPDX tag-value or RDF format
		if strings.Contains(bodyStr, "SPDXVersion:") || strings.Contains(bodyStr, "spdx:") {
			confidence += 50
			details.WriteString("- SPDX tag-value format detected\n")
		} else if strings.Contains(bodyStr, "<spdx:") || strings.Contains(bodyStr, "xmlns:spdx") {
			confidence += 50
			details.WriteString("- SPDX RDF/XML format detected\n")
		} else {
			// Not a recognizable SBOM format
			return false, "", 0
		}
	}

	details.WriteString("\nNote: Exposed SBOMs reveal complete dependency information to potential attackers.\n")

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsCsafValidationFunc validates CSAF provider metadata files
func IsCsafValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("CSAF provider metadata detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 20
		details.WriteString("- Valid JSON content type\n")
	}

	// Try to parse as JSON
	var csafData map[string]interface{}
	if err := json.Unmarshal(body, &csafData); err != nil {
		return false, "", 0
	}

	confidence += 20
	details.WriteString("- Valid JSON structure\n")

	// Check for CSAF-specific fields
	if canonical, ok := csafData["canonical_url"].(string); ok {
		confidence += 20
		details.WriteString(fmt.Sprintf("- Canonical URL: %s\n", canonical))
	}

	if publisher, ok := csafData["publisher"].(map[string]interface{}); ok {
		confidence += 20
		details.WriteString("- Publisher information present\n")
		if name, ok := publisher["name"].(string); ok {
			details.WriteString(fmt.Sprintf("  - Publisher: %s\n", name))
		}
	}

	if distributions, ok := csafData["distributions"].([]interface{}); ok {
		confidence += 15
		details.WriteString(fmt.Sprintf("- Distribution channels: %d\n", len(distributions)))
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsChangePasswordValidationFunc validates change-password well-known URL
func IsChangePasswordValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	// Accept 200 (page content) or 3xx (redirect to actual change password page)
	if history.StatusCode != 200 && (history.StatusCode < 300 || history.StatusCode >= 400) {
		return false, "", 0
	}

	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Change password endpoint detected: %s\n\n", history.URL))

	if history.StatusCode >= 300 && history.StatusCode < 400 {
		// Redirect is the expected behavior per W3C spec
		confidence += 60
		details.WriteString("- Endpoint redirects (expected behavior)\n")

		// Check for Location header - must point to a password-related URL
		headers, err := history.ResponseHeaders()
		if err == nil {
			if locations, ok := headers["Location"]; ok && len(locations) > 0 {
				location := strings.ToLower(locations[0])
				// Verify the redirect target looks like a password change page
				if strings.Contains(location, "password") ||
					strings.Contains(location, "account") ||
					strings.Contains(location, "settings") ||
					strings.Contains(location, "security") ||
					strings.Contains(location, "profile") {
					confidence += 25
					details.WriteString(fmt.Sprintf("- Redirects to: %s\n", locations[0]))
				} else {
					// Redirect to unrelated location - likely a homepage redirect (soft 404)
					return false, "", 0
				}
			}
		}
	} else if history.StatusCode == 200 {
		// CRITICAL: Check if this is a generic HTML page (homepage)
		// The /.well-known/change-password endpoint should NOT return a full website
		if isHTMLResponse(history) {
			body, err := history.ResponseBody()
			if err != nil {
				return false, "", 0
			}
			bodyStr := strings.ToLower(string(body))

			// Check if this looks like a generic webpage vs a password change form
			// A password change page should have form elements and password fields
			hasPasswordForm := strings.Contains(bodyStr, "type=\"password\"") ||
				strings.Contains(bodyStr, "type='password'") ||
				strings.Contains(bodyStr, "input type=password")

			hasChangePasswordContext := (strings.Contains(bodyStr, "change") && strings.Contains(bodyStr, "password")) ||
				(strings.Contains(bodyStr, "update") && strings.Contains(bodyStr, "password")) ||
				(strings.Contains(bodyStr, "reset") && strings.Contains(bodyStr, "password")) ||
				strings.Contains(bodyStr, "new password") ||
				strings.Contains(bodyStr, "current password") ||
				strings.Contains(bodyStr, "confirm password")

			// Check for signs this is a generic homepage/app, not a password change page
			// Look for navigation, multiple sections, unrelated content
			looksLikeHomepage := (strings.Contains(bodyStr, "<nav") || strings.Contains(bodyStr, "navigation")) &&
				(strings.Count(bodyStr, "<a href") > 10) // Many links suggest a homepage

			if looksLikeHomepage && !hasPasswordForm && !hasChangePasswordContext {
				// This is likely the homepage being returned for any path
				return false, "", 0
			}

			if hasPasswordForm {
				confidence += 50
				details.WriteString("- Contains password input field\n")
			}

			if hasChangePasswordContext {
				confidence += 30
				details.WriteString("- Contains change password context\n")
			}

			if !hasPasswordForm && !hasChangePasswordContext {
				// HTML page without any password-related functionality
				return false, "", 0
			}

			details.WriteString("- Endpoint returns content directly\n")
		} else {
			// Non-HTML response for change-password is unusual
			return false, "", 0
		}
	}

	details.WriteString("\nThis follows the W3C change-password-url spec for password managers.\n")

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverWellKnownMetadata discovers well-known metadata files
func DiscoverWellKnownMetadata(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	var allResults []DiscoverAndCreateIssueResults

	// Check for host-meta
	hostMetaResults, _ := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/host-meta", ".well-known/host-meta.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsHostMetaValidationFunc,
		IssueCode:      db.HostMetaDetectedCode,
	})
	allResults = append(allResults, hostMetaResults)

	// Check for keybase.txt
	keybaseResults, _ := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/keybase.txt"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsKeybaseTxtValidationFunc,
		IssueCode:      db.KeybaseTxtDetectedCode,
	})
	allResults = append(allResults, keybaseResults)

	// Check for SBOM
	sbomResults, _ := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/sbom"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsSbomValidationFunc,
		IssueCode:      db.SbomDetectedCode,
	})
	allResults = append(allResults, sbomResults)

	// Check for CSAF
	csafResults, _ := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/csaf/provider-metadata.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsCsafValidationFunc,
		IssueCode:      db.CsafDetectedCode,
	})
	allResults = append(allResults, csafResults)

	// Check for change-password
	changePasswordResults, _ := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/change-password"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsChangePasswordValidationFunc,
		IssueCode:      db.ChangePasswordDetectedCode,
	})
	allResults = append(allResults, changePasswordResults)

	// Combine all results
	combined := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: make([]*db.History, 0),
			Errors:    make([]error, 0),
		},
		Issues: make([]db.Issue, 0),
		Errors: make([]error, 0),
	}

	for _, result := range allResults {
		combined.Responses = append(combined.Responses, result.Responses...)
		combined.DiscoverResults.Errors = append(combined.DiscoverResults.Errors, result.DiscoverResults.Errors...)
		combined.Issues = append(combined.Issues, result.Issues...)
		combined.Errors = append(combined.Errors, result.Errors...)
	}

	return combined, nil
}
