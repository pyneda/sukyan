package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
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

	confidence := 50
	details := fmt.Sprintf("Version control file exposed: %s\n", history.URL)

	bodyStr := strings.ToLower(string(history.ResponseBody))
	if strings.Contains(bodyStr, "[core]") ||
		strings.Contains(bodyStr, "ref: refs/") ||
		strings.Contains(bodyStr, "[remote") {
		confidence = 100
		details += "File contains version control system data\n"
	}

	if strings.Contains(strings.ToLower(history.ResponseContentType), "text/plain") {
		confidence += 5
	}

	if confidence > 100 {
		confidence = 100
	}

	return true, details, confidence
}

func DiscoverVersionControlFiles(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       VersionControlPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsVersionControlFileValidationFunc,
		IssueCode:      db.VersionControlFileDetectedCode,
	})
}
