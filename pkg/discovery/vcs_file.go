package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var VersionControlPaths = []string{
	".git/",
	".git/config",
	".git/HEAD",
	".gitignore",
	".gitattributes",
	".svn/",
	".svn/entries",
	".hg/",
	".hg/hgrc",
	".bzr/",
	".bzr/branch",
	".cvs/",
	"CVS/Entries",
	".gitmodules",
	".gitkeep",
	".Rhistory",
	".DS_Store",
	".project",
	".classpath",
	".idea/",
	".vscode/",
	".hg/store",
	"CVS/Repository",
	".bzr/checkout",
	".bzr/repository",
}

func IsVersionControlFileValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	confidence := 30
	details := fmt.Sprintf("Version control file exposed: %s\n", history.URL)

	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 30
		details += "- Correct content type for version control files\n"
	}

	if strings.Contains(history.ResponseContentType, "application/xml") ||
		strings.Contains(history.ResponseContentType, "text/xml") {
		confidence += 30
		details += "- Correct content type for version control files\n"
	}

	if strings.Contains(history.ResponseContentType, "application/octet-stream") {
		confidence += 30
		details += "- Correct content type for version control files\n"
	}

	bodyStr := strings.ToLower(string(history.ResponseBody))
	if strings.Contains(bodyStr, "[core]") ||
		strings.Contains(bodyStr, "ref: refs/") ||
		strings.Contains(bodyStr, "[remote") {
		confidence = 100
		details += "File contains version control system data\n"
	}

	return confidence >= minConfidence(), details, min(confidence, 100)
}

func DiscoverVersionControlFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       VersionControlPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsVersionControlFileValidationFunc,
		IssueCode:      db.VersionControlFileDetectedCode,
	})
}
