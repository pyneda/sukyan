package db

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestGetChildrenHistories(t *testing.T) {
	workspace, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	workspaceID := workspace.ID

	parent := &History{Depth: 1, URL: "/test", WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(parent)
	assert.Nil(t, err)

	child1 := &History{Depth: 2, URL: "/test/child1", WorkspaceID: &workspaceID}
	child2 := &History{Depth: 2, URL: "/test/child2", WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(child1)
	assert.Nil(t, err)
	_, err = Connection.CreateHistory(child2)
	assert.Nil(t, err)

	children, err := Connection.GetChildrenHistories(parent)
	assert.Nil(t, err)
	assert.Equal(t, true, len(children) >= 2)
}

func TestCreateHistoryIgnoredExtensions(t *testing.T) {
	workspace, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.extensions", []string{".jpg", ".png"})
	ignoredExtensions := viper.GetStringSlice("history.responses.ignored.extensions")
	assert.Contains(t, ignoredExtensions, ".jpg")
	history := &History{URL: "/test.jpg", ResponseBody: []byte("image data"), WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(history)
	assert.Nil(t, err)
	assert.Equal(t, "", string(history.ResponseBody))
	assert.Equal(t, "Response body was removed due to ignored file extension: .jpg", history.Note)
}

func TestCreateHistoryIgnoredContentTypes(t *testing.T) {
	workspace, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.content_types", []string{"image"})
	ignoredContentTypes := viper.GetStringSlice("history.responses.ignored.content_types")
	assert.Contains(t, ignoredContentTypes, "image")
	history := &History{URL: "/test-image", ResponseContentType: "image/jpeg", ResponseBody: []byte("image data"), WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(history)
	assert.Nil(t, err)
	assert.Equal(t, "", string(history.ResponseBody))
	assert.Equal(t, "Response body was removed due to ignored content type: image", history.Note)
}

func TestCreateHistoryIgnoredMaxSize(t *testing.T) {
	workspace, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.max_size", 10)
	maxSize := viper.GetInt("history.responses.ignored.max_size")
	assert.Equal(t, 10, maxSize)
	history := &History{URL: "/test.html", ResponseBody: []byte("12345678901"), WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(history)
	assert.Nil(t, err)
	assert.Equal(t, "", string(history.ResponseBody))
	assert.Equal(t, "Response body was removed due to exceeding max size limit.", history.Note)
}

func TestGetRootHistoryNodes(t *testing.T) {
	workspace, err := Connection.CreateDefaultWorkspace()
	assert.Nil(t, err)
	workspaceID := workspace.ID

	root1 := &History{Depth: 0, URL: "/root1/", WorkspaceID: &workspaceID}
	root2 := &History{Depth: 0, URL: "/root2/", WorkspaceID: &workspaceID}
	_, err = Connection.CreateHistory(root1)
	assert.Nil(t, err)
	_, err = Connection.CreateHistory(root2)
	assert.Nil(t, err)

	roots, err := Connection.GetRootHistoryNodes(workspaceID)
	assert.Nil(t, err)
	assert.Equal(t, true, len(roots) >= 2)
}

func TestGetHistoriesByID(t *testing.T) {
	workspace, err := Connection.GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetHistoriesByID",
		Title:       "TestGetHistoriesByID",
		Description: "TestGetHistoriesByID",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	history1 := &History{URL: "/test1", WorkspaceID: &workspaceID}
	history2 := &History{URL: "/test2", WorkspaceID: &workspaceID}
	history1, err = Connection.CreateHistory(history1)
	assert.Nil(t, err)
	history2, err = Connection.CreateHistory(history2)
	assert.Nil(t, err)

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
	workspace, err := Connection.GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetHistoryByID",
		Title:       "TestGetHistoryByID",
		Description: "TestGetHistoryByID",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	history := &History{URL: "/test3", WorkspaceID: &workspaceID}
	history, err = Connection.CreateHistory(history)
	assert.Nil(t, err)

	fetchedHistory, err := Connection.GetHistoryByID(history.ID)
	assert.Nil(t, err)
	assert.Equal(t, history.ID, fetchedHistory.ID)
}
