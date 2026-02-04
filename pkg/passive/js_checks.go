package passive

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BishopFox/jsluice"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
)

func PassiveJavascriptSecretsScan(item *db.History) {
	body, _ := item.ResponseBody()
	secrets := findSecretsInJavascript(body)
	for _, secret := range secrets {
		db.CreateIssueFromHistoryAndTemplate(item, db.SecretsInJsCode, secret.Details, 90, secret.Severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}

func PassiveJSONSecretsScan(item *db.History) {
	body, err := item.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to get response body for JSON secrets scan")
		return
	}
	secrets := findSecretsInJSON(body)
	for _, secret := range secrets {
		db.CreateIssueFromHistoryAndTemplate(item, db.SecretsInJsonCode, secret.Details, 90, secret.Severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}

func PassiveHTMLJavascriptSecretsScan(item *db.History) {
	body, err := item.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to get response body for HTML JS secrets scan")
		return
	}

	scripts := web.ExtractJavascriptFromHTML(body)
	if len(scripts) == 0 {
		return
	}

	for _, script := range scripts {
		secrets := findSecretsInJavascript([]byte(script.Code))
		for _, secret := range secrets {
			details := fmt.Sprintf("Source: %s\n\n%s", script.Source, secret.Details)
			db.CreateIssueFromHistoryAndTemplate(item, db.SecretsInJsCode, details, 90, secret.Severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
		}
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

func wrapJSONAsJS(jsonData []byte) []byte {
	buf := make([]byte, 0, len(jsonData)+len("var _=;")+1)
	buf = append(buf, "var _="...)
	buf = append(buf, jsonData...)
	buf = append(buf, ';')
	return buf
}

func findSecrets(code []byte, sourceLabel string) []JavascriptSecret {
	secrets := make([]JavascriptSecret, 0)

	analyzer := jsluice.NewAnalyzer(code)
	for _, match := range analyzer.GetSecrets() {
		var sb strings.Builder
		sb.WriteString("The following " + match.Kind + " secret has been found analyzing the " + sourceLabel + ":\n\n")
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

func findSecretsInJavascript(code []byte) []JavascriptSecret {
	return findSecrets(code, "javascript code")
}

func findSecretsInJSON(jsonData []byte) []JavascriptSecret {
	return findSecrets(wrapJSONAsJS(jsonData), "JSON response")
}
