package passive

// Analysis of window.postMessage usage.
//
// Two things are worth reporting from source alone, and both are decidable
// locally, which is what keeps this honest on real-world bundles:
//
//   - a send with "*" as the target origin, which hands the message to whatever
//     document currently occupies the target frame
//   - a receiver whose origin check is demonstrably too loose
//
// Proving that a handler has *no* origin check is a different problem: the check
// is often a call into a helper, so absence cannot be established without
// whole-program analysis. Anything unresolvable is left unclassified rather than
// reported.
//
// https://medium.com/kminthein/postmessage-vuln-what-is-it-1faef47f22fb

import (
	"strings"

	"github.com/BishopFox/jsluice"
	"github.com/pyneda/sukyan/db"
)

// TargetOriginFinding is a postMessage call that sends to any origin.
type TargetOriginFinding struct {
	Evidence  string
	DataExpr  string
	Sensitive bool
}

// PostMessageAnalysis is everything found in one script or document.
type PostMessageAnalysis struct {
	WildcardSends []TargetOriginFinding
	Handlers      []MessageHandlerFinding
}

// sensitiveDataHints are substrings that raise a wildcard send from "sloppy" to
// "leaking something that matters". Deliberately excludes bare "key", which
// matches keyCode, keyPress and friends.
var sensitiveDataHints = []string{
	"token", "auth", "session", "jwt", "secret", "password", "passwd",
	"credential", "apikey", "api_key", "privatekey", "private_key",
	"accesskey", "access_key", "bearer", "cookie",
}

const postMessageCallQuery = `[
	(call_expression
		function: (member_expression property: (property_identifier) @prop)
		arguments: (arguments) @args)
	(call_expression
		function: (identifier) @prop
		arguments: (arguments) @args)
]`

// AnalyzePostMessageUsage parses source and reports insecure postMessage usage.
// The source may be a JavaScript file or an HTML document; jsluice extracts
// inline scripts on its own.
func AnalyzePostMessageUsage(src []byte) PostMessageAnalysis {
	analysis := PostMessageAnalysis{}
	if len(src) == 0 {
		return analysis
	}

	analyzer := jsluice.NewAnalyzer(src)

	analyzer.QueryMulti(postMessageCallQuery, func(res jsluice.QueryResult) {
		prop, args := res["prop"], res["args"]
		if prop == nil || args == nil || prop.Content() != "postMessage" {
			return
		}

		params := args.NamedChildren()
		if len(params) < 2 {
			// A single argument is not a target origin: that is the Worker and
			// MessagePort form, which has no origin to get wrong.
			return
		}

		target := params[1]
		if target.Type() != "string" || target.RawString() != "*" {
			return
		}

		data := params[0]
		analysis.WildcardSends = append(analysis.WildcardSends, TargetOriginFinding{
			Evidence:  strings.TrimSpace(args.Parent().Content()),
			DataExpr:  data.Content(),
			Sensitive: looksSensitive(data.Content()),
		})
	})

	analysis.Handlers = analyzeMessageHandlers(analyzer)

	return analysis
}

// looksSensitive reports whether a broadcast expression names something worth
// stealing. String and numeric literals never qualify: their value is already
// public by virtue of being in the source.
func looksSensitive(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return false
	}
	if c := trimmed[0]; c == '"' || c == '\'' || c == '`' {
		return false
	}

	lowered := strings.ToLower(trimmed)
	for _, hint := range sensitiveDataHints {
		if strings.Contains(lowered, hint) {
			return true
		}
	}
	return false
}

// PostMessageScan reports insecure postMessage usage in an HTML document or a
// JavaScript asset.
//
// Only weaknesses that are decidable from the source are reported. A handler
// with no visible origin check is deliberately not reported: the check is often
// a call into a helper, and claiming its absence without whole-program analysis
// produces noise on real applications.
func PostMessageScan(item *db.History) {
	body, err := item.ResponseBody()
	if err != nil || len(body) == 0 {
		return
	}

	analysis := AnalyzePostMessageUsage(body)

	for _, handler := range analysis.Handlers {
		if !handler.Validation.Weak() {
			continue
		}

		var sb strings.Builder
		sb.WriteString("A postMessage handler validates the sender origin with a ")
		sb.WriteString(weakValidationLabel(handler.Validation))
		sb.WriteString(".\n\n")
		sb.WriteString("Guard: " + handler.Evidence + "\n")
		if len(handler.Sinks) > 0 {
			sb.WriteString("\nMessage data reaches these sinks in the handler:\n")
			for _, sink := range handler.Sinks {
				sb.WriteString("  - " + sink + "\n")
			}
		}

		severity := "Medium"
		if len(handler.Sinks) > 0 {
			severity = "High"
		}

		db.CreateIssueFromHistoryAndTemplate(item, db.PostmessageWeakOriginValidationCode,
			sb.String(), 85, severity, item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}

	for _, send := range analysis.WildcardSends {
		if !send.Sensitive {
			continue
		}

		details := "A postMessage call sends to any origin (\"*\").\n\n" +
			"Call: " + send.Evidence + "\n" +
			"Data sent: " + send.DataExpr + "\n\n" +
			"The expression being sent names credential-like data, so any document " +
			"that occupies the target frame receives it."

		db.CreateIssueFromHistoryAndTemplate(item, db.PostmessageInsecureTargetOriginCode,
			details, 85, "High", item.WorkspaceID, item.TaskID, &defaultTaskJobID, item.ScanID, item.ScanJobID)
	}
}

func weakValidationLabel(v OriginValidation) string {
	switch v {
	case OriginValidationSubstring:
		return "substring match, which accepts any origin merely containing the trusted string"
	case OriginValidationPrefix:
		return "prefix match, which accepts origins such as https://trusted.example.attacker.test"
	case OriginValidationSuffix:
		return "suffix match, which accepts origins such as https://eviltrusted.example"
	case OriginValidationUnanchoredRegex:
		return "regular expression that is not anchored at both ends, so it matches a substring of the origin"
	case OriginValidationNullAccepted:
		return "check that trusts the literal \"null\" origin, which any sandboxed iframe or data URI presents"
	}
	return "weak comparison"
}
