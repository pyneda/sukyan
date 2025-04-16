package passive

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"

	"github.com/stretchr/testify/assert"
)

func TestGetHeadersOccurrences(t *testing.T) {
	// Create histories with raw HTTP responses
	history1 := &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nHeader1: Value1\r\n\r\nBody content"),
	}
	history2 := &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nHeader1: Value1\r\nHeader2: Value2\r\n\r\nBody content"),
	}
	history3 := &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nHeader1: Value1\r\nHeader1: Value3\r\n\r\nBody content"),
	}

	histories := []*db.History{history1, history2, history3}

	result := getHeadersOccurrences(histories)

	category1 := http_utils.ClassifyHTTPResponseHeader("Header1")
	category2 := http_utils.ClassifyHTTPResponseHeader("Header2")

	assert.NotNil(t, result[category1]["Header1"])
	assert.Equal(t, 4, result[category1]["Header1"].Count)
	assert.ElementsMatch(t, []string{"Value1", "Value3"}, result[category1]["Header1"].Values)

	assert.NotNil(t, result[category2]["Header2"])
	assert.Equal(t, 1, result[category2]["Header2"].Count)
	assert.ElementsMatch(t, []string{"Value2"}, result[category2]["Header2"].Values)
}
