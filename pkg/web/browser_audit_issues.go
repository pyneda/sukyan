package web

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"strconv"
	"strings"
)

func handleBrowserAuditIssues(url string, e *proto.AuditsIssueAdded) {
	// Check if it is a Mixed Content Issue Details
	// Codes in: https://github.com/go-rod/rod/blob/ba02d6c76c1e2ef7ab4a58909c58877b34761fd9/lib/proto/audits.go#L809
	if e.Issue.Details.MixedContentIssueDetails != nil {
		var details strings.Builder
		details.WriteString("The insecure content loaded url comes from: " + e.Issue.Details.MixedContentIssueDetails.InsecureURL)
		if e.Issue.Details.MixedContentIssueDetails.Frame.FrameID != "" {
			details.WriteString("\nAffected frame: " + string(e.Issue.Details.MixedContentIssueDetails.Frame.FrameID))
		}
		if e.Issue.Details.MixedContentIssueDetails.ResourceType != "" {
			details.WriteString("\nResource type: " + string(e.Issue.Details.MixedContentIssueDetails.ResourceType))
		}
		if e.Issue.Details.MixedContentIssueDetails.ResolutionStatus != "" {
			details.WriteString("\nResolution status: " + string(e.Issue.Details.MixedContentIssueDetails.ResolutionStatus))
		}
		if e.Issue.Details.MixedContentIssueDetails.MainResourceURL != "" {
			details.WriteString("\nMain resource url: " + string(e.Issue.Details.MixedContentIssueDetails.MainResourceURL))
		}
		browserAuditIssue := db.GetIssueTemplateByCode(db.MixedContentCode)
		browserAuditIssue.URL = url
		browserAuditIssue.Details = details.String()
		browserAuditIssue.Confidence = 80
		db.Connection.CreateIssue(*browserAuditIssue)

	} else if e.Issue.Details.CorsIssueDetails != nil {
		var details strings.Builder
		if e.Issue.Details.CorsIssueDetails.CorsErrorStatus != nil {
			details.WriteString("\nCORS Error: " + string(e.Issue.Details.CorsIssueDetails.CorsErrorStatus.CorsError))
			details.WriteString("\nCORS Error Failed Parameter: " + string(e.Issue.Details.CorsIssueDetails.CorsErrorStatus.FailedParameter))

		}
		details.WriteString("\nIs Warning: " + strconv.FormatBool(e.Issue.Details.CorsIssueDetails.IsWarning))
		if e.Issue.Details.CorsIssueDetails.Location != nil {
			details.WriteString("\nSource code location:")
			details.WriteString("\n		- URL: " + string(e.Issue.Details.CorsIssueDetails.Location.URL))
			details.WriteString("\n		- Line number: " + strconv.Itoa(e.Issue.Details.CorsIssueDetails.Location.LineNumber))
			details.WriteString("\n		- Column number: " + strconv.Itoa(e.Issue.Details.CorsIssueDetails.Location.ColumnNumber))

		}
		if e.Issue.Details.CorsIssueDetails.InitiatorOrigin != "" {
			details.WriteString("\nInitiator Origin: " + string(e.Issue.Details.CorsIssueDetails.InitiatorOrigin))
		}
		if e.Issue.Details.CorsIssueDetails.ClientSecurityState != nil {
			details.WriteString("\nNetwork Client Security State:")
			details.WriteString("\n		- Initiator is secure context: " + strconv.FormatBool(e.Issue.Details.CorsIssueDetails.ClientSecurityState.InitiatorIsSecureContext))
			details.WriteString("\n		- Initiator IP address space: " + string(e.Issue.Details.CorsIssueDetails.ClientSecurityState.InitiatorIPAddressSpace))
			details.WriteString("\n		- Private network request policy: " + string(e.Issue.Details.CorsIssueDetails.ClientSecurityState.PrivateNetworkRequestPolicy))

		}
		browserAuditIssue := db.GetIssueTemplateByCode(db.CorsCode)
		browserAuditIssue.URL = url
		browserAuditIssue.Details = details.String()
		browserAuditIssue.Confidence = 80
		db.Connection.CreateIssue(*browserAuditIssue)

	}
}
