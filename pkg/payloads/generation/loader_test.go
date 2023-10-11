package generation

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMergeGenerators(t *testing.T) {
	local := []*PayloadGenerator{
		{ID: "1", IssueCode: "Local1"},
		{ID: "2", IssueCode: "Local2"},
	}
	user := []*PayloadGenerator{
		{ID: "2", IssueCode: "User2"},
		{ID: "3", IssueCode: "User3"},
	}

	result := mergeGenerators(local, user)
	assert.NotNil(t, result)
	assert.Equal(t, 3, len(result))

	var found bool
	for _, gen := range result {
		if gen.ID == "2" {
			assert.Equal(t, "User2", gen.IssueCode)
			found = true
		}
	}
	assert.True(t, found, "Overlapping generator was not found in the merged result.")
}

func TestLoadGenerators(t *testing.T) {
	gens, err := LoadGenerators("")
	assert.NoError(t, err)
	assert.NotNil(t, gens)
	assert.True(t, len(gens) > 0)
}
