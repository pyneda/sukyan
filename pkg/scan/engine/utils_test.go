package engine

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestSeparateHistoriesByBaseURL(t *testing.T) {
	// Create some mock history items
	histories := []*db.History{
		&db.History{URL: "https://example.com/page1"},
		&db.History{URL: "https://example.com/page2"},
		&db.History{URL: "http://example.net/page1"},
		&db.History{URL: "https://example.org/page1"},
		&db.History{URL: "https://example.org/page2"},
		&db.History{URL: "https://example.org/page3"},
	}

	// Run the function
	result := separateHistoriesByBaseURL(histories)

	// Assertions
	assert.Len(t, result, 3) // Should separate into 3 base URLs

	assert.Len(t, result["https://example.com"], 2) // Should have 2 items for "https://example.com"
	assert.Len(t, result["http://example.net"], 1)  // Should have 1 item for "http://example.net"
	assert.Len(t, result["https://example.org"], 3) // Should have 3 items for "https://example.org"

	// Validate the URLs are correctly categorized
	assert.Equal(t, "https://example.com/page1", result["https://example.com"][0].URL)
	assert.Equal(t, "https://example.com/page2", result["https://example.com"][1].URL)

	assert.Equal(t, "http://example.net/page1", result["http://example.net"][0].URL)

	assert.Equal(t, "https://example.org/page1", result["https://example.org"][0].URL)
	assert.Equal(t, "https://example.org/page2", result["https://example.org"][1].URL)
	assert.Equal(t, "https://example.org/page3", result["https://example.org"][2].URL)
}
