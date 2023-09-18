package passive

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestGetHeadersOccurrences(t *testing.T) {
	// Create actual instances of the History struct
	history1 := &db.History{
		ResponseHeaders: datatypes.JSON(`{"Header1": ["Value1"]}`),
	}
	history2 := &db.History{
		ResponseHeaders: datatypes.JSON(`{"Header1": ["Value1"], "Header2": ["Value2"]}`),
	}
	history3 := &db.History{
		ResponseHeaders: datatypes.JSON(`{"Header1": ["Value1", "Value3"]}`),
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
