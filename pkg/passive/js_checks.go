package passive

import (
	"encoding/json"
	"strings"

	"github.com/BishopFox/jsluice"
	"github.com/pyneda/sukyan/db"
)

func PassiveJavascriptSecretsScan(item *db.History) {
	// NOTE: By now we only support javascript, but should also be able to extract scripts from HTML and analyze them.
	body, _ := item.ResponseBody()
	secrets := findSecretsInJavascript(body)
	for _, secret := range secrets {
		db.CreateIssueFromHistoryAndTemplate(item, db.SecretsInJsCode, secret.Details, 90, secret.Severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}

type JavascriptSecret struct {
	Kind     string
	Details  string
	Severity string
}

func jsluiceSeverity(severity jsluice.Severity) string {
	switch severity {
	case jsluice.SeverityInfo:
		return "Info"
	case jsluice.SeverityLow:
		return "Low"
	case jsluice.SeverityMedium:
		return "Medium"
	case jsluice.SeverityHigh:
		return "High"
	default:
		return "Unknown"
	}
}

func findSecretsInJavascript(code []byte) []JavascriptSecret {
	secrets := make([]JavascriptSecret, 0)

	analyzer := jsluice.NewAnalyzer(code)
	for _, match := range analyzer.GetSecrets() {
		var sb strings.Builder
		sb.WriteString("The following " + match.Kind + " secret has been found analyzing the javascript code:\n\n")
		data, err := json.MarshalIndent(match.Data, "", "  ")
		if err != nil {
			continue
		}
		sb.WriteString(string(data))
		context, err := json.MarshalIndent(match.Context, "", "  ")
		if err == nil {
			sb.WriteString("\n\nContext where the secret has been detected:\n\n")
			sb.WriteString(string(context))
		}
		details := sb.String()
		secrets = append(secrets, JavascriptSecret{
			Kind:     match.Kind,
			Details:  details,
			Severity: jsluiceSeverity(match.Severity),
		})
	}
	return secrets
}
