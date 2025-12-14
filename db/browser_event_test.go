package db

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserEventCreateAndFetch(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events Workspace",
		Description: "Temporary workspace for browser events tests",
	})
	require.NoError(t, err)

	eventData := map[string]interface{}{
		"message": "Test console log",
		"level":   "info",
	}
	dataBytes, err := json.Marshal(eventData)
	require.NoError(t, err)

	event := &BrowserEvent{
		EventType:   BrowserEventConsole,
		Category:    BrowserEventCategoryRuntime,
		URL:         "https://example.com/test",
		Description: "Console log message",
		Data:        dataBytes,
		WorkspaceID: workspace.ID,
		Source:      "test",
	}

	// Create
	err = Connection().SaveBrowserEvent(event)
	assert.NoError(t, err)
	assert.NotEqual(t, "", event.ID.String())
	assert.Equal(t, 1, event.OccurrenceCount)
	assert.False(t, event.FirstSeenAt.IsZero())
	assert.False(t, event.LastSeenAt.IsZero())

	// Fetch
	fetched, err := Connection().GetBrowserEvent(event.ID)
	assert.NoError(t, err)
	assert.Equal(t, event.ID, fetched.ID)
	assert.Equal(t, event.EventType, fetched.EventType)
	assert.Equal(t, event.Category, fetched.Category)
	assert.Equal(t, event.URL, fetched.URL)
	assert.Equal(t, event.Description, fetched.Description)
	assert.Equal(t, event.WorkspaceID, fetched.WorkspaceID)
	assert.Equal(t, event.Source, fetched.Source)
}

func TestBrowserEventAggregation(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-aggregation-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events Aggregation Workspace",
		Description: "Temporary workspace for browser events aggregation tests",
	})
	require.NoError(t, err)

	eventData := map[string]interface{}{
		"message": "Repeated console log",
		"level":   "warning",
	}
	dataBytes, err := json.Marshal(eventData)
	require.NoError(t, err)

	// Create first event
	event1 := &BrowserEvent{
		EventType:   BrowserEventConsole,
		Category:    BrowserEventCategoryRuntime,
		URL:         "https://example.com/aggregation-test",
		Description: "Repeated console message",
		Data:        dataBytes,
		WorkspaceID: workspace.ID,
		Source:      "test",
	}

	err = Connection().SaveBrowserEvent(event1)
	require.NoError(t, err)
	originalID := event1.ID
	originalFirstSeen := event1.FirstSeenAt

	// Wait a moment to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Create duplicate event (same content hash)
	event2 := &BrowserEvent{
		EventType:   BrowserEventConsole,
		Category:    BrowserEventCategoryRuntime,
		URL:         "https://example.com/aggregation-test",
		Description: "Repeated console message", // Different description shouldn't matter
		Data:        dataBytes,
		WorkspaceID: workspace.ID,
		Source:      "test",
	}

	err = Connection().SaveBrowserEvent(event2)
	require.NoError(t, err)

	// Fetch the original event and verify aggregation
	fetched, err := Connection().GetBrowserEvent(originalID)
	assert.NoError(t, err)
	assert.Equal(t, 2, fetched.OccurrenceCount)
	assert.Equal(t, originalFirstSeen.Unix(), fetched.FirstSeenAt.Unix())
	assert.True(t, fetched.LastSeenAt.After(fetched.FirstSeenAt) || fetched.LastSeenAt.Equal(fetched.FirstSeenAt))
}

func TestBrowserEventDifferentContentHash(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-different-hash-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events Different Hash Workspace",
		Description: "Temporary workspace for browser events different hash tests",
	})
	require.NoError(t, err)

	eventData1 := map[string]interface{}{
		"message": "First message",
	}
	dataBytes1, _ := json.Marshal(eventData1)

	eventData2 := map[string]interface{}{
		"message": "Second message",
	}
	dataBytes2, _ := json.Marshal(eventData2)

	// Create first event
	event1 := &BrowserEvent{
		EventType:   BrowserEventConsole,
		Category:    BrowserEventCategoryRuntime,
		URL:         "https://example.com/different-hash",
		Data:        dataBytes1,
		WorkspaceID: workspace.ID,
		Source:      "test",
	}
	err = Connection().SaveBrowserEvent(event1)
	require.NoError(t, err)

	// Create second event with different data (different hash)
	event2 := &BrowserEvent{
		EventType:   BrowserEventConsole,
		Category:    BrowserEventCategoryRuntime,
		URL:         "https://example.com/different-hash",
		Data:        dataBytes2,
		WorkspaceID: workspace.ID,
		Source:      "test",
	}
	err = Connection().SaveBrowserEvent(event2)
	require.NoError(t, err)

	// Both events should exist separately
	fetched1, err := Connection().GetBrowserEvent(event1.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, fetched1.OccurrenceCount)

	fetched2, err := Connection().GetBrowserEvent(event2.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, fetched2.OccurrenceCount)

	assert.NotEqual(t, fetched1.ID, fetched2.ID)
	assert.NotEqual(t, fetched1.ContentHash, fetched2.ContentHash)
}

func TestListBrowserEvents(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-list-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events List Workspace",
		Description: "Temporary workspace for browser events list tests",
	})
	require.NoError(t, err)

	// Create multiple events with different types
	events := []*BrowserEvent{
		{
			EventType:   BrowserEventConsole,
			Category:    BrowserEventCategoryRuntime,
			URL:         "https://example.com/list-test-1",
			Data:        []byte(`{"msg": "console 1"}`),
			WorkspaceID: workspace.ID,
			Source:      "crawler",
		},
		{
			EventType:   BrowserEventSecurity,
			Category:    BrowserEventCategorySecurity,
			URL:         "https://example.com/list-test-2",
			Data:        []byte(`{"issue": "mixed content"}`),
			WorkspaceID: workspace.ID,
			Source:      "audit",
		},
		{
			EventType:   BrowserEventDOMStorage,
			Category:    BrowserEventCategoryStorage,
			URL:         "https://example.com/list-test-3",
			Data:        []byte(`{"key": "test", "value": "data"}`),
			WorkspaceID: workspace.ID,
			Source:      "crawler",
		},
	}

	for _, event := range events {
		err = Connection().SaveBrowserEvent(event)
		require.NoError(t, err)
	}

	// Test basic listing
	filter := BrowserEventFilter{
		Pagination:  Pagination{Page: 1, PageSize: 10},
		WorkspaceID: workspace.ID,
	}
	results, count, err := Connection().ListBrowserEvents(filter)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(3))
	assert.GreaterOrEqual(t, len(results), 3)

	// Test filtering by event type
	filter.EventTypes = []BrowserEventType{BrowserEventConsole}
	results, count, err = Connection().ListBrowserEvents(filter)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))
	for _, r := range results {
		assert.Equal(t, BrowserEventConsole, r.EventType)
	}

	// Test filtering by category
	filter.EventTypes = nil
	filter.Categories = []BrowserEventCategory{BrowserEventCategorySecurity}
	results, count, err = Connection().ListBrowserEvents(filter)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))
	for _, r := range results {
		assert.Equal(t, BrowserEventCategorySecurity, r.Category)
	}

	// Test filtering by source
	filter.Categories = nil
	filter.Sources = []string{"crawler"}
	results, count, err = Connection().ListBrowserEvents(filter)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(2))
	for _, r := range results {
		assert.Equal(t, "crawler", r.Source)
	}

	// Test filtering by URL (partial match)
	filter.Sources = nil
	filter.URL = "list-test"
	results, count, err = Connection().ListBrowserEvents(filter)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(3))
}

func TestBrowserEventStats(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-stats-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events Stats Workspace",
		Description: "Temporary workspace for browser events stats tests",
	})
	require.NoError(t, err)

	// Create a scan for the stats test
	scan := &Scan{
		WorkspaceID: workspace.ID,
		Status:      ScanStatusCompleted,
		Title:       "Stats Test Scan",
	}
	scan, err = Connection().CreateScan(scan)
	require.NoError(t, err)

	// Create events with different types and categories
	events := []*BrowserEvent{
		{EventType: BrowserEventConsole, Category: BrowserEventCategoryRuntime, URL: "https://example.com/stats-1", Data: []byte(`{"a":1}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: BrowserEventConsole, Category: BrowserEventCategoryRuntime, URL: "https://example.com/stats-2", Data: []byte(`{"a":2}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: BrowserEventSecurity, Category: BrowserEventCategorySecurity, URL: "https://example.com/stats-3", Data: []byte(`{"a":3}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: BrowserEventDOMStorage, Category: BrowserEventCategoryStorage, URL: "https://example.com/stats-4", Data: []byte(`{"a":4}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
	}

	for _, event := range events {
		err = Connection().SaveBrowserEvent(event)
		require.NoError(t, err)
	}

	// Test type stats
	typeStats, err := Connection().GetBrowserEventTypeStats(scan.ID)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, typeStats[BrowserEventConsole], int64(2))
	assert.GreaterOrEqual(t, typeStats[BrowserEventSecurity], int64(1))
	assert.GreaterOrEqual(t, typeStats[BrowserEventDOMStorage], int64(1))

	// Test category stats
	categoryStats, err := Connection().GetBrowserEventCategoryStats(scan.ID)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, categoryStats[BrowserEventCategoryRuntime], int64(2))
	assert.GreaterOrEqual(t, categoryStats[BrowserEventCategorySecurity], int64(1))
	assert.GreaterOrEqual(t, categoryStats[BrowserEventCategoryStorage], int64(1))

	// Test count
	count, err := Connection().CountBrowserEventsByScanID(scan.ID)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(4))
}

func TestDeleteBrowserEventsByScanID(t *testing.T) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        fmt.Sprintf("test-browser-events-delete-%d", time.Now().UnixNano()),
		Title:       "Test Browser Events Delete Workspace",
		Description: "Temporary workspace for browser events delete tests",
	})
	require.NoError(t, err)

	// Create a scan
	scan := &Scan{
		WorkspaceID: workspace.ID,
		Status:      ScanStatusCompleted,
		Title:       "Delete Test Scan",
	}
	scan, err = Connection().CreateScan(scan)
	require.NoError(t, err)

	// Create events for this scan
	for i := 0; i < 3; i++ {
		event := &BrowserEvent{
			EventType:   BrowserEventConsole,
			Category:    BrowserEventCategoryRuntime,
			URL:         "https://example.com/delete-test",
			Data:        []byte(`{"msg": "test"}`),
			WorkspaceID: workspace.ID,
			ScanID:      &scan.ID,
			Source:      "test",
		}
		// Use different data to avoid aggregation
		event.Data = []byte(`{"msg": "test", "i": ` + string(rune('0'+i)) + `}`)
		err = Connection().SaveBrowserEvent(event)
		require.NoError(t, err)
	}

	// Verify events exist
	count, err := Connection().CountBrowserEventsByScanID(scan.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Delete events
	err = Connection().DeleteBrowserEventsByScanID(scan.ID)
	assert.NoError(t, err)

	// Verify events are deleted
	count, err = Connection().CountBrowserEventsByScanID(scan.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}
