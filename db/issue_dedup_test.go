package db

import (
	"testing"

	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateIssue_DedupSuppressesDuplicateWithNilPointers guards D11: the dedup
// lookup in CreateIssue must treat nil id pointers as SQL NULL (IS NULL), not
// "col = NULL" (which is never true). HTTP issues have nil task_id/task_job_id/
// websocket_connection_id, so the naive "col = ?" form never matched and identical
// findings piled up as duplicates.
func TestCreateIssue_DedupSuppressesDuplicateWithNilPointers(t *testing.T) {
	conn := Connection()
	ws, err := conn.GetOrCreateWorkspace(&Workspace{
		Code:        "issue-dedup-d11",
		Title:       "issue dedup d11",
		Description: "D11 dedup test",
	})
	require.NoError(t, err)

	marker := lib.GenerateRandomLowercaseString(12)
	title := "D11 Dedup " + marker
	wsID := ws.ID
	newIssue := func() Issue {
		return Issue{
			Code:        "sql_injection",
			Title:       title,
			Details:     "evidence " + marker,
			URL:         "http://dedup.test/" + marker,
			StatusCode:  200,
			HTTPMethod:  "GET",
			Payload:     "' OR '1'='1",
			Confidence:  90,
			Severity:    NewSeverity("High"),
			WorkspaceID: &wsID,
			// task_id, task_job_id, scan_id, scan_job_id, websocket_connection_id all nil
		}
	}

	t.Cleanup(func() {
		conn.DB().Where("title = ?", title).Delete(&Issue{})
	})

	first, err := conn.CreateIssue(newIssue())
	require.NoError(t, err)
	require.NotZero(t, first.ID)

	second, err := conn.CreateIssue(newIssue())
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "identical issue with nil id pointers must be deduplicated")

	var count int64
	require.NoError(t, conn.DB().Model(&Issue{}).Where("title = ?", title).Count(&count).Error)
	assert.Equal(t, int64(1), count, "expected exactly one issue row after dedup")
}
