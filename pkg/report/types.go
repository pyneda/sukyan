package report

// ReportIssue is an optimized representation of an issue for report generation
type ReportIssue struct {
	ID                  uint                       `json:"id"`
	Code                string                     `json:"code"`
	Title               string                     `json:"title"`
	Description         string                     `json:"description"`
	Details             string                     `json:"details"`
	Remediation         string                     `json:"remediation"`
	URL                 string                     `json:"url"`
	StatusCode          int                        `json:"status_code"`
	HTTPMethod          string                     `json:"http_method"`
	Payload             string                     `json:"payload,omitempty"`
	CreatedAt           string                     `json:"created_at"`
	Confidence          int                        `json:"confidence"`
	Severity            string                     `json:"severity"`
	FalsePositive       bool                       `json:"false_positive"`
	References          []string                   `json:"references,omitempty"`
	CURLCommand         string                     `json:"curl_command,omitempty"`
	Note                string                     `json:"note,omitempty"`
	Request             string                     `json:"request,omitempty"`            // Base64 encoded
	Response            string                     `json:"response,omitempty"`           // Base64 encoded
	RequestTruncated    bool                       `json:"request_truncated,omitempty"`  // True if request content was truncated
	ResponseTruncated   bool                       `json:"response_truncated,omitempty"` // True if response content was truncated
	CWE                 int                        `json:"cwe,omitempty"`
	WebSocketConnection *ReportWebSocketConnection `json:"websocket_connection,omitempty"` // Included when issue is WebSocket-related
}

// GroupedIssues represents issues grouped by their type
type GroupedIssues struct {
	Code        string         `json:"code"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Remediation string         `json:"remediation,omitempty"`
	Count       int            `json:"count"`
	Severity    string         `json:"severity"`
	Issues      []*ReportIssue `json:"issues"`
	CWE         int            `json:"cwe,omitempty"`
}

// Summary contains report statistics
type Summary struct {
	TotalIssues             int            `json:"total_issues"`
	CriticalCount           int            `json:"critical_count"`
	HighCount               int            `json:"high_count"`
	MediumCount             int            `json:"medium_count"`
	LowCount                int            `json:"low_count"`
	InfoCount               int            `json:"info_count"`
	UniqueAffectedEndpoints int            `json:"unique_affected_endpoints"`
	UniqueIssueTypes        int            `json:"unique_issue_types"`
	TopVulnTypes            []TopVulnType  `json:"top_vuln_types"`
	SeverityCounts          map[string]int `json:"severity_counts"`
}

type TopVulnType struct {
	Code  string `json:"code"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// HTMLReportData contains structured data for the HTML template
type HTMLReportData struct {
	Title         string           `json:"title"`
	Summary       Summary          `json:"summary"`
	Issues        []*ReportIssue   `json:"issues"`
	GroupedIssues []*GroupedIssues `json:"grouped_issues"`
	GeneratedAt   string           `json:"generated_at"`
}

// ReportWebSocketConnection represents a WebSocket connection for report generation
type ReportWebSocketConnection struct {
	ID              uint                      `json:"id"`
	URL             string                    `json:"url"`
	StatusCode      int                       `json:"status_code"`
	StatusText      string                    `json:"status_text"`
	CreatedAt       string                    `json:"created_at"`
	ClosedAt        string                    `json:"closed_at"`
	Source          string                    `json:"source"`
	Messages        []*ReportWebSocketMessage `json:"messages"`
	RequestHeaders  map[string][]string       `json:"request_headers,omitempty"`
	ResponseHeaders map[string][]string       `json:"response_headers,omitempty"`
}

// ReportWebSocketMessage represents a WebSocket message for report generation
type ReportWebSocketMessage struct {
	ID          uint    `json:"id"`
	Opcode      float64 `json:"opcode"`
	Mask        bool    `json:"mask"`
	PayloadData string  `json:"payload_data"`
	Timestamp   string  `json:"timestamp"`
	Direction   string  `json:"direction"`
}
