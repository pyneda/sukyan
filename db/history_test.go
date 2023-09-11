package db

import (
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	"testing"
)

func TestGetChildrenHistories(t *testing.T) {
	parent := &History{Depth: 1, URL: "/test"}
	Connection.CreateHistory(parent)

	child1 := &History{Depth: 2, URL: "/test/child1"}
	child2 := &History{Depth: 2, URL: "/test/child2"}
	Connection.CreateHistory(child1)
	Connection.CreateHistory(child2)

	children, err := Connection.GetChildrenHistories(parent)
	assert.Nil(t, err)
	assert.Equal(t, true, len(children) >= 2)
}

func TestGetRootHistoryNodes(t *testing.T) {
	workspaceID := uint(1)
	root1 := &History{Depth: 0, URL: "/root1/", WorkspaceID: &workspaceID}
	root2 := &History{Depth: 0, URL: "/root2/", WorkspaceID: &workspaceID}
	Connection.CreateHistory(root1)
	Connection.CreateHistory(root2)

	roots, err := Connection.GetRootHistoryNodes(workspaceID)
	assert.Nil(t, err)
	assert.Equal(t, true, len(roots) >= 2)
}

func TestGetHistoriesByID(t *testing.T) {
	history1 := &History{URL: "/test1"}
	history2 := &History{URL: "/test2"}
	Connection.CreateHistory(history1)
	Connection.CreateHistory(history2)

	ids := []uint{history1.ID, history2.ID}
	histories, err := Connection.GetHistoriesByID(ids)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(histories))
}

func TestGetResponseHeadersAsMap(t *testing.T) {
	history := &History{
		ResponseHeaders: datatypes.JSON(`{"Content-Type": ["application/json"], "Authorization": ["Bearer token"]}`),
	}

	headers, err := history.GetResponseHeadersAsMap()
	assert.Nil(t, err)
	assert.Equal(t, "application/json", headers["Content-Type"][0])
	assert.Equal(t, "Bearer token", headers["Authorization"][0])
}

func TestGetRequestHeadersAsMap(t *testing.T) {
	history := &History{
		RequestHeaders: datatypes.JSON(`{"User-Agent": ["TestAgent"], "Accept": ["application/json"]}`),
	}

	headers, err := history.GetRequestHeadersAsMap()
	assert.Nil(t, err)
	assert.Equal(t, "TestAgent", headers["User-Agent"][0])
	assert.Equal(t, "application/json", headers["Accept"][0])
}

func TestGetHistoryByID(t *testing.T) {
	history := &History{URL: "/test3"}
	Connection.CreateHistory(history)

	fetchedHistory, err := Connection.GetHistoryByID(history.ID)
	assert.Nil(t, err)
	assert.Equal(t, history.ID, fetchedHistory.ID)
}
