package report

import (
	"encoding/base64"
	"sort"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

// processIssues converts db.Issue objects to optimized ReportIssue objects
func processIssues(issues []*db.Issue, maxRequestSize int) []*ReportIssue {
	reportIssues := make([]*ReportIssue, 0, len(issues))

	for _, issue := range issues {
		request := issue.Request
		response := issue.Response
		requestTruncated := false
		responseTruncated := false

		// Truncate request and response if maxRequestSize is specified and > 0
		if maxRequestSize > 0 {
			if len(request) > maxRequestSize {
				request = request[:maxRequestSize]
				requestTruncated = true
			}
			if len(response) > maxRequestSize {
				response = response[:maxRequestSize]
				responseTruncated = true
			}
		}

		requestBase64 := base64.StdEncoding.EncodeToString(request)
		responseBase64 := base64.StdEncoding.EncodeToString(response)

		createdAt := ""
		if !issue.CreatedAt.IsZero() {
			createdAt = issue.CreatedAt.Format("2006-01-02 15:04:05")
		}

		reportIssue := &ReportIssue{
			ID:                issue.ID,
			Code:              issue.Code,
			Title:             issue.Title,
			Description:       issue.Description,
			Details:           issue.Details,
			Remediation:       issue.Remediation,
			URL:               issue.URL,
			StatusCode:        issue.StatusCode,
			HTTPMethod:        issue.HTTPMethod,
			Payload:           issue.Payload,
			CreatedAt:         createdAt,
			Confidence:        issue.Confidence,
			Severity:          string(issue.Severity),
			FalsePositive:     issue.FalsePositive,
			References:        issue.References,
			CURLCommand:       issue.CURLCommand,
			Note:              issue.Note,
			Request:           requestBase64,
			Response:          responseBase64,
			RequestTruncated:  requestTruncated,
			ResponseTruncated: responseTruncated,
			CWE:               issue.Cwe,
		}

		// Include WebSocket connection data if this is a WebSocket-related issue
		if issue.WebsocketConnectionID != nil && *issue.WebsocketConnectionID > 0 {
			wsConnection, err := db.Connection().GetWebSocketConnection(*issue.WebsocketConnectionID)
			if err != nil {
				log.Error().Err(err).Uint("connection_id", *issue.WebsocketConnectionID).Msg("Failed to fetch WebSocket connection for issue")
			} else {
				reportWsConnection := processWebSocketConnection(wsConnection)
				reportIssue.WebSocketConnection = reportWsConnection
			}
		}

		reportIssues = append(reportIssues, reportIssue)
	}

	// Sort issues by severity and then by title
	sort.Slice(reportIssues, func(i, j int) bool {
		severityOrder := map[string]int{
			"Critical": 5,
			"High":     4,
			"Medium":   3,
			"Low":      2,
			"Info":     1,
			"Unknown":  0,
		}

		si := severityOrder[reportIssues[i].Severity]
		sj := severityOrder[reportIssues[j].Severity]

		if si != sj {
			return si > sj
		}

		return strings.ToLower(reportIssues[i].Title) < strings.ToLower(reportIssues[j].Title)
	})

	return reportIssues
}

// groupIssuesByType organizes issues by their type/code for more efficient display
func groupIssuesByType(issues []*ReportIssue) []*GroupedIssues {
	groupMap := make(map[string]*GroupedIssues)

	for _, issue := range issues {
		groupKey := issue.Code

		group, exists := groupMap[groupKey]
		if !exists {
			group = &GroupedIssues{
				Code:        issue.Code,
				Title:       issue.Title,
				Description: issue.Description,
				Remediation: issue.Remediation,
				Severity:    issue.Severity,
				CWE:         issue.CWE,
				Issues:      []*ReportIssue{},
			}
			groupMap[groupKey] = group
		}

		group.Issues = append(group.Issues, issue)
		group.Count = len(group.Issues)

		// If this issue has a higher severity than the current group severity, update it
		severityOrder := map[string]int{
			"Critical": 5,
			"High":     4,
			"Medium":   3,
			"Low":      2,
			"Info":     1,
			"Unknown":  0,
		}

		if severityOrder[issue.Severity] > severityOrder[group.Severity] {
			group.Severity = issue.Severity
		}
	}

	// Convert map to slice for template usage
	groups := make([]*GroupedIssues, 0, len(groupMap))
	for _, group := range groupMap {
		// Sort issues within each group by confidence (high to low)
		sort.Slice(group.Issues, func(i, j int) bool {
			return group.Issues[i].Confidence > group.Issues[j].Confidence
		})
		groups = append(groups, group)
	}

	// Sort groups by severity (Critical > High > Medium > Low > Info)
	sort.Slice(groups, func(i, j int) bool {
		severityOrder := map[string]int{
			"Critical": 5,
			"High":     4,
			"Medium":   3,
			"Low":      2,
			"Info":     1,
			"Unknown":  0,
		}

		si := severityOrder[groups[i].Severity]
		sj := severityOrder[groups[j].Severity]

		if si != sj {
			return si > sj
		}

		// If severity is same, sort by issue count (most issues first)
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}

		// If count is same, sort alphabetically by title
		return strings.ToLower(groups[i].Title) < strings.ToLower(groups[j].Title)
	})

	return groups
}

// generateSummary creates statistics about the issues for the dashboard
func generateSummary(issues []*ReportIssue) Summary {
	// Count issues by severity
	severityCounts := map[string]int{
		"Critical": 0,
		"High":     0,
		"Medium":   0,
		"Low":      0,
		"Info":     0,
		"Unknown":  0,
	}

	typeCounts := make(map[string]int)
	typeNames := make(map[string]string)
	uniqueEndpoints := make(map[string]bool)

	for _, issue := range issues {
		severityCounts[issue.Severity]++

		typeCounts[issue.Code]++
		typeNames[issue.Code] = issue.Title

		baseURL, err := lib.GetBaseURL(issue.URL)
		if err != nil {
			uniqueEndpoints[issue.URL] = true
		} else {
			uniqueEndpoints[baseURL] = true
		}

	}

	// Create top vulnerability types data
	var topTypes []TopVulnType
	for code, count := range typeCounts {
		topTypes = append(topTypes, TopVulnType{
			Code:  code,
			Title: typeNames[code],
			Count: count,
		})
	}

	// Sort top types by count (descending)
	sort.Slice(topTypes, func(i, j int) bool {
		return topTypes[i].Count > topTypes[j].Count
	})

	// Limit to top 5 types
	if len(topTypes) > 5 {
		topTypes = topTypes[:5]
	}

	return Summary{
		TotalIssues:             len(issues),
		CriticalCount:           severityCounts["Critical"],
		HighCount:               severityCounts["High"],
		MediumCount:             severityCounts["Medium"],
		LowCount:                severityCounts["Low"],
		InfoCount:               severityCounts["Info"],
		UniqueIssueTypes:        len(typeCounts),
		UniqueAffectedEndpoints: len(uniqueEndpoints),
		TopVulnTypes:            topTypes,
		SeverityCounts:          severityCounts,
	}
}

// processWebSocketConnection converts a single db.WebSocketConnection object to optimized ReportWebSocketConnection object
func processWebSocketConnection(conn *db.WebSocketConnection) *ReportWebSocketConnection {
	requestHeaders, _ := conn.GetRequestHeadersAsMap()
	responseHeaders, _ := conn.GetResponseHeadersAsMap()

	createdAt := ""
	if !conn.CreatedAt.IsZero() {
		createdAt = conn.CreatedAt.Format("2006-01-02 15:04:05")
	}

	closedAt := ""
	if !conn.ClosedAt.IsZero() {
		closedAt = conn.ClosedAt.Format("2006-01-02 15:04:05")
	}

	messages := make([]*ReportWebSocketMessage, 0, len(conn.Messages))
	for _, msg := range conn.Messages {
		timestamp := ""
		if !msg.Timestamp.IsZero() {
			timestamp = msg.Timestamp.Format("2006-01-02 15:04:05")
		}

		reportMessage := &ReportWebSocketMessage{
			ID:          msg.ID,
			Opcode:      msg.Opcode,
			Mask:        msg.Mask,
			PayloadData: msg.PayloadData,
			Timestamp:   timestamp,
			Direction:   string(msg.Direction),
		}
		messages = append(messages, reportMessage)
	}

	reportConn := &ReportWebSocketConnection{
		ID:              conn.ID,
		URL:             conn.URL,
		StatusCode:      conn.StatusCode,
		StatusText:      conn.StatusText,
		CreatedAt:       createdAt,
		ClosedAt:        closedAt,
		Source:          conn.Source,
		Messages:        messages,
		RequestHeaders:  requestHeaders,
		ResponseHeaders: responseHeaders,
	}

	return reportConn
}
