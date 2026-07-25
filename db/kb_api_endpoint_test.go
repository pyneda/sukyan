package db

import (
	"testing"

	"github.com/google/uuid"
)

// TestFillIssueFromHistoryAndTemplate_PropagatesAPIEndpointID guards D19: issues
// created from an API-endpoint history must inherit that history's APIEndpointID.
// MarkAPIEndpointScanned counts issues by api_endpoint_id, so without this the
// per-endpoint issue count is always 0 even when the endpoint has findings.
func TestFillIssueFromHistoryAndTemplate_PropagatesAPIEndpointID(t *testing.T) {
	endpointID := uuid.New()
	history := &History{
		URL:           "http://example.com/api/orders",
		Method:        "POST",
		APIEndpointID: &endpointID,
	}

	issue := FillIssueFromHistoryAndTemplate(history, SqlInjectionCode, "details", 90, "", nil, nil, nil, nil, nil)
	if issue.APIEndpointID == nil {
		t.Fatal("APIEndpointID not propagated to issue: got nil")
	}
	if *issue.APIEndpointID != endpointID {
		t.Errorf("APIEndpointID mismatch: want %s, got %s", endpointID, *issue.APIEndpointID)
	}

	// A history with no endpoint linkage must leave the issue unlinked.
	plain := &History{URL: "http://example.com/", Method: "GET"}
	issue2 := FillIssueFromHistoryAndTemplate(plain, SqlInjectionCode, "details", 90, "", nil, nil, nil, nil, nil)
	if issue2.APIEndpointID != nil {
		t.Errorf("expected nil APIEndpointID for non-API history, got %v", *issue2.APIEndpointID)
	}
}
