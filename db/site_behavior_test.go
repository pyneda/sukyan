package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSiteBehaviorTestData(t *testing.T) (*Workspace, *Scan, *History, []*History) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Code:        "site-behavior-test",
		Title:       "Site Behavior Test Workspace",
		Description: "Temporary workspace for site behavior tests",
	})
	require.NoError(t, err)

	scan := &Scan{
		WorkspaceID: workspace.ID,
		Status:      ScanStatusPending,
		Title:       "Site Behavior Test Scan",
	}
	scan, err = Connection().CreateScan(scan)
	require.NoError(t, err)

	baseHistory := &History{
		URL:         "http://example.com/",
		StatusCode:  200,
		WorkspaceID: &workspace.ID,
	}
	_, err = Connection().CreateHistory(baseHistory)
	require.NoError(t, err)

	var nfHistories []*History
	for _, path := range []string{"/nf1", "/nf2", "/nf3"} {
		h := &History{
			URL:         "http://example.com" + path,
			StatusCode:  200,
			WorkspaceID: &workspace.ID,
		}
		_, err = Connection().CreateHistory(h)
		require.NoError(t, err)
		nfHistories = append(nfHistories, h)
	}

	return workspace, scan, baseHistory, nfHistories
}

func TestCreateSiteBehaviorWithSamples(t *testing.T) {
	workspace, scan, baseHistory, nfHistories := setupSiteBehaviorTestData(t)

	result := &SiteBehaviorResult{
		ScanID:             scan.ID,
		WorkspaceID:        workspace.ID,
		BaseURL:            "http://example.com",
		NotFoundReturns404: false,
		NotFoundChanges:    false,
		NotFoundCommonHash: "testhash123",
		NotFoundStatusCode: 200,
		BaseURLSampleID:    &baseHistory.ID,
	}

	created, err := Connection().CreateSiteBehaviorResult(result)
	require.NoError(t, err)
	assert.NotNil(t, created)

	for _, h := range nfHistories {
		sample := &SiteBehaviorNotFoundSample{
			SiteBehaviorResultID: created.ID,
			HistoryID:            h.ID,
		}
		err := Connection().CreateSiteBehaviorNotFoundSample(sample)
		assert.NoError(t, err)
		assert.NotZero(t, sample.ID)
	}

	loaded, err := Connection().GetSiteBehaviorWithSamples(scan.ID, "http://example.com")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, created.ID, loaded.ID)
	assert.False(t, loaded.NotFoundReturns404)
	assert.Equal(t, "testhash123", loaded.NotFoundCommonHash)

	require.NotNil(t, loaded.BaseURLSample)
	assert.Equal(t, baseHistory.ID, loaded.BaseURLSample.ID)
	assert.Equal(t, "http://example.com/", loaded.BaseURLSample.URL)

	require.Len(t, loaded.NotFoundSamples, 3)
	for _, sample := range loaded.NotFoundSamples {
		assert.NotZero(t, sample.History.ID)
		assert.Contains(t, sample.History.URL, "http://example.com/nf")
	}
}

func TestGetSiteBehaviorWithSamplesNotFound(t *testing.T) {
	result, err := Connection().GetSiteBehaviorWithSamples(999999, "http://nonexistent.example.com")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetSiteBehaviorWithSamplesNoSamples(t *testing.T) {
	workspace, scan, _, _ := setupSiteBehaviorTestData(t)

	result := &SiteBehaviorResult{
		ScanID:             scan.ID,
		WorkspaceID:        workspace.ID,
		BaseURL:            "http://no-samples.example.com",
		NotFoundReturns404: true,
		NotFoundStatusCode: 404,
	}

	created, err := Connection().CreateSiteBehaviorResult(result)
	require.NoError(t, err)

	loaded, err := Connection().GetSiteBehaviorWithSamples(scan.ID, "http://no-samples.example.com")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, created.ID, loaded.ID)
	assert.True(t, loaded.NotFoundReturns404)
	assert.Nil(t, loaded.BaseURLSample)
	assert.Empty(t, loaded.NotFoundSamples)
}
