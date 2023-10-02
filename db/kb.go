package db

import (
	"github.com/rs/zerolog/log"
)

type IssueCode string

type IssueTemplate struct {
	Code        IssueCode `json:"code"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Remediation string    `json:"remediation"`
	Cwe         int       `json:"cwe"`
	Severity    string    `json:"severity"`
	References  []string  `json:"references"`
}

func GetIssueTemplateByCode(code IssueCode) *Issue {
	for _, issueTemplate := range issueTemplates {
		if issueTemplate.Code == code {
			return &Issue{
				Code:        string(issueTemplate.Code),
				Title:       issueTemplate.Title,
				Description: issueTemplate.Description,
				Remediation: issueTemplate.Remediation,
				Cwe:         issueTemplate.Cwe,
				Severity:    NewSeverity(issueTemplate.Severity),
				References:  StringSlice(issueTemplate.References),
			}
		}
	}
	return nil
}

func FillIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID *uint) *Issue {
	issue := GetIssueTemplateByCode(code)
	issue.URL = history.URL
	issue.Request = history.RawRequest
	issue.Response = history.RawResponse
	issue.StatusCode = history.StatusCode
	issue.HTTPMethod = history.Method
	issue.Confidence = confidence
	issue.Details = details
	issue.WorkspaceID = workspaceID
	issue.Requests = []History{*history}
	if severity != "" {
		issue.Severity = NewSeverity(severity)
	}
	return issue
}

func CreateIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID *uint) {
	issue := FillIssueFromHistoryAndTemplate(history, code, details, confidence, severity, workspaceID, taskID, taskJobID)
	log.Warn().Str("issue", issue.Title).Str("url", history.URL).Uint("workspace", *workspaceID).Msg("New issue found")
	Connection.CreateIssue(*issue)
}
