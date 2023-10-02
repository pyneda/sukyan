package passive

import (
	"encoding/json"
	"github.com/BishopFox/jsluice"
	"github.com/pyneda/sukyan/db"
	"regexp"
	"strings"
)

// 1. Outdated libraries matching could be based on retirejs dataset.
// For usage implementation can see:
// - https://github.com/FallibleInc/retirejslib
// - https://github.com/stamparm/DSJS/blob/master/dsjs.py

// 2. Should also have some regex or ways to detect unsafe JS code such as eval(), .innerHTML() or usage of user controllable inputs.
// https://github.com/wisec/domxsswiki/wiki/Finding-DOMXSS

// Regular expression patterns
const (
	CommonJsSourcesPattern   = `/(location\s*[\[.])|([.\[]\s*["']?\s*(arguments|dialogArguments|innerHTML|write(ln)?|open(Dialog)?|showModalDialog|cookie|URL|documentURI|baseURI|referrer|name|opener|parent|top|content|self|frames)\W)|(localStorage|sessionStorage|Database)/`
	CommonJsSinksPattern     = `/((src|href|data|location|code|value|action)\s*["'\]]*\s*\+?\s*=)|((replace|assign|navigate|getResponseHeader|open(Dialog)?|showModalDialog|eval|evaluate|execCommand|execScript|setTimeout|setInterval)\s*["'\]]*\s*\()/`
	CommonJquerySinksPattern = `/after\(|\.append\(|\.before\(|\.html\(|\.prepend\(|\.replaceWith\(|\.wrap\(|\.wrapAll\(|\$\(|\.globalEval\(|\.add\(|jQuery\(|\$\(|\.parseHTML\(/`
)

// Compiled regular expressions
var (
	CommonJsSourcesRegex   = regexp.MustCompile(CommonJsSourcesPattern)
	CommonJsSinksRegex     = regexp.MustCompile(CommonJsSinksPattern)
	CommonJquerySinksRegex = regexp.MustCompile(CommonJquerySinksPattern)
)

func match(text string, regex *regexp.Regexp) []string {
	parsed := regex.FindAllString(text, -1)
	return parsed
}

// findJsSources searches for common javascript sources
func findJsSources(text string) []string {
	return match(text, CommonJsSourcesRegex)
}

// findJsSinks searches for common javascript sinks
func findJsSinks(text string) []string {
	return match(text, CommonJsSinksRegex)
}

// findJquerySinks searches for common jquery sinks
func findJquerySinks(text string) []string {
	return match(text, CommonJquerySinksRegex)
}

func PassiveJavascriptScan(item *db.History) {
	bodyStr := string(item.ResponseBody)
	jsSources := findJsSources(bodyStr)
	jsSinks := findJsSinks(bodyStr)
	jquerySinks := findJquerySinks(bodyStr)
	// log.Info().Str("url", item.URL).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked HTML response")
	if len(jsSources) > 0 || len(jsSinks) > 0 || len(jquerySinks) > 0 {
		CreateJavascriptSourcesAndSinksInformationalIssue(item, jsSources, jsSinks, jquerySinks)
	}
}

func passiveJavascriptSecretsScan(item *db.History) {
	// NOTE: By now we only support javascript, but should also be able to extract scripts from HTML and analyze them.
	secrets := findSecretsInJavascript(item.ResponseBody)
	for _, secret := range secrets {
		db.CreateIssueFromHistoryAndTemplate(item, db.SecretsInJsCode, secret.Details, 90, secret.Severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID)
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
