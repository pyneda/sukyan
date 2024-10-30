package discovery

import (
	"fmt"

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
	if history.StatusCode == 200 {
		details := fmt.Sprintf("Exposed version control file detected: %s\n", history.URL)
		return true, details, 90
	}
	return false, "", 0
}

func DiscoverVersionControlFiles(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       VersionControlPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsVersionControlFileValidationFunc,
		IssueCode:      db.VersionControlFileDetectedCode,
	})
}
