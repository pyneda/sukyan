package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// AppAssociationPaths contains paths for app association files
var AppAssociationPaths = []string{
	".well-known/assetlinks.json",
	".well-known/apple-app-site-association",
	"apple-app-site-association",
}

// IsAndroidAssetLinksValidationFunc validates Android assetlinks.json files
func IsAndroidAssetLinksValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	confidence := 0
	var details strings.Builder
	details.WriteString(fmt.Sprintf("Android Asset Links file detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 20
		details.WriteString("- Valid JSON content type\n")
	}

	// Try to parse as JSON array (assetlinks.json is an array)
	var assetLinks []map[string]interface{}
	if err := json.Unmarshal(body, &assetLinks); err != nil {
		// Try parsing as object (some implementations)
		var singleLink map[string]interface{}
		if err := json.Unmarshal(body, &singleLink); err != nil {
			return false, "", 0
		}
		assetLinks = []map[string]interface{}{singleLink}
	}

	if len(assetLinks) > 0 {
		confidence += 30

		for _, link := range assetLinks {
			// Check for Android app link structure
			if relation, ok := link["relation"].([]interface{}); ok {
				for _, r := range relation {
					if rStr, ok := r.(string); ok {
						if strings.Contains(rStr, "handle_all_urls") || strings.Contains(rStr, "android") {
							confidence += 20
							details.WriteString(fmt.Sprintf("- Relation: %s\n", rStr))
						}
					}
				}
			}

			// Check for target with package name
			if target, ok := link["target"].(map[string]interface{}); ok {
				if namespace, ok := target["namespace"].(string); ok && namespace == "android_app" {
					confidence += 20
					details.WriteString("- Target namespace: android_app\n")
				}
				if packageName, ok := target["package_name"].(string); ok {
					details.WriteString(fmt.Sprintf("- Package name: %s\n", packageName))
				}
				if fingerprints, ok := target["sha256_cert_fingerprints"].([]interface{}); ok {
					details.WriteString(fmt.Sprintf("- Certificate fingerprints: %d configured\n", len(fingerprints)))
				}
			}
		}
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// IsAppleAppSiteAssociationValidationFunc validates Apple app site association files
func IsAppleAppSiteAssociationValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	confidence := 0
	var details strings.Builder
	details.WriteString(fmt.Sprintf("Apple App Site Association file detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "application/json") ||
		strings.Contains(history.ResponseContentType, "application/pkcs7-mime") {
		confidence += 20
		details.WriteString("- Valid content type for AASA\n")
	}

	// Try to parse as JSON
	var aasa map[string]interface{}
	if err := json.Unmarshal(body, &aasa); err != nil {
		return false, "", 0
	}

	confidence += 20

	// Check for applinks
	if applinks, ok := aasa["applinks"].(map[string]interface{}); ok {
		confidence += 30
		details.WriteString("- Contains applinks configuration\n")

		if apps, ok := applinks["apps"].([]interface{}); ok {
			details.WriteString(fmt.Sprintf("- Apps array: %d entries\n", len(apps)))
		}

		if details, ok := applinks["details"].([]interface{}); ok {
			for _, d := range details {
				if dMap, ok := d.(map[string]interface{}); ok {
					if appID, ok := dMap["appID"].(string); ok {
						confidence += 10
						_ = appID // Avoid unused variable warning
					}
					if appIDs, ok := dMap["appIDs"].([]interface{}); ok {
						confidence += 10
						_ = appIDs
					}
				}
			}
		}
	}

	// Check for webcredentials
	if webcredentials, ok := aasa["webcredentials"].(map[string]interface{}); ok {
		confidence += 20
		details.WriteString("- Contains webcredentials configuration\n")

		if apps, ok := webcredentials["apps"].([]interface{}); ok {
			details.WriteString(fmt.Sprintf("- Webcredentials apps: %d configured\n", len(apps)))
		}
	}

	// Check for appclips
	if appclips, ok := aasa["appclips"].(map[string]interface{}); ok {
		confidence += 15
		details.WriteString("- Contains App Clips configuration\n")
		_ = appclips
	}

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverAppAssociation discovers app association files
func DiscoverAppAssociation(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	// First check for Android assetlinks.json
	androidResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/assetlinks.json"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsAndroidAssetLinksValidationFunc,
		IssueCode:      db.AndroidAssetLinksDetectedCode,
	})

	if err != nil {
		return androidResults, err
	}

	// Then check for Apple app site association
	appleResults, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  []string{".well-known/apple-app-site-association", "apple-app-site-association"},
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsAppleAppSiteAssociationValidationFunc,
		IssueCode:      db.AppleAppSiteAssociationDetectedCode,
	})

	if err != nil {
		// Return what we have from Android check
		return androidResults, err
	}

	// Combine results
	combined := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: append(androidResults.Responses, appleResults.Responses...),
			Errors:    append(androidResults.Errors, appleResults.Errors...),
			Stopped:   androidResults.Stopped || appleResults.Stopped,
		},
		Issues: append(androidResults.Issues, appleResults.Issues...),
		Errors: append(androidResults.Errors, appleResults.Errors...),
	}

	return combined, nil
}
