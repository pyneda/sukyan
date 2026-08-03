package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCountedOOBTests(t *testing.T) (*Workspace, func()) {
	t.Helper()

	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceOOBInteractionCounts",
		Code:        "test-workspace-oob-counts-" + uuid.New().String(),
		Description: "Test workspace for OOB interaction counts",
	})
	require.NoError(t, err)

	return workspace, func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&OOBInteraction{})
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&OOBTest{})
		Connection().DeleteWorkspace(workspace.ID)
	}
}

func createCountedOOBTest(t *testing.T, workspaceID uint, name string, callbacks int) *OOBTest {
	t.Helper()

	test, err := Connection().CreateOOBTest(OOBTest{
		Code:              "os_cmd_injection",
		TestName:          name,
		Target:            "https://counts.test/ping",
		InteractionDomain: "counts.oast.test",
		InteractionFullID: "counts-" + uuid.NewString(),
		Payload:           ";curl counts.oast.test",
		InsertionPoint:    "body",
		WorkspaceID:       &workspaceID,
	})
	require.NoError(t, err)

	for i := 0; i < callbacks; i++ {
		_, err := Connection().CreateInteraction(&OOBInteraction{
			OOBTestID:     &test.ID,
			Protocol:      "dns",
			FullID:        "counts-full-" + uuid.NewString(),
			UniqueID:      "counts-uniq-" + uuid.NewString(),
			QType:         "A",
			RemoteAddress: "203.0.113.10",
			Timestamp:     time.Now(),
			WorkspaceID:   &workspaceID,
		})
		require.NoError(t, err)
	}

	return &test
}

// InteractionsCount is derived rather than stored, so nothing populates it unless a
// read path asks for it. The listing previously returned no such field at all, which
// left every row in the UI claiming no callbacks had come back.
func TestListOOBTestsPopulatesInteractionCounts(t *testing.T) {
	workspace, cleanup := setupCountedOOBTests(t)
	defer cleanup()

	withCallbacks := createCountedOOBTest(t, workspace.ID, "Has callbacks", 3)
	withOne := createCountedOOBTest(t, workspace.ID, "Has one callback", 1)
	withNone := createCountedOOBTest(t, workspace.ID, "Has no callbacks", 0)

	items, count, err := Connection().ListOOBTests(OOBTestsFilter{
		WorkspaceID: workspace.ID,
		Pagination:  Pagination{Page: 1, PageSize: 50},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

	counts := make(map[uint]int, len(items))
	for _, item := range items {
		counts[item.ID] = item.InteractionsCount
	}

	assert.Equal(t, 3, counts[withCallbacks.ID])
	assert.Equal(t, 1, counts[withOne.ID])
	assert.Equal(t, 0, counts[withNone.ID], "a test with no callbacks must report zero, not the previous row's count")
}

// The count is keyed on the ids actually returned, so paging must not leak a
// neighbouring page's totals onto the rows in hand.
func TestListOOBTestsCountsOnlyTheReturnedPage(t *testing.T) {
	workspace, cleanup := setupCountedOOBTests(t)
	defer cleanup()

	createCountedOOBTest(t, workspace.ID, "First", 2)
	createCountedOOBTest(t, workspace.ID, "Second", 5)

	items, _, err := Connection().ListOOBTests(OOBTestsFilter{
		WorkspaceID: workspace.ID,
		SortBy:      "id",
		SortOrder:   "asc",
		Pagination:  Pagination{Page: 1, PageSize: 1},
	})
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, 2, items[0].InteractionsCount)
}

func TestLoadInteractionCountsHandlesAnEmptyPage(t *testing.T) {
	assert.NoError(t, Connection().LoadInteractionCounts(nil))
	assert.NoError(t, Connection().LoadInteractionCounts([]*OOBTest{}))
}
