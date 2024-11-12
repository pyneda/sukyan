package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var SocketIOPaths = []string{
	"socket.io/",
	"socket.io/info",
	"socket.io/default/",
	"socket.io/websocket/",
	"socketio/",
	"ws/socket.io/",
	"websocket/socket.io/",
	"api/socket.io/",
	"v1/socket.io/",
	"v2/socket.io/",
	"socket.io/?EIO=3",
	"socket.io/?EIO=4",
	"socket.io/1/",
	"socket.io/2/",
	"socket.io/3/",
	"socket.io/4/",
}

func isSocketIOValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 && history.StatusCode != 400 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Socket.IO endpoint found: %s\n", history.URL)
	confidence := 0

	socketioIndicators := []string{
		"\"sid\":\"",
		"\"upgrades\":",
		"\"pingInterval\":",
		"\"pingTimeout\":",
		"\"maxPayload\":",
		"engineio",
		"websocket",
		"polling",
	}

	headers, _ := history.GetResponseHeadersAsMap()

	if strings.Contains(history.URL, "socket.io") {
		confidence += 20
		details += "- Socket.IO path pattern detected\n"
	}

	if strings.Contains(history.ResponseContentType, "application/json") {
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(bodyStr), &jsonData); err == nil {
			for _, indicator := range socketioIndicators {
				if strings.Contains(bodyStr, indicator) {
					confidence += 10
					details += fmt.Sprintf("- Contains Socket.IO configuration: %s\n", indicator)
				}
			}
		}
	}

	for header, values := range headers {
		headerValue := strings.Join(values, " ")
		if strings.Contains(strings.ToLower(header), "websocket") ||
			strings.Contains(strings.ToLower(headerValue), "websocket") {
			confidence += 10
			details += "- WebSocket header detected\n"
		}
	}

	if cors, exists := headers["Access-Control-Allow-Origin"]; exists {
		details += fmt.Sprintf("- CORS configuration: %v\n", cors)
		confidence += 5
	}

	if strings.Contains(bodyStr, "Bad request") || strings.Contains(bodyStr, "Not Found") {
		confidence += 10
		details += "- Standard Socket.IO error response detected\n"
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 25 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverSocketIO(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       SocketIOPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept":     "application/json,text/plain",
				"Connection": "upgrade",
				"Upgrade":    "websocket",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: isSocketIOValidationFunc,
		IssueCode:      db.SocketioDetectedCode,
	})
}
