package passive

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

func ReactDevelopmentModeScan(item *db.History) {
	if item.StatusCode == 404 {
		return
	}

	var sb strings.Builder
	confidence := 0
	bodyStr := string(item.ResponseBody)

	devBundlePatterns := []string{
		"react.development.js",
		"react-dom.development.js",
	}

	sb.WriteString("The following fingerprints have been detected:\n")
	for _, pattern := range devBundlePatterns {
		if strings.Contains(item.URL, pattern) {
			confidence += 50
			sb.WriteString(fmt.Sprintf("- Development bundle URL detected: %s\n", pattern))
		}
		if strings.Contains(bodyStr, pattern) {
			confidence += 20
			sb.WriteString(fmt.Sprintf("- Reference to development bundle found: %s\n", pattern))
		}
	}

	if strings.Contains(bodyStr, "@license React") && strings.Contains(bodyStr, "development.js") {
		confidence += 20
		sb.WriteString("- React development mode license header found\n")
	}

	indicatorPatterns := []string{
		"// eslint-disable-next-line",
		"* @param {ReactClass}",
		"issues in DEV builds",
		"// In production",
		"// Match production behavior",
		"react-internal/safe-string-coercion",
	}

	for _, pattern := range indicatorPatterns {
		if strings.Contains(bodyStr, pattern) {
			confidence += 25
			sb.WriteString(fmt.Sprintf("- Common development pattern detected: %s\n", pattern))
		}
	}

	// NOTE: Version could be extracted from: var ReactVersion = '18.3.1';

	if confidence >= 50 {
		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.ReactDevelopmentModeCode,
			sb.String(),
			min(confidence, 100),
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
		)
	}
}
