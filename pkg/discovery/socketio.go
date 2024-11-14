package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
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

	var jsonData map[string]interface{}
	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 15
		if err := json.Unmarshal([]byte(bodyStr), &jsonData); err == nil {
			if sid, ok := jsonData["sid"].(string); ok && sid != "" {
				confidence += 35
				details += "- Valid Socket.IO session ID detected\n"
			}

			if upgrades, ok := jsonData["upgrades"].([]interface{}); ok {
				for _, upgrade := range upgrades {
					if upgradeStr, ok := upgrade.(string); ok && upgradeStr == "websocket" {
						confidence += 20
						details += "- WebSocket upgrade supported\n"
						break
					}
				}
			}

			configParams := map[string]string{
				"pingInterval": "Ping interval configuration",
				"pingTimeout":  "Ping timeout configuration",
				"maxPayload":   "Maximum payload size configuration",
			}

			for param, description := range configParams {
				if _, ok := jsonData[param]; ok {
					confidence += 20
					details += fmt.Sprintf("- %s present\n", description)
				}
			}
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	if headers != nil {
		if _, hasUpgrade := headers["Upgrade"]; hasUpgrade {
			confidence += 40
			details += "- WebSocket upgrade header present\n"
		}

		if origins, hasCORS := headers["Access-Control-Allow-Origin"]; hasCORS {
			confidence += 5
			details += fmt.Sprintf("- CORS configuration present: %v\n", origins)
		}
	}

	return confidence >= minConfidence(), details, min(confidence, 100)
}

func DiscoverSocketIO(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       SocketIOPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept":     "application/json,text/plain",
				"Connection": "upgrade",
				"Upgrade":    "websocket",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: isSocketIOValidationFunc,
		IssueCode:      db.SocketioDetectedCode,
	})
}
