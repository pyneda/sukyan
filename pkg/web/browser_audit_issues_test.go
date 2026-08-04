package web

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
)

const listenerPageURL = "https://example.com/crawled/page"

func auditEvent(details *proto.AuditsInspectorIssueDetails) *proto.AuditsIssueAdded {
	return &proto.AuditsIssueAdded{Issue: &proto.AuditsInspectorIssue{Details: details}}
}

func TestBuildBrowserAuditIssueURL(t *testing.T) {
	tests := []struct {
		name        string
		event       *proto.AuditsIssueAdded
		expectedURL string
	}{
		{
			name: "mixed content uses the main resource url",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				MixedContentIssueDetails: &proto.AuditsMixedContentIssueDetails{
					InsecureURL:     "http://cdn.example.com/app.js",
					MainResourceURL: "https://example.com/lab/mixed-content/passive",
					ResourceType:    proto.AuditsMixedContentResourceTypeScript,
				},
			}),
			expectedURL: "https://example.com/lab/mixed-content/passive",
		},
		{
			name: "mixed content without main resource url falls back to the page url",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				MixedContentIssueDetails: &proto.AuditsMixedContentIssueDetails{
					InsecureURL: "http://cdn.example.com/app.js",
				},
			}),
			expectedURL: listenerPageURL,
		},
		{
			name: "cors uses the affected request url",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				CorsIssueDetails: &proto.AuditsCorsIssueDetails{
					Request:  &proto.AuditsAffectedRequest{URL: "https://api.example.com/v1/account"},
					Location: &proto.AuditsSourceCodeLocation{URL: "https://example.com/static/app.js"},
					CorsErrorStatus: &proto.NetworkCorsErrorStatus{
						CorsError:       proto.NetworkCorsErrorMissingAllowOriginHeader,
						FailedParameter: "",
					},
				},
			}),
			expectedURL: "https://api.example.com/v1/account",
		},
		{
			name: "cors without a request url falls back to the source location",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				CorsIssueDetails: &proto.AuditsCorsIssueDetails{
					Request:  &proto.AuditsAffectedRequest{},
					Location: &proto.AuditsSourceCodeLocation{URL: "https://example.com/static/app.js", LineNumber: 12, ColumnNumber: 3},
				},
			}),
			expectedURL: "https://example.com/static/app.js",
		},
		{
			name: "cors without any event url falls back to the page url",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				CorsIssueDetails: &proto.AuditsCorsIssueDetails{IsWarning: true},
			}),
			expectedURL: listenerPageURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := buildBrowserAuditIssue(listenerPageURL, tt.event, 1, 0, 0, 0)
			if issue == nil {
				t.Fatal("expected an issue to be built")
			}
			if issue.URL != tt.expectedURL {
				t.Errorf("expected URL %q, got %q", tt.expectedURL, issue.URL)
			}
		})
	}
}

func TestBuildBrowserAuditIssueScanAttribution(t *testing.T) {
	const (
		workspaceID uint = 7
		taskID      uint = 0
		scanID      uint = 2730
		scanJobID   uint = 91
	)

	tests := []struct {
		name         string
		event        *proto.AuditsIssueAdded
		expectedCode db.IssueCode
	}{
		{
			name: "mixed content",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				MixedContentIssueDetails: &proto.AuditsMixedContentIssueDetails{
					InsecureURL:     "http://cdn.example.com/app.js",
					MainResourceURL: "https://example.com/lab/mixed-content/passive",
				},
			}),
			expectedCode: db.MixedContentCode,
		},
		{
			name: "cors",
			event: auditEvent(&proto.AuditsInspectorIssueDetails{
				CorsIssueDetails: &proto.AuditsCorsIssueDetails{
					Request: &proto.AuditsAffectedRequest{URL: "https://api.example.com/v1/account"},
				},
			}),
			expectedCode: db.CorsCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := buildBrowserAuditIssue(listenerPageURL, tt.event, workspaceID, taskID, scanID, scanJobID)
			if issue == nil {
				t.Fatal("expected an issue to be built")
			}
			if issue.Code != string(tt.expectedCode) {
				t.Errorf("expected code %q, got %q", tt.expectedCode, issue.Code)
			}
			assertUintPtr(t, "workspace_id", issue.WorkspaceID, workspaceID)
			assertUintPtr(t, "task_id", issue.TaskID, taskID)
			assertUintPtr(t, "scan_id", issue.ScanID, scanID)
			assertUintPtr(t, "scan_job_id", issue.ScanJobID, scanJobID)
		})
	}
}

func TestBuildBrowserAuditIssueMixedContentDetails(t *testing.T) {
	event := auditEvent(&proto.AuditsInspectorIssueDetails{
		MixedContentIssueDetails: &proto.AuditsMixedContentIssueDetails{
			InsecureURL:      "http://cdn.example.com/app.js",
			MainResourceURL:  "https://example.com/lab/mixed-content/passive",
			ResourceType:     proto.AuditsMixedContentResourceTypeScript,
			ResolutionStatus: proto.AuditsMixedContentResolutionStatusMixedContentBlocked,
			Frame:            &proto.AuditsAffectedFrame{FrameID: "FRAME-1"},
		},
	})

	issue := buildBrowserAuditIssue(listenerPageURL, event, 1, 0, 0, 0)
	if issue == nil {
		t.Fatal("expected an issue to be built")
	}
	if !strings.Contains(issue.Details, "http://cdn.example.com/app.js") {
		t.Errorf("details should report the insecure url, got %q", issue.Details)
	}
	if !strings.Contains(issue.Details, "FRAME-1") {
		t.Errorf("details should report the affected frame, got %q", issue.Details)
	}
	if strings.Contains(issue.Details, listenerPageURL) {
		t.Errorf("details should not carry the listener page url, got %q", issue.Details)
	}
}

func TestBuildBrowserAuditIssueMixedContentWithoutFrame(t *testing.T) {
	event := auditEvent(&proto.AuditsInspectorIssueDetails{
		MixedContentIssueDetails: &proto.AuditsMixedContentIssueDetails{
			InsecureURL:     "http://cdn.example.com/app.js",
			MainResourceURL: "https://example.com/lab/mixed-content/passive",
		},
	})

	issue := buildBrowserAuditIssue(listenerPageURL, event, 1, 0, 0, 0)
	if issue == nil {
		t.Fatal("expected an issue to be built")
	}
	if strings.Contains(issue.Details, "Affected frame") {
		t.Errorf("details should omit the frame when the event has none, got %q", issue.Details)
	}
}

func TestBuildBrowserAuditIssueUnhandledDetails(t *testing.T) {
	event := auditEvent(&proto.AuditsInspectorIssueDetails{
		HeavyAdIssueDetails: &proto.AuditsHeavyAdIssueDetails{},
	})

	if issue := buildBrowserAuditIssue(listenerPageURL, event, 1, 0, 0, 0); issue != nil {
		t.Errorf("expected no issue for unhandled audit details, got %+v", issue)
	}
}

func assertUintPtr(t *testing.T, field string, got *uint, expected uint) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %d, got nil", field, expected)
		return
	}
	if *got != expected {
		t.Errorf("%s: expected %d, got %d", field, expected, *got)
	}
}
