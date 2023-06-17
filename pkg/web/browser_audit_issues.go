package web

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"strings"
	"encoding/json"
	"fmt"
	"strconv"

)

func handleBrowserAuditIssues(url string, e *proto.AuditsIssueAdded) {
			jsonDetails, err := json.Marshal(e.Issue.Details)
			if err != nil {
				log.Error().Err(err).Str("url", url).Msg("Could not convert browser audit issue event details to JSON")
			}
			// Check if it is a Mixed Content Issue Details
			// Codes in: https://github.com/go-rod/rod/blob/ba02d6c76c1e2ef7ab4a58909c58877b34761fd9/lib/proto/audits.go#L809
			if e.Issue.Details.MixedContentIssueDetails != nil {
				var description strings.Builder
				description.WriteString("DETAILS:\nThe insecure content loaded url comes from: " + e.Issue.Details.MixedContentIssueDetails.InsecureURL)
				if e.Issue.Details.MixedContentIssueDetails.Frame.FrameID != "" {
					description.WriteString("\nAffected frame: " + string(e.Issue.Details.MixedContentIssueDetails.Frame.FrameID))
				}
				if e.Issue.Details.MixedContentIssueDetails.ResourceType != "" {
					description.WriteString("\nResource type: " + string(e.Issue.Details.MixedContentIssueDetails.ResourceType))
				}
				if e.Issue.Details.MixedContentIssueDetails.ResolutionStatus != "" {
					description.WriteString("\nResolution status: " + string(e.Issue.Details.MixedContentIssueDetails.ResolutionStatus))
				}
				if e.Issue.Details.MixedContentIssueDetails.MainResourceURL != "" {
					description.WriteString("\nMain resource url: " + string(e.Issue.Details.MixedContentIssueDetails.MainResourceURL))
				}
				browserAuditIssue := db.GetIssueTemplateByCode(db.MixedContentCode)
				browserAuditIssue.URL = url
				browserAuditIssue.Description = fmt.Sprintf("%v\n%v", browserAuditIssue.Description, description.String())
				browserAuditIssue.AdditionalInfo = jsonDetails
				browserAuditIssue.Confidence = 80
				db.Connection.CreateIssue(*browserAuditIssue)

			} else if e.Issue.Details.CorsIssueDetails != nil {
				var description strings.Builder
				description.WriteString("DETAILS:\n")
				if e.Issue.Details.CorsIssueDetails.CorsErrorStatus != nil {
					description.WriteString("\nCORS Error: " + string(e.Issue.Details.CorsIssueDetails.CorsErrorStatus.CorsError))
					description.WriteString("\nCORS Error Failed Parameter: " + string(e.Issue.Details.CorsIssueDetails.CorsErrorStatus.FailedParameter))

				}
				description.WriteString("\nIs Warning: " + strconv.FormatBool(e.Issue.Details.CorsIssueDetails.IsWarning))
				if e.Issue.Details.CorsIssueDetails.Location != nil {
					description.WriteString("\nSource code location:")
					description.WriteString("\n		- URL: " + string(e.Issue.Details.CorsIssueDetails.Location.URL))
					description.WriteString("\n		- Line number: " + strconv.Itoa(e.Issue.Details.CorsIssueDetails.Location.LineNumber))
					description.WriteString("\n		- Column number: " + strconv.Itoa(e.Issue.Details.CorsIssueDetails.Location.ColumnNumber))

				}
				if e.Issue.Details.CorsIssueDetails.InitiatorOrigin != "" {
					description.WriteString("\nInitiator Origin: " + string(e.Issue.Details.CorsIssueDetails.InitiatorOrigin))
				}
				if e.Issue.Details.CorsIssueDetails.ClientSecurityState != nil {
					description.WriteString("\nNetwork Client Security State:")
					description.WriteString("\n		- Initiator is secure context: " + strconv.FormatBool(e.Issue.Details.CorsIssueDetails.ClientSecurityState.InitiatorIsSecureContext))
					description.WriteString("\n		- Initiator IP address space: " + string(e.Issue.Details.CorsIssueDetails.ClientSecurityState.InitiatorIPAddressSpace))
					description.WriteString("\n		- Private network request policy: " + string(e.Issue.Details.CorsIssueDetails.ClientSecurityState.PrivateNetworkRequestPolicy))

				}
				browserAuditIssue := db.GetIssueTemplateByCode(db.CorsCode)
				browserAuditIssue.URL = url
				browserAuditIssue.Description = fmt.Sprintf("%v\n%v", browserAuditIssue.Description, description.String())
				browserAuditIssue.AdditionalInfo = jsonDetails
				browserAuditIssue.Confidence = 80
				db.Connection.CreateIssue(*browserAuditIssue)

			} else {
				// Generic while dont have customized for every event type
				browserAuditIssue := db.Issue{
					Code:           "browser-audit-" + string(e.Issue.Code),
					URL:            url,
					Title:          "Browser audit issue (classification needed)",
					Cwe:            1,
					StatusCode:     200,
					HTTPMethod:     "GET?",
					Description:    string(jsonDetails),
					Payload:        "N/A",
					Confidence:     80,
					AdditionalInfo: jsonDetails,
					Severity:       "Low",
				}
				db.Connection.CreateIssue(browserAuditIssue)

			}
}
