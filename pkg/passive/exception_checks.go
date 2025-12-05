package passive

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

func ExceptionsScan(item *db.History) {
	apacheTapestryExceptionScan(item)
	grailsExceptionScan(item)
}

func apacheTapestryExceptionScan(item *db.History) {
	tapestryException := "<h1 class=\"t-exception-report\">An unexpected application exception has occurred.</h1>"
	matchAgainst := string(item.RawResponse)

	if strings.Contains(matchAgainst, tapestryException) {
		details := fmt.Sprintf("Apache Tapestry Exception Detected in response for %s", item.URL)
		db.CreateIssueFromHistoryAndTemplate(item, db.ApacheTapestryExceptionCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}

func grailsExceptionScan(item *db.History) {
	grailsException := "<h1>Grails Runtime Exception</h1>"
	matchAgainst := string(item.RawResponse)

	if strings.Contains(matchAgainst, grailsException) {
		details := fmt.Sprintf("Grails Runtime Exception Detected in response for %s", item.URL)
		db.CreateIssueFromHistoryAndTemplate(item, db.GrailsExceptionCode, details, 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}
