package db

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetChildrenHistories(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "history-test",
		Title:       "history test workspace",
		Description: "Workspace for history validation tests",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	parent := &History{Depth: 1, URL: "/test", WorkspaceID: &workspaceID}
	_, err = Connection().CreateHistory(parent)
	assert.Nil(t, err)

	child1 := &History{Depth: 2, URL: "/test/child1", WorkspaceID: &workspaceID}
	child2 := &History{Depth: 2, URL: "/test/child2", WorkspaceID: &workspaceID}
	_, err = Connection().CreateHistory(child1)
	assert.Nil(t, err)
	_, err = Connection().CreateHistory(child2)
	assert.Nil(t, err)

	children, err := Connection().GetChildrenHistories(parent)
	assert.Nil(t, err)
	assert.Equal(t, true, len(children) >= 2)
}

func TestCreateHistoryIgnoredExtensions(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "history-test",
		Title:       "history test workspace",
		Description: "Workspace for history validation tests",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.extensions", []string{".jpg", ".png"})
	ignoredExtensions := viper.GetStringSlice("history.responses.ignored.extensions")
	assert.Contains(t, ignoredExtensions, ".jpg")

	// Create a raw HTTP response with image data
	rawResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: image/jpeg\r\n\r\nimage data")

	history := &History{
		URL:         "/test.jpg",
		RawResponse: rawResponse,
		WorkspaceID: &workspaceID,
	}

	_, err = Connection().CreateHistory(history)
	assert.Nil(t, err)

	// Check that the body was removed by extracting it from RawResponse
	body, err := history.ResponseBody()
	assert.Nil(t, err)
	assert.Empty(t, body)

	assert.Equal(t, "Response body was removed due to ignored file extension: .jpg", history.Note)
}

func TestCreateHistoryIgnoredContentTypes(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "history-test",
		Title:       "history test workspace",
		Description: "Workspace for history validation tests",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.content_types", []string{"image"})
	ignoredContentTypes := viper.GetStringSlice("history.responses.ignored.content_types")
	assert.Contains(t, ignoredContentTypes, "image")

	rawResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: image/jpeg\r\nContent-Length: 10\r\n\r\nimagedata")

	history := &History{
		URL:                 "/api/images/profile-photo.jpg",
		ResponseContentType: "image/jpeg",
		RawResponse:         rawResponse,
		WorkspaceID:         &workspaceID,
	}

	newHistory, err := Connection().CreateHistory(history)
	assert.Nil(t, err)
	body, err := newHistory.ResponseBody()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(body), "Response body should be empty")
	assert.Equal(t, "Response body was removed due to ignored file extension: .jpg", history.Note)
}

func TestCreateHistoryIgnoredMaxSize(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "history-test",
		Title:       "history test workspace",
		Description: "Workspace for history validation tests",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	viper.Set("history.responses.ignored.max_size", 10)
	maxSize := viper.GetInt("history.responses.ignored.max_size")
	assert.Equal(t, 10, maxSize)

	// Create a raw HTTP response with content exceeding the max size
	rawResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n12345678901")

	history := &History{
		URL:              "/test.html",
		RawResponse:      rawResponse,
		ResponseBodySize: 11, // Set this to trigger the size check
		WorkspaceID:      &workspaceID,
	}

	_, err = Connection().CreateHistory(history)
	assert.Nil(t, err)

	// Check that the body was removed by extracting it from RawResponse
	body, err := history.ResponseBody()
	assert.Nil(t, err)
	assert.Empty(t, body)

	assert.Equal(t, "Response body was removed due to exceeding max size limit.", history.Note)
}

func TestGetRootHistoryNodes(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "history-test",
		Title:       "history test workspace",
		Description: "Workspace for history validation tests",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	root1 := &History{Depth: 0, URL: "/root1/", WorkspaceID: &workspaceID}
	root2 := &History{Depth: 0, URL: "/root2/", WorkspaceID: &workspaceID}
	_, err = Connection().CreateHistory(root1)
	assert.Nil(t, err)
	_, err = Connection().CreateHistory(root2)
	assert.Nil(t, err)

	roots, err := Connection().GetRootHistoryNodes(workspaceID)
	assert.Nil(t, err)
	assert.Equal(t, true, len(roots) >= 2)
}

func TestGetHistoriesByID(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetHistoriesByID",
		Title:       "TestGetHistoriesByID",
		Description: "TestGetHistoriesByID",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	history1 := &History{URL: "/test1", WorkspaceID: &workspaceID}
	history2 := &History{URL: "/test2", WorkspaceID: &workspaceID}
	history1, err = Connection().CreateHistory(history1)
	assert.Nil(t, err)
	history2, err = Connection().CreateHistory(history2)
	assert.Nil(t, err)

	ids := []uint{history1.ID, history2.ID}
	histories, err := Connection().GetHistoriesByID(ids)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(histories))
}

func TestResponseHeaders(t *testing.T) {
	// Create a history with raw response including headers
	history := &History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAuthorization: Bearer token\r\n\r\n{}"),
	}

	// Test the new method
	headers, err := history.ResponseHeaders()
	assert.Nil(t, err)
	assert.Equal(t, "application/json", headers["Content-Type"][0])
	assert.Equal(t, "Bearer token", headers["Authorization"][0])

	// Test backward compatibility with deprecated method
	legacyHeaders, err := history.GetResponseHeadersAsMap()
	assert.Nil(t, err)
	assert.Equal(t, "application/json", legacyHeaders["Content-Type"][0])
	assert.Equal(t, "Bearer token", legacyHeaders["Authorization"][0])
}

func TestRequestHeaders(t *testing.T) {
	// Create a history with raw request including headers
	history := &History{
		RawRequest: []byte("GET / HTTP/1.1\r\nUser-Agent: TestAgent\r\nAccept: application/json\r\n\r\n"),
	}

	// Test the new method
	headers, err := history.RequestHeaders()
	assert.Nil(t, err)
	assert.Equal(t, "TestAgent", headers["User-Agent"][0])
	assert.Equal(t, "application/json", headers["Accept"][0])

	// Test backward compatibility with deprecated method
	legacyHeaders, err := history.GetRequestHeadersAsMap()
	assert.Nil(t, err)
	assert.Equal(t, "TestAgent", legacyHeaders["User-Agent"][0])
	assert.Equal(t, "application/json", legacyHeaders["Accept"][0])
}

func TestGetHistoryByID(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "TestGetHistoryByID",
		Title:       "TestGetHistoryByID",
		Description: "TestGetHistoryByID",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID

	history := &History{URL: "/test3", WorkspaceID: &workspaceID}
	history, err = Connection().CreateHistory(history)
	assert.Nil(t, err)

	fetchedHistory, err := Connection().GetHistoryByID(history.ID)
	assert.Nil(t, err)
	assert.Equal(t, history.ID, fetchedHistory.ID)
}

func TestListHistory(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "list-history-test",
		Title:       "List History Test",
		Description: "Workspace for testing history listing functionality",
	})
	assert.Nil(t, err)
	workspaceID := workspace.ID
	assert.Nil(t, Connection().DB().Unscoped().Where("workspace_id = ?", workspaceID).Delete(&History{}).Error)

	testCases := []struct {
		url         string
		method      string
		statusCode  int
		source      string
		contentType string
		note        string
	}{
		{"/api/users", "GET", 200, "Scanner", "application/json", "User listing endpoint"},
		{"/api/admin", "POST", 403, "Scanner", "application/json", "Failed admin access"},
		{"/images/logo.png", "GET", 200, "Crawler", "image/png", "Website logo"},
		{"/api/products", "PUT", 500, "Repeater", "application/json", "Server error in products API"},
		{"/docs/index.html", "GET", 404, "Browser", "text/html", "Missing documentation"},
		{"/api/auth", "POST", 401, "Scanner", "application/json", "Invalid authentication attempt"},
		{"/api/users/search", "GET", 200, "Browser", "application/json", "Search functionality test"},
	}

	createdIDs := make([]uint, 0)
	for _, tc := range testCases {
		history := &History{
			URL:                 tc.url,
			Method:              tc.method,
			StatusCode:          tc.statusCode,
			Source:              tc.source,
			ResponseContentType: tc.contentType,
			Note:                tc.note,
			WorkspaceID:         &workspaceID,
		}
		created, err := Connection().CreateHistory(history)
		assert.Nil(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	// Test cases for filtering
	t.Run("Query Filter", func(t *testing.T) {
		filter := HistoryFilter{
			Query:       "api",
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(5), count)
		for _, item := range items {
			assert.Contains(t, strings.ToLower(item.URL), "api")
		}
	})

	t.Run("Status Code Filter", func(t *testing.T) {
		filter := HistoryFilter{
			StatusCodes: []int{200},
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(3), count)
		for _, item := range items {
			assert.Equal(t, 200, item.StatusCode)
		}
	})

	t.Run("Method Filter", func(t *testing.T) {
		filter := HistoryFilter{
			Methods:     []string{"GET"},
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(4), count)
		for _, item := range items {
			assert.Equal(t, "GET", item.Method)
		}
	})

	t.Run("Source Filter", func(t *testing.T) {
		filter := HistoryFilter{
			Sources:     []string{"Scanner"},
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(3), count)
		for _, item := range items {
			assert.Equal(t, "Scanner", item.Source)
		}
	})

	t.Run("Content Type Filter", func(t *testing.T) {
		filter := HistoryFilter{
			ResponseContentTypes: []string{"application/json"},
			WorkspaceID:          workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(5), count)
		for _, item := range items {
			assert.Equal(t, "application/json", item.ResponseContentType)
		}
	})

	t.Run("Combined Filters", func(t *testing.T) {
		filter := HistoryFilter{
			Query:       "api",
			Methods:     []string{"GET"},
			StatusCodes: []int{200},
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(2), count)
		for _, item := range items {
			assert.Contains(t, strings.ToLower(item.URL), "api")
			assert.Equal(t, "GET", item.Method)
			assert.Equal(t, 200, item.StatusCode)
		}
	})

	t.Run("Note Search", func(t *testing.T) {
		filter := HistoryFilter{
			Query:       "error",
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(1), count)
		assert.Contains(t, items[0].Note, "error")
	})

	t.Run("Pagination", func(t *testing.T) {
		filter := HistoryFilter{
			WorkspaceID: workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 3,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.Equal(t, int64(7), count)
		assert.Equal(t, 3, len(items))
	})

	t.Run("Sort By URL", func(t *testing.T) {
		filter := HistoryFilter{
			WorkspaceID: workspaceID,
			SortBy:      "url",
			SortOrder:   "asc",
			Pagination: Pagination{
				Page:     1,
				PageSize: 10,
			},
		}
		items, _, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		for i := 1; i < len(items); i++ {
			assert.True(t, items[i-1].URL <= items[i].URL)
		}
	})

	t.Run("Date Range Filter", func(t *testing.T) {
		testTime := time.Now()
		beforeTime := testTime.Add(-30 * time.Minute)
		afterTime := testTime.Add(30 * time.Minute)

		filter := HistoryFilter{
			CreatedAfter: &beforeTime,
			WorkspaceID:  workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 100,
			},
		}
		items, count, err := Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.True(t, count > 0)
		for _, item := range items {
			assert.True(t, item.CreatedAt.After(beforeTime) || item.CreatedAt.Equal(beforeTime))
		}

		filter = HistoryFilter{
			CreatedBefore: &afterTime,
			WorkspaceID:   workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 100,
			},
		}
		items, count, err = Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.True(t, count > 0)
		for _, item := range items {
			assert.True(t, item.CreatedAt.Before(afterTime) || item.CreatedAt.Equal(afterTime))
		}

		filter = HistoryFilter{
			CreatedAfter:  &beforeTime,
			CreatedBefore: &afterTime,
			WorkspaceID:   workspaceID,
			Pagination: Pagination{
				Page:     1,
				PageSize: 100,
			},
		}
		items, count, err = Connection().ListHistory(filter)
		assert.Nil(t, err)
		assert.True(t, count > 0)
		for _, item := range items {
			assert.True(t, (item.CreatedAt.After(beforeTime) || item.CreatedAt.Equal(beforeTime)) &&
				(item.CreatedAt.Before(afterTime) || item.CreatedAt.Equal(afterTime)))
		}
	})

	// Cleanup test data
	for _, id := range createdIDs {
		err := Connection().DB().Unscoped().Delete(&History{}, id).Error
		assert.Nil(t, err)
	}
	err = Connection().DeleteWorkspace(workspaceID)
	assert.Nil(t, err)
}
