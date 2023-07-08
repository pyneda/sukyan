package passive

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"strings"
)

func MissconfigurationScan(item *db.History) {
	apacheStrutsDevModeScan(item)
	djangoDebugPageExceptionScan(item)
}

func apacheStrutsDevModeScan(item *db.History) {
	strutsDevMode := "<title>Struts Problem Report</title>"
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	if strings.Contains(matchAgainst, strutsDevMode) {
		details := fmt.Sprintf("Apache Struts Dev Mode Detected in response for %s", item.URL)
		db.CreateIssueFromHistoryAndTemplate(item, db.ApacheStrutsDevModeCode, details, 90, "")
	}
}

func djangoDebugPageExceptionScan(item *db.History) {
	djangoDebugException := "You're seeing this error because you have <code>DEBUG = True</code> in your Django settings file."
	matchAgainst := string(item.RawResponse)
	if matchAgainst == "" {
		matchAgainst = string(item.ResponseBody)
	}
	if strings.Contains(matchAgainst, djangoDebugException) {
		details := fmt.Sprintf("Django Debug Page Exception Detected in response for %s", item.URL)
		db.CreateIssueFromHistoryAndTemplate(item, db.DjangoDebugExceptionCode, details, 90, "")
	}
}
