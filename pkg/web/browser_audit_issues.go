package web

import (
	"strconv"
	"strings"

	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

func mixedContentIssueURL(pageURL string, details *proto.AuditsMixedContentIssueDetails) string {
	if details.MainResourceURL != "" {
		return details.MainResourceURL
	}
	return pageURL
}

func corsIssueURL(pageURL string, details *proto.AuditsCorsIssueDetails) string {
	if details.Request != nil && details.Request.URL != "" {
		return details.Request.URL
	}
	if details.Location != nil && details.Location.URL != "" {
		return details.Location.URL
	}
	return pageURL
}

func buildBrowserAuditIssue(pageURL string, e *proto.AuditsIssueAdded, workspaceID, taskID, scanID, scanJobID uint) *db.Issue {
	// Codes in: https://github.com/go-rod/rod/blob/ba02d6c76c1e2ef7ab4a58909c58877b34761fd9/lib/proto/audits.go#L809
	var browserAuditIssue *db.Issue
	var details strings.Builder

	if mixedContent := e.Issue.Details.MixedContentIssueDetails; mixedContent != nil {
		details.WriteString("The insecure content loaded url comes from: " + mixedContent.InsecureURL)
		if mixedContent.Frame != nil && mixedContent.Frame.FrameID != "" {
			details.WriteString("\nAffected frame: " + string(mixedContent.Frame.FrameID))
		}
		if mixedContent.ResourceType != "" {
			details.WriteString("\nResource type: " + string(mixedContent.ResourceType))
		}
		if mixedContent.ResolutionStatus != "" {
			details.WriteString("\nResolution status: " + string(mixedContent.ResolutionStatus))
		}
		browserAuditIssue = db.GetIssueTemplateByCode(db.MixedContentCode)
		browserAuditIssue.URL = mixedContentIssueURL(pageURL, mixedContent)
	} else if cors := e.Issue.Details.CorsIssueDetails; cors != nil {
		if cors.CorsErrorStatus != nil {
			details.WriteString("\nCORS Error: " + string(cors.CorsErrorStatus.CorsError))
			details.WriteString("\nCORS Error Failed Parameter: " + cors.CorsErrorStatus.FailedParameter)
		}
		details.WriteString("\nIs Warning: " + strconv.FormatBool(cors.IsWarning))
		if cors.Location != nil {
			details.WriteString("\nSource code location:")
			details.WriteString("\n		- URL: " + cors.Location.URL)
			details.WriteString("\n		- Line number: " + strconv.Itoa(cors.Location.LineNumber))
			details.WriteString("\n		- Column number: " + strconv.Itoa(cors.Location.ColumnNumber))
		}
		if cors.InitiatorOrigin != "" {
			details.WriteString("\nInitiator Origin: " + cors.InitiatorOrigin)
		}
		if cors.ClientSecurityState != nil {
			details.WriteString("\nNetwork Client Security State:")
			details.WriteString("\n		- Initiator is secure context: " + strconv.FormatBool(cors.ClientSecurityState.InitiatorIsSecureContext))
			details.WriteString("\n		- Initiator IP address space: " + string(cors.ClientSecurityState.InitiatorIPAddressSpace))
			details.WriteString("\n		- Private network request policy: " + string(cors.ClientSecurityState.PrivateNetworkRequestPolicy))
		}
		browserAuditIssue = db.GetIssueTemplateByCode(db.CorsCode)
		browserAuditIssue.URL = corsIssueURL(pageURL, cors)
	}

	if browserAuditIssue == nil {
		return nil
	}

	browserAuditIssue.Details = details.String()
	browserAuditIssue.Confidence = 80
	browserAuditIssue.WorkspaceID = &workspaceID
	browserAuditIssue.TaskID = &taskID
	browserAuditIssue.ScanID = &scanID
	browserAuditIssue.ScanJobID = &scanJobID
	return browserAuditIssue
}

func handleBrowserAuditIssues(pageURL string, e *proto.AuditsIssueAdded, workspaceID, taskID, scanID, scanJobID uint) db.Issue {
	browserAuditIssue := buildBrowserAuditIssue(pageURL, e, workspaceID, taskID, scanID, scanJobID)
	if browserAuditIssue == nil {
		return db.Issue{}
	}

	issue, err := db.Connection().CreateIssue(*browserAuditIssue)
	if err != nil {
		log.Error().Err(err).
			Str("code", browserAuditIssue.Code).
			Str("url", browserAuditIssue.URL).
			Uint("scan_id", scanID).
			Msg("Failed to store browser audit issue")
	}
	return issue
}
