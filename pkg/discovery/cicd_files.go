package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var CICDBuildFilePaths = []string{
	".travis.yml",
	"circle.yml",
	"Jenkinsfile",
	".gitlab-ci.yml",
	"buildspec.yml",
	"build.gradle",
	"pom.xml",
	"Makefile",
	"docker-compose.yml",
	"docker-compose.override.yml",
	"Dockerfile",
	"cloudbuild.yaml",
	"azure-pipelines.yml",
	"bitbucket-pipelines.yml",
	"appveyor.yml",
	"terraform.tf",
	"terraform.tfvars",
	"kustomization.yaml",
	"teamcity.yml",
	"wercker.yml",
	".github/workflows/",
	"build.xml",
	"gruntfile.js",
	"gulpfile.js",
	"ecosystem.config.js",
	"compose.yaml",
	"docker-compose.ci.yml",
	"helm/values.yaml",
	"skaffold.yaml",
	"ansible.cfg",
	"inventory",
	"packer.json",
	"vars.tf",
}

func IsCICDBuildFileValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		details := fmt.Sprintf("Exposed CI/CD or infrastructure configuration file detected: %s\n", history.URL)
		confidence := 70

		if history.ResponseContentType == "text/yaml" || history.ResponseContentType == "application/json" {
			confidence += 10
			details += "- Content-Type indicates configuration format\n"
		}

		sensitiveIndicators := []string{
			"aws_access_key_id", "aws_secret_access_key", "secret_key", "client_id",
			"client_secret", "access_token", "private_key", "consumer_key",
			"consumer_secret", "db_password", "db_username", "db_host",
			"auth_token", "api_key", "encryption_key", "slack_token",
			"password", "oauth_token", "jwt_secret", "smtp_password",
		}

		bodyStr := strings.ToLower(string(history.ResponseBody))
		for _, indicator := range sensitiveIndicators {
			if strings.Contains(bodyStr, strings.ToLower(indicator)) {
				confidence = min(confidence+5, 100)
				details += fmt.Sprintf("- Contains sensitive indicator: %s\n", indicator)
			}
		}

		return true, details, confidence
	}
	return false, "", 0
}

func DiscoverCICDBuildFiles(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       CICDBuildFilePaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsCICDBuildFileValidationFunc,
		IssueCode:      db.CiCdInfrastructureFileDetectedCode,
	})
}