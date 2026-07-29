package db

import (
	"strconv"
	"testing"

	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorkspaceRollups_CountsIssuesBySeverity(t *testing.T) {
	conn := Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	ws, err := conn.GetOrCreateWorkspace(&Workspace{
		Code:        "rollup-" + marker,
		Title:       "rollup " + marker,
		Description: "workspace rollup test",
	})
	require.NoError(t, err)
	wsID := ws.ID

	t.Cleanup(func() {
		conn.DB().Where("workspace_id = ?", wsID).Delete(&Issue{})
		conn.DB().Delete(&Workspace{}, wsID)
	})

	for i, sev := range []severity{Critical, Critical, High, Low} {
		suffix := strconv.Itoa(i)
		_, err := conn.CreateIssue(Issue{
			Code:        "sql_injection",
			Title:       "rollup " + marker + " " + string(sev) + " " + suffix,
			Details:     "detail " + marker + " " + string(sev) + " " + suffix,
			URL:         "http://rollup.test/" + marker + "/" + string(sev) + "/" + suffix,
			StatusCode:  200,
			HTTPMethod:  "GET",
			Confidence:  90,
			Severity:    sev,
			WorkspaceID: &wsID,
		})
		require.NoError(t, err)
	}

	rows, count, err := conn.ListWorkspaceRollups(WorkspaceRollupFilter{
		Query:      "rollup " + marker,
		Pagination: Pagination{Page: 1, PageSize: 25},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, wsID, row.WorkspaceID)
	assert.EqualValues(t, 2, row.Issues.Critical)
	assert.EqualValues(t, 1, row.Issues.High)
	assert.EqualValues(t, 1, row.Issues.Low)
	assert.EqualValues(t, 0, row.Issues.Medium)
	assert.EqualValues(t, 4, row.IssuesCount)
}

func TestListWorkspaceRollups_IncludesWorkspaceWithNoActivity(t *testing.T) {
	conn := Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	ws, err := conn.GetOrCreateWorkspace(&Workspace{
		Code:        "empty-" + marker,
		Title:       "empty " + marker,
		Description: "empty workspace rollup test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { conn.DB().Delete(&Workspace{}, ws.ID) })

	rows, count, err := conn.ListWorkspaceRollups(WorkspaceRollupFilter{
		Query:      "empty " + marker,
		Pagination: Pagination{Page: 1, PageSize: 25},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Len(t, rows, 1)

	assert.EqualValues(t, 0, rows[0].IssuesCount)
	assert.EqualValues(t, 0, rows[0].HistoryCount)
	assert.EqualValues(t, 0, rows[0].ActiveScanCount)
	assert.Nil(t, rows[0].LastActivityAt)
}

func TestListWorkspaceRollups_SortsByCriticalDescending(t *testing.T) {
	conn := Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	low, err := conn.GetOrCreateWorkspace(&Workspace{
		Code: "sortlow-" + marker, Title: "sortws " + marker + " low", Description: "sort test",
	})
	require.NoError(t, err)
	high, err := conn.GetOrCreateWorkspace(&Workspace{
		Code: "sorthigh-" + marker, Title: "sortws " + marker + " high", Description: "sort test",
	})
	require.NoError(t, err)
	lowID, highID := low.ID, high.ID

	t.Cleanup(func() {
		conn.DB().Where("workspace_id IN ?", []uint{lowID, highID}).Delete(&Issue{})
		conn.DB().Delete(&Workspace{}, lowID)
		conn.DB().Delete(&Workspace{}, highID)
	})

	mk := func(wsID uint, n int) {
		for i := 0; i < n; i++ {
			_, err := conn.CreateIssue(Issue{
				Code:        "sql_injection",
				Title:       "sortws " + marker + " " + string(rune('a'+i)),
				Details:     "d" + marker + string(rune('a'+i)),
				URL:         "http://sort.test/" + marker + "/" + string(rune('a'+i)),
				StatusCode:  200,
				HTTPMethod:  "GET",
				Confidence:  90,
				Severity:    Critical,
				WorkspaceID: &wsID,
			})
			require.NoError(t, err)
		}
	}
	mk(lowID, 1)
	mk(highID, 3)

	rows, _, err := conn.ListWorkspaceRollups(WorkspaceRollupFilter{
		Query:      "sortws " + marker,
		SortBy:     "critical",
		SortOrder:  "desc",
		Pagination: Pagination{Page: 1, PageSize: 25},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, highID, rows[0].WorkspaceID)
	assert.Equal(t, lowID, rows[1].WorkspaceID)
}

func TestListWorkspaceRollups_Paginates(t *testing.T) {
	conn := Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	var ids []uint
	for i := 0; i < 3; i++ {
		ws, err := conn.GetOrCreateWorkspace(&Workspace{
			Code:        "page-" + marker + "-" + string(rune('a'+i)),
			Title:       "pagews " + marker + " " + string(rune('a'+i)),
			Description: "pagination test",
		})
		require.NoError(t, err)
		ids = append(ids, ws.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			conn.DB().Delete(&Workspace{}, id)
		}
	})

	rows, count, err := conn.ListWorkspaceRollups(WorkspaceRollupFilter{
		Query:      "pagews " + marker,
		SortBy:     "title",
		SortOrder:  "asc",
		Pagination: Pagination{Page: 2, PageSize: 2},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)
	assert.Len(t, rows, 1)
}
