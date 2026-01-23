package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

type CloudProvider struct {
	Name    string
	Paths   []string
	Headers map[string]string
}

var AWSMetadata = CloudProvider{
	Name: "AWS",
	Paths: []string{
		"latest/meta-data/",
		"latest/meta-data/iam/security-credentials/",
		"latest/user-data",
		"latest/dynamic/instance-identity/document",
		"latest/meta-data/iam/info",
		"latest/meta-data/public-keys/",
		"latest/meta-data/public-hostname",
		"latest/meta-data/public-ipv4",
	},
	Headers: map[string]string{
		"X-aws-ec2-metadata-token-ttl-seconds": "21600",
		"X-aws-ec2-metadata-token":             "true",
	},
}

var GCPMetadata = CloudProvider{
	Name: "GCP",
	Paths: []string{
		"computeMetadata/v1/",
		"computeMetadata/v1/instance/service-accounts/default/token",
		"metadata.google.internal/computeMetadata/v1/",
		"metadata/instance/service-accounts/default/token",
	},
	Headers: map[string]string{
		"Metadata-Flavor": "Google",
	},
}

var AzureMetadata = CloudProvider{
	Name: "Azure",
	Paths: []string{
		"metadata/instance/",
		"metadata/instance/compute",
		"metadata/instance/network",
	},
	Headers: map[string]string{
		"Metadata":          "true",
		"X-IDENTITY-HEADER": "true",
	},
}

func isCloudMetadataValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}
	body, _ := history.ResponseBody()
	bodyStr := string(body)
	details := fmt.Sprintf("Cloud metadata endpoint exposed: %s\n", history.URL)
	confidence := 30

	indicators := map[string]map[string]string{
		"AWS": {
			"ami-id":               "AMI ID",
			"instance-id":          "Instance ID",
			"security-credentials": "IAM Credentials",
			"security-groups":      "Security Groups",
		},
		"Azure": {
			"azEnvironment":     "Azure Environment",
			"subscriptionId":    "Subscription ID",
			"resourceGroupName": "Resource Group",
			"virtualMachineId":  "VM ID",
			"customData":        "Custom Data",
			"provisioningState": "Provisioning State",
		},
		"GCP": {
			"project-id":            "Project ID",
			"service-accounts":      "Service Accounts",
			"numeric-project-id":    "Numeric Project ID",
			"instance/hostname":     "Instance Hostname",
			"instance/zone":         "Instance Zone",
			"instance/cpu-platform": "CPU Platform",
		},
	}

	headers, _ := history.GetResponseHeadersAsMap()
	providerHeaders := map[string]map[string]string{
		"AWS": {
			"Server":              "EC2",
			"X-Amz-Instance-Type": "",
			"X-Amz-Region":        "",
		},
		"GCP": {
			"Metadata-Flavor":        "Google",
			"X-GCP-Metadata-Request": "",
		},
		"Azure": {
			"X-MSI-Endpoint":    "",
			"X-IDENTITY-HEADER": "",
		},
	}

	for provider, providerIndicators := range indicators {
		foundIndicators := 0
		for indicator, description := range providerIndicators {
			if strings.Contains(bodyStr, indicator) {
				if strings.Contains(strings.ToLower(history.URL), strings.ToLower(indicator)) {
					confidence += 5
				} else {
					confidence += 20
				}
				details += fmt.Sprintf("- Contains %s %s: %s\n", provider, description, indicator)
				foundIndicators++
			}
		}
		if foundIndicators > 0 {
			details += fmt.Sprintf("Found %d %s metadata indicators\n", foundIndicators, provider)
		}
	}

	for provider, expectedHeaders := range providerHeaders {
		for header, expectedValue := range expectedHeaders {
			if value, exists := headers[header]; exists {
				if expectedValue == "" || strings.Contains(strings.Join(value, ""), expectedValue) {
					confidence += 20
					details += fmt.Sprintf("- Found %s metadata header: %s\n", provider, header)
				}
			}
		}
	}

	sensitivePatterns := []string{
		"accessKeyId",
		"secretAccessKey",
		"private_key",
		"bearer_token",
		"api_token",
		"password",
		"credentials",
		"secret",
	}

	for _, pattern := range sensitivePatterns {
		lowerPattern := strings.ToLower(pattern)
		if strings.Contains(strings.ToLower(bodyStr), lowerPattern) {
			if strings.Contains(strings.ToLower(history.URL), lowerPattern) {
				confidence += 2
			} else {
				confidence += 5
			}
			details += fmt.Sprintf("- Contains sensitive information: %s\n", pattern)
		}
	}

	return confidence >= 50, details, min(confidence, 100)
}

func DiscoverCloudMetadata(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	providers := []CloudProvider{AWSMetadata, GCPMetadata, AzureMetadata}
	var allResults []DiscoverAndCreateIssueResults

	for _, provider := range providers {
		result, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
			DiscoveryInput: DiscoveryInput{
				URL:                    options.BaseURL,
				Method:                 "GET",
				Paths:                  provider.Paths,
				Concurrency:            DefaultConcurrency,
				Timeout:                5,
				Headers:                provider.Headers,
				HistoryCreationOptions: options.HistoryCreationOptions,
				HttpClient:             options.HttpClient,
				SiteBehavior:           options.SiteBehavior,
			},
			ValidationFunc: isCloudMetadataValidationFunc,
			IssueCode:      db.ExposedCloudMetadataCode,
		})

		if err != nil {
			continue
		}
		allResults = append(allResults, result)
	}

	finalResult := DiscoverAndCreateIssueResults{}
	for _, result := range allResults {
		finalResult.DiscoverResults.Responses = append(finalResult.DiscoverResults.Responses, result.DiscoverResults.Responses...)
		finalResult.DiscoverResults.Errors = append(finalResult.DiscoverResults.Errors, result.DiscoverResults.Errors...)
		finalResult.Issues = append(finalResult.Issues, result.Issues...)
	}

	return finalResult, nil
}
