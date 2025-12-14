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

// GetAllIssueTemplates returns all issue templates from the knowledge base
func GetAllIssueTemplates() []IssueTemplate {
	return issueTemplates
}

func FillIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID, scanID, scanJobID *uint) *Issue {
	issue := GetIssueTemplateByCode(code)
	if history != nil {
		issue.URL = history.URL
		issue.Request = history.RawRequest
		issue.Response = history.RawResponse
		issue.StatusCode = history.StatusCode
		issue.HTTPMethod = history.Method
		issue.Requests = []History{*history}
	} else {
		log.Warn().Str("code", string(code)).Msg("No history provided for issue creation")
	}

	issue.Confidence = confidence
	issue.Details = details
	issue.WorkspaceID = workspaceID
	issue.TaskID = taskID
	issue.TaskJobID = taskJobID
	issue.ScanID = scanID
	issue.ScanJobID = scanJobID
	if severity != "" {
		issue.Severity = NewSeverity(severity)
	}
	return issue
}

func CreateIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID, scanID, scanJobID *uint) (Issue, error) {
	issue := FillIssueFromHistoryAndTemplate(history, code, details, confidence, severity, workspaceID, taskID, taskJobID, scanID, scanJobID)
	createdIssue, err := Connection().CreateIssue(*issue)
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

func FillIssueFromWebSocketConnectionAndTemplate(connection *WebSocketConnection, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID, scanID, scanJobID *uint) *Issue {
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
	issue.ScanID = scanID
	issue.ScanJobID = scanJobID
	issue.WebsocketConnectionID = &connection.ID

	if severity != "" {
		issue.Severity = NewSeverity(severity)
	}
	return issue
}

func CreateIssueFromWebSocketConnectionAndTemplate(connection *WebSocketConnection, code IssueCode, details string, confidence int, severity string, workspaceID, taskID, taskJobID, scanID, scanJobID *uint) (Issue, error) {
	log.Info().Str("code", string(code)).Str("url", connection.URL).Msg("Creating issue from WebSocket connection")
	issue := FillIssueFromWebSocketConnectionAndTemplate(connection, code, details, confidence, severity, workspaceID, taskID, taskJobID, scanID, scanJobID)
	if issue == nil {
		err := fmt.Errorf("issue template with code %s not found", code)
		log.Error().Err(err).Str("code", string(code)).Msg("Failed to get issue template")
		return Issue{}, err
	}

	createdIssue, err := Connection().CreateIssue(*issue)
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

// CreateIssueFromWebSocketMessage creates an issue from a WebSocket message
func CreateIssueFromWebSocketMessage(message *WebSocketMessage, code IssueCode, details string, confidence int, severityOverride string, workspaceID, taskID, taskJobID, scanID, scanJobID, connectionID, upgradeRequestID *uint) (Issue, error) {
	template := GetIssueTemplateByCode(code)

	issue := Issue{
		Code:                  string(code),
		Title:                 template.Title,
		Description:           template.Description,
		Details:               details,
		Remediation:           template.Remediation,
		References:            template.References,
		Cwe:                   template.Cwe,
		Confidence:            confidence,
		WebsocketConnectionID: connectionID,
		Payload:               message.PayloadData,
		WorkspaceID:           workspaceID,
		TaskID:                taskID,
		TaskJobID:             taskJobID,
		ScanID:                scanID,
		ScanJobID:             scanJobID,
	}

	// Get connection details
	if connectionID != nil && *connectionID > 0 {
		connection, err := Connection().GetWebSocketConnection(*connectionID)
		if err != nil {
			log.Error().Err(err).Uint("connection_id", *connectionID).Msg("Error retrieving WebSocket connection")
			// Continue even if connection retrieval fails
		} else {
			issue.URL = connection.URL
			issue.Request = []byte(connection.RequestHeaders)
			issue.Response = []byte(connection.ResponseHeaders)
			issue.StatusCode = connection.StatusCode
			issue.HTTPMethod = "WEBSOCKET"
		}
	}

	if upgradeRequestID != nil && *upgradeRequestID > 0 {
		upgradeRequest, err := Connection().GetHistoryByID(*upgradeRequestID)
		if err != nil {
			log.Error().Err(err).Uint("upgrade_request_id", *upgradeRequestID).Msg("Error retrieving upgrade request while creating issue")
		} else {
			// issue.URL = upgradeRequest.URL
			issue.Request = upgradeRequest.RawRequest
			issue.Response = upgradeRequest.RawResponse
			issue.StatusCode = upgradeRequest.StatusCode
			issue.HTTPMethod = upgradeRequest.Method
		}
	}

	// Override severity if specified
	if severityOverride != "" {
		issue.Severity = NewSeverity(severityOverride)
	} else {
		issue.Severity = template.Severity
	}

	createdIssue, err := Connection().CreateIssue(issue)
	if err != nil {
		log.Error().Err(err).Interface("issue", issue).Msg("Error creating issue from WebSocket message")
		return Issue{}, fmt.Errorf("error creating issue: %w", err)
	}
	issue = createdIssue

	log.Info().
		Uint("id", issue.ID).
		Str("code", issue.Code).
		Str("title", issue.Title).
		Str("url", issue.URL).
		Int("confidence", issue.Confidence).
		Msg("Created issue from WebSocket message")

	return issue, nil
}
