package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var PHPInfoPaths = []string{
	"phpinfo.php",
	"info.php",
	"php_info.php",
	"test.php",
	"i.php",
	"php/phpinfo.php",
	"php/info.php",
	"phpinfo",
	"test/phpinfo.php",
}

func isPHPInfoValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("PHPInfo page found: %s\n", history.URL)
	confidence := 0

	phpInfoIndicators := []string{
		"<title>phpinfo()</title>",
		"PHP Version",
		"<h1 class=\"p\">PHP Version",
		"PHP Extension",
		"PHP License",
		"<h2>PHP License</h2>",
		"module_Zend Optimizer",
		"This program makes use of the Zend Scripting Language Engine",
		"PHP Credits",
		"Configuration File (php.ini) Path",
	}

	for _, indicator := range phpInfoIndicators {
		if strings.Contains(bodyStr, indicator) {
			confidence += 20
			details += fmt.Sprintf("- Contains phpinfo() indicator: %s\n", indicator)
		}
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 40 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverPHPInfo(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       PHPInfoPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: isPHPInfoValidationFunc,
		IssueCode:      db.PhpInfoDetectedCode,
	})
}
