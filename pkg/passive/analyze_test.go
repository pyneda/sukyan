package passive

import (
	"github.com/pyneda/sukyan/db"
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

	// Assert the results
	assert.Equal(t, 4, result["Header1"].Count)
	assert.ElementsMatch(t, []string{"Value1", "Value3"}, result["Header1"].Values)
	assert.Equal(t, 1, result["Header2"].Count)
	assert.ElementsMatch(t, []string{"Value2"}, result["Header2"].Values)
}
