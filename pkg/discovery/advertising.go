package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// AdvertisingPaths contains paths for advertising-related files
var AdvertisingPaths = []string{
	"ads.txt",
	"app-ads.txt",
	"sellers.json",
}

// IsAdsTxtValidationFunc validates ads.txt files
func IsAdsTxtValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
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

	details.WriteString(fmt.Sprintf("Ads.txt file detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 20
		details.WriteString("- Valid content type (text/plain)\n")
	}

	lines := strings.Split(bodyStr, "\n")
	validEntries := 0
	directEntries := 0
	resellerEntries := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// ads.txt format: domain, pub-id, relationship[, cert-id]
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			validEntries++
			relationship := strings.TrimSpace(strings.ToUpper(parts[2]))
			if relationship == "DIRECT" {
				directEntries++
			} else if relationship == "RESELLER" {
				resellerEntries++
			}
		}
	}

	if validEntries > 0 {
		confidence += 40
		details.WriteString(fmt.Sprintf("- Valid ads.txt entries: %d\n", validEntries))
		details.WriteString(fmt.Sprintf("  - DIRECT relationships: %d\n", directEntries))
		details.WriteString(fmt.Sprintf("  - RESELLER relationships: %d\n", resellerEntries))
	}

	// Check for common ad networks
	adNetworks := []string{
		"google.com",
		"pubmatic.com",
		"rubiconproject.com",
		"openx.com",
		"appnexus.com",
		"indexexchange.com",
		"33across.com",
		"criteo.com",
	}

	foundNetworks := 0
	for _, network := range adNetworks {
		if strings.Contains(strings.ToLower(bodyStr), network) {
			foundNetworks++
		}
	}

	if foundNetworks > 0 {
		confidence += 20
		details.WriteString(fmt.Sprintf("- Recognized ad networks: %d\n", foundNetworks))
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsSellersJsonValidationFunc validates sellers.json files
func IsSellersJsonValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Sellers.json file detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 20
		details.WriteString("- Valid JSON content type\n")
	}

	// Try to parse as JSON
	var sellersData map[string]interface{}
	if err := json.Unmarshal(body, &sellersData); err != nil {
		return false, "", 0
	}

	confidence += 20
	details.WriteString("- Valid JSON structure\n")

	// Check for sellers.json specific fields
	if sellers, ok := sellersData["sellers"].([]interface{}); ok {
		confidence += 30
		details.WriteString(fmt.Sprintf("- Sellers array: %d entries\n", len(sellers)))

		// Analyze seller types
		publisherCount := 0
		intermediaryCount := 0
		bothCount := 0

		for _, seller := range sellers {
			if sellerMap, ok := seller.(map[string]interface{}); ok {
				if sellerType, ok := sellerMap["seller_type"].(string); ok {
					switch strings.ToUpper(sellerType) {
					case "PUBLISHER":
						publisherCount++
					case "INTERMEDIARY":
						intermediaryCount++
					case "BOTH":
						bothCount++
					}
				}
			}
		}

		if publisherCount > 0 || intermediaryCount > 0 || bothCount > 0 {
			details.WriteString(fmt.Sprintf("  - Publishers: %d\n", publisherCount))
			details.WriteString(fmt.Sprintf("  - Intermediaries: %d\n", intermediaryCount))
			details.WriteString(fmt.Sprintf("  - Both: %d\n", bothCount))
		}
	}

	// Check for contact info
	if contactEmail, ok := sellersData["contact_email"].(string); ok {
		confidence += 10
		details.WriteString(fmt.Sprintf("- Contact email: %s\n", contactEmail))
	}

	if contactAddress, ok := sellersData["contact_address"].(string); ok {
		confidence += 10
		_ = contactAddress
		details.WriteString("- Contact address present\n")
	}

	// Check for version
	if version, ok := sellersData["version"].(string); ok {
		confidence += 10
		details.WriteString(fmt.Sprintf("- Version: %s\n", version))
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverAdvertising discovers advertising-related files
func DiscoverAdvertising(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	// Check for ads.txt and app-ads.txt
	adsTxtResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{"ads.txt", "app-ads.txt"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsAdsTxtValidationFunc,
		IssueCode:      db.AdsTxtDetectedCode,
	})

	if err != nil {
		return adsTxtResults, err
	}

	// Check for sellers.json
	sellersResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{"sellers.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsSellersJsonValidationFunc,
		IssueCode:      db.SellersJsonDetectedCode,
	})

	if err != nil {
		return adsTxtResults, err
	}

	// Combine results
	combined := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: append(adsTxtResults.Responses, sellersResults.Responses...),
			Errors:    append(adsTxtResults.Errors, sellersResults.Errors...),
		},
		Issues: append(adsTxtResults.Issues, sellersResults.Issues...),
		Errors: append(adsTxtResults.Errors, sellersResults.Errors...),
	}

	return combined, nil
}
