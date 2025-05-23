package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
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

	body, _ := history.ResponseBody()
	bodyStr := string(body)
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

func DiscoverPHPInfo(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       PHPInfoPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: isPHPInfoValidationFunc,
		IssueCode:      db.PhpInfoDetectedCode,
	})
}
