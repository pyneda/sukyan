package db

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

type IssueCode string

func (i IssueCode) Name() string {
	return strings.ReplaceAll(string(i), "_", " ")
}

func (i IssueCode) String() string {
	return string(i)
}

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
	issue.TaskID = taskID
	issue.TaskJobID = taskJobID
	issue.Requests = []History{*history}
	if severity != "" {
		issue.Severity = NewSeverity(severity)
	}
	return issue
}

func CreateIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID *uint) (Issue, error) {
	issue := FillIssueFromHistoryAndTemplate(history, code, details, confidence, severity, workspaceID, taskID, taskJobID)
	createdIssue, err := Connection.CreateIssue(*issue)
	if err != nil {
		log.Error().Err(err).Str("issue", issue.Title).Str("url", history.URL).Msg("Failed to create issue")
		return createdIssue, err
	}

	workspaceIDValue := uint(0)
	if workspaceID != nil {
		workspaceIDValue = *workspaceID
	}

	taskIDValue := uint(0)
	if taskID != nil {
		taskIDValue = *taskID
	}

	log.Warn().Uint("id", createdIssue.ID).Str("issue", issue.Title).Str("url", history.URL).Uint("workspace", workspaceIDValue).Uint("task", taskIDValue).Msg("New issue found")
	return createdIssue, nil
}

func FillIssueFromWebSocketConnectionAndTemplate(connection *WebSocketConnection, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID *uint) *Issue {
	issue := GetIssueTemplateByCode(code)
	if issue == nil {
		return nil
	}
	requestHeaders, _ := connection.GetRequestHeadersAsString()
	responseHeaders, _ := connection.GetResponseHeadersAsString()

	issue.URL = connection.URL
	issue.Request = []byte(requestHeaders)
	issue.Response = []byte(responseHeaders)
	issue.StatusCode = connection.StatusCode
	issue.HTTPMethod = "WEBSOCKET"
	issue.Confidence = confidence
	issue.Details = details
	issue.WorkspaceID = workspaceID
	issue.TaskID = taskID
	issue.TaskJobID = taskJobID

	if severity != "" {
		issue.Severity = NewSeverity(severity)
	}
	return issue
}

func CreateIssueFromWebSocketConnectionAndTemplate(connection *WebSocketConnection, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID *uint) (Issue, error) {
	issue := FillIssueFromWebSocketConnectionAndTemplate(connection, code, details, confidence, severity, workspaceID, taskID, taskJobID)
	if issue == nil {
		err := fmt.Errorf("issue template with code %s not found", code)
		log.Error().Err(err).Str("code", string(code)).Msg("Failed to get issue template")
		return Issue{}, err
	}

	createdIssue, err := Connection.CreateIssue(*issue)
	if err != nil {
		log.Error().Err(err).Str("issue", issue.Title).Str("url", connection.URL).Msg("Failed to create issue")
		return createdIssue, err
	}

	workspaceIDValue := uint(0)
	if workspaceID != nil {
		workspaceIDValue = *workspaceID
	}

	taskIDValue := uint(0)
	if taskID != nil {
		taskIDValue = *taskID
	}

	log.Warn().Uint("id", createdIssue.ID).Str("issue", issue.Title).Str("url", connection.URL).Uint("workspace", workspaceIDValue).Uint("task", taskIDValue).Msg("New issue found")
	return createdIssue, nil
}
