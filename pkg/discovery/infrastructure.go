package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// InfrastructurePaths contains paths for infrastructure-related files
var InfrastructurePaths = []string{
	".well-known/terraform.json",
	"terraform.json",
	".well-known/acme-challenge/",
	".well-known/pki-validation/",
}

// IsTerraformConfigValidationFunc validates Terraform configuration files
func IsTerraformConfigValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Terraform configuration detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 20
		details.WriteString("- Valid JSON content type\n")
	}

	// Try to parse as JSON
	var tfConfig map[string]interface{}
	if err := json.Unmarshal(body, &tfConfig); err != nil {
		// Also check for HCL-style content
		if strings.Contains(bodyStr, "resource ") ||
			strings.Contains(bodyStr, "provider ") ||
			strings.Contains(bodyStr, "variable ") ||
			strings.Contains(bodyStr, "terraform {") {
			confidence += 50
			details.WriteString("- Contains Terraform HCL syntax\n")
		}
	} else {
		confidence += 30
		details.WriteString("- Valid JSON structure\n")

		// Check for Terraform-specific keys
		tfKeys := []string{"terraform", "provider", "resource", "data", "variable", "output", "module"}
		for _, key := range tfKeys {
			if _, ok := tfConfig[key]; ok {
				confidence += 15
				details.WriteString(fmt.Sprintf("- Contains Terraform key: %s\n", key))
			}
		}
	}

	// Check for cloud provider indicators
	cloudIndicators := []struct {
		pattern     string
		description string
	}{
		{"aws_", "AWS resource"},
		{"azurerm_", "Azure resource"},
		{"google_", "Google Cloud resource"},
		{"digitalocean_", "DigitalOcean resource"},
		{"kubernetes_", "Kubernetes resource"},
	}

	for _, indicator := range cloudIndicators {
		if strings.Contains(bodyStr, indicator.pattern) {
			confidence += 10
			details.WriteString(fmt.Sprintf("- Contains %s references\n", indicator.description))
		}
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsAcmeChallengeValidationFunc validates ACME challenge directory access
func IsAcmeChallengeValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	// Only accept 200 with proper directory listing or 403
	if history.StatusCode != 200 && history.StatusCode != 403 {
		return false, "", 0
	}

	// CRITICAL: Check if response is HTML that doesn't look like a directory listing
	// Servers often return their homepage for any path
	if isHTMLResponse(history) {
		body, err := history.ResponseBody()
		if err != nil {
			return false, "", 0
		}
		bodyStr := string(body)

		// Only accept HTML if it's an actual directory listing
		if !looksLikeDirectoryListing(bodyStr) {
			return false, "", 0
		}
	}

	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("ACME challenge directory detected: %s\n\n", history.URL))

	if history.StatusCode == 200 {
		body, err := history.ResponseBody()
		if err != nil {
			return false, "", 0
		}
		bodyStr := string(body)

		// Check for actual directory listing indicators
		if looksLikeDirectoryListing(bodyStr) {
			confidence += 70
			details.WriteString("- Directory listing is accessible (200 OK)\n")
			details.WriteString("- Directory listing enabled\n")
		} else {
			// 200 without directory listing is likely a soft 404
			return false, "", 0
		}
	} else if history.StatusCode == 403 {
		// 403 alone is not enough evidence - many servers return 403 for any path
		// We need to verify it's specifically for ACME directory
		body, err := history.ResponseBody()
		if err == nil {
			bodyStr := strings.ToLower(string(body))
			// Look for ACME/Let's Encrypt specific content in 403 response
			if strings.Contains(bodyStr, "acme") ||
				strings.Contains(bodyStr, "letsencrypt") ||
				strings.Contains(bodyStr, "certbot") ||
				strings.Contains(bodyStr, "challenge") {
				confidence += 60
				details.WriteString("- Directory exists but access is forbidden (403)\n")
				details.WriteString("- ACME-related content detected in response\n")
			} else {
				// Generic 403 is not enough evidence
				return false, "", 0
			}
		} else {
			return false, "", 0
		}
	}

	details.WriteString("\nThis directory is used for Let's Encrypt/ACME certificate validation.\n")

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsPkiValidationValidationFunc validates PKI validation directory access
func IsPkiValidationValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	// Only accept 200 with proper directory listing or 403
	if history.StatusCode != 200 && history.StatusCode != 403 {
		return false, "", 0
	}

	// CRITICAL: Check if response is HTML that doesn't look like a directory listing
	if isHTMLResponse(history) {
		body, err := history.ResponseBody()
		if err != nil {
			return false, "", 0
		}
		bodyStr := string(body)

		// Only accept HTML if it's an actual directory listing
		if !looksLikeDirectoryListing(bodyStr) {
			return false, "", 0
		}
	}

	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("PKI validation directory detected: %s\n\n", history.URL))

	if history.StatusCode == 200 {
		body, err := history.ResponseBody()
		if err != nil {
			return false, "", 0
		}
		bodyStr := string(body)

		// Check for actual directory listing
		if looksLikeDirectoryListing(bodyStr) {
			confidence += 70
			details.WriteString("- Directory listing is accessible (200 OK)\n")
		} else {
			// Check for PKI-specific validation file content
			if strings.Contains(bodyStr, ".txt") ||
				strings.Contains(bodyStr, "validation") ||
				strings.Contains(bodyStr, "domain-control") {
				confidence += 50
				details.WriteString("- Directory is accessible (200 OK)\n")
			} else {
				// Generic 200 response is likely a soft 404
				return false, "", 0
			}
		}
	} else if history.StatusCode == 403 {
		// 403 alone is not enough evidence
		body, err := history.ResponseBody()
		if err == nil {
			bodyStr := strings.ToLower(string(body))
			// Look for PKI/SSL-related content
			if strings.Contains(bodyStr, "pki") ||
				strings.Contains(bodyStr, "certificate") ||
				strings.Contains(bodyStr, "validation") ||
				strings.Contains(bodyStr, "ssl") {
				confidence += 55
				details.WriteString("- Directory exists but access is forbidden (403)\n")
				details.WriteString("- PKI-related content detected in response\n")
			} else {
				// Generic 403 is not enough evidence
				return false, "", 0
			}
		} else {
			return false, "", 0
		}
	}

	details.WriteString("\nThis directory is used for SSL/TLS certificate domain control validation.\n")

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverInfrastructure discovers infrastructure-related files
func DiscoverInfrastructure(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	// Check for Terraform configs
	terraformResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/terraform.json", "terraform.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsTerraformConfigValidationFunc,
		IssueCode:      db.TerraformConfigDetectedCode,
	})

	if err != nil {
		return terraformResults, err
	}

	// Check for ACME challenge directory
	acmeResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/acme-challenge/"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsAcmeChallengeValidationFunc,
		IssueCode:      db.AcmeChallengeDetectedCode,
	})

	if err != nil {
		return terraformResults, err
	}

	// Check for PKI validation directory
	pkiResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/pki-validation/"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsPkiValidationValidationFunc,
		IssueCode:      db.PkiValidationDetectedCode,
	})

	if err != nil {
		return terraformResults, err
	}

	// Combine results
	combined := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: append(append(terraformResults.Responses, acmeResults.Responses...), pkiResults.Responses...),
			Errors:    append(append(terraformResults.Errors, acmeResults.Errors...), pkiResults.Errors...),
		},
		Issues: append(append(terraformResults.Issues, acmeResults.Issues...), pkiResults.Issues...),
		Errors: append(append(terraformResults.Errors, acmeResults.Errors...), pkiResults.Errors...),
	}

	return combined, nil
}
