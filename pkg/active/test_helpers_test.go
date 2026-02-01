package active

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func cleanupIssues(t *testing.T, code db.IssueCode) {
	t.Helper()
	db.Connection().DB().Where("code = ?", string(code)).Delete(&db.Issue{})
}
