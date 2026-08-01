package passive

import (
	"testing"
)

func TestAnalyzePostMessageWildcardSends(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantCount     int
		wantSensitive bool
		wantDataExpr  string
	}{
		{
			name:          "wildcard with a token identifier is sensitive",
			src:           `parent.postMessage(authToken, "*");`,
			wantCount:     1,
			wantSensitive: true,
			wantDataExpr:  "authToken",
		},
		{
			name:          "wildcard sending document.cookie is sensitive",
			src:           `window.parent.postMessage(document.cookie, '*');`,
			wantCount:     1,
			wantSensitive: true,
			wantDataExpr:  "document.cookie",
		},
		{
			name:          "wildcard sending localStorage is sensitive",
			src:           `opener.postMessage(localStorage.getItem("session"), "*")`,
			wantCount:     1,
			wantSensitive: true,
		},
		{
			name:          "wildcard with an ordinary expression is reported but not sensitive",
			src:           `frames[0].postMessage(buildPayload(), "*");`,
			wantCount:     1,
			wantSensitive: false,
		},
		{
			name:      "wildcard with a string literal is still reported",
			src:       `parent.postMessage("ready", "*");`,
			wantCount: 1,
		},
		{
			name:      "explicit target origin is not reported",
			src:       `parent.postMessage(authToken, "https://trusted.example.com");`,
			wantCount: 0,
		},
		{
			name:      "template literal target origin is not reported",
			src:       "parent.postMessage(data, `${location.origin}`);",
			wantCount: 0,
		},
		{
			name:      "worker postMessage with a single argument is not a target origin",
			src:       `worker.postMessage({cmd: "start"});`,
			wantCount: 0,
		},
		{
			name:          "minified wildcard send is still found",
			src:           `!function(){var e=n.token;t.parent.postMessage(e,"*")}();`,
			wantCount:     1,
			wantSensitive: false,
		},
		{
			name:      "inline script inside html is analysed",
			src:       `<html><body><script>parent.postMessage(sessionKey, "*");</script></body></html>`,
			wantCount: 1,
			// sessionKey matches the sensitive identifier list
			wantSensitive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzePostMessageUsage([]byte(tt.src))

			if len(got.WildcardSends) != tt.wantCount {
				t.Fatalf("got %d wildcard sends, want %d (%+v)", len(got.WildcardSends), tt.wantCount, got.WildcardSends)
			}
			if tt.wantCount == 0 {
				return
			}
			if got.WildcardSends[0].Sensitive != tt.wantSensitive {
				t.Errorf("sensitive = %v, want %v (data expr %q)",
					got.WildcardSends[0].Sensitive, tt.wantSensitive, got.WildcardSends[0].DataExpr)
			}
			if tt.wantDataExpr != "" && got.WildcardSends[0].DataExpr != tt.wantDataExpr {
				t.Errorf("data expr = %q, want %q", got.WildcardSends[0].DataExpr, tt.wantDataExpr)
			}
		})
	}
}

func TestAnalyzePostMessageOriginValidation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want OriginValidation
	}{
		{
			name: "indexOf on the origin is a substring match",
			src: `window.addEventListener("message", function (e) {
				if (e.origin.indexOf("trusted.com") === -1) return;
				document.getElementById("out").innerHTML = e.data;
			});`,
			want: OriginValidationSubstring,
		},
		{
			name: "minified startsWith is a prefix match",
			src:  `addEventListener("message",function(t){t.origin.startsWith("https://x.com")&&(n.innerHTML=t.data)});`,
			want: OriginValidationPrefix,
		},
		{
			name: "destructured parameter with endsWith is a suffix match",
			src: `window.addEventListener("message", ({origin, data}) => {
				if (!origin.endsWith("trusted.com")) return;
				eval(data);
			});`,
			want: OriginValidationSuffix,
		},
		{
			name: "destructuring inside the body still binds the origin",
			src: `window.addEventListener("message", function (e) {
				const { origin, data } = e;
				if (origin !== "https://trusted.com") return;
				render(data);
			});`,
			want: OriginValidationStrong,
		},
		{
			name: "allowlist membership with origin as the argument is strong",
			src: `window.addEventListener("message", function (e) {
				if (!ALLOWED_ORIGINS.includes(e.origin)) return;
				render(e.data);
			});`,
			want: OriginValidationStrong,
		},
		{
			name: "includes called on the origin is a substring match",
			src: `window.addEventListener("message", function (e) {
				if (!e.origin.includes("trusted.com")) return;
				render(e.data);
			});`,
			want: OriginValidationSubstring,
		},
		{
			name: "strict equality against a literal is strong",
			src: `window.onmessage = function (e) {
				if (e.origin === "https://trusted.com") { render(e.data); }
			};`,
			want: OriginValidationStrong,
		},
		{
			name: "unanchored regex is a substring match in disguise",
			src: `window.addEventListener("message", function (e) {
				if (!/trusted\.com/.test(e.origin)) return;
				render(e.data);
			});`,
			want: OriginValidationUnanchoredRegex,
		},
		{
			name: "fully anchored regex is strong",
			src: `window.addEventListener("message", function (e) {
				if (!/^https:\/\/trusted\.com$/.test(e.origin)) return;
				render(e.data);
			});`,
			want: OriginValidationStrong,
		},
		{
			name: "accepting the null origin is reported",
			src: `window.addEventListener("message", function (e) {
				if (e.origin === "null") { render(e.data); }
			});`,
			want: OriginValidationNullAccepted,
		},
		{
			name: "no reference to origin at all",
			src: `window.addEventListener("message", function (e) {
				document.getElementById("out").innerHTML = e.data;
			});`,
			want: OriginValidationNone,
		},
		{
			name: "origin passed to a helper is unresolvable, never 'none'",
			src: `window.addEventListener("message", function (e) {
				if (!isTrustedOrigin(e.origin)) return;
				render(e.data);
			});`,
			want: OriginValidationUnknown,
		},
		{
			name: "the weakest check in the handler wins",
			src: `window.addEventListener("message", function (e) {
				if (e.origin === "https://trusted.com" || e.origin.startsWith("https://cdn.")) {
					render(e.data);
				}
			});`,
			want: OriginValidationPrefix,
		},
		{
			name: "origin read only for logging is not a guard",
			src: `window.addEventListener("message", function (e) {
				console.log("message from " + e.origin);
				document.getElementById("out").innerHTML = e.data;
			});`,
			want: OriginValidationNone,
		},
		{
			name: "origin interpolated into a log template is not a guard",
			src: "window.addEventListener(\"message\", function (e) {\n" +
				"  log(`Received from ${e.origin}`);\n" +
				"  eval(e.data);\n" +
				"});",
			want: OriginValidationNone,
		},
		{
			name: "unanchored regex hoisted into a variable is still classified",
			src: `const trustedPattern = /trusted\.com/;
			window.addEventListener("message", function (e) {
				if (trustedPattern.test(e.origin)) { render(e.data); }
			});`,
			want: OriginValidationUnanchoredRegex,
		},
		{
			name: "anchored regex hoisted into a variable is strong",
			src: `const trustedPattern = /^https:\/\/trusted\.com$/;
			window.addEventListener("message", function (e) {
				if (trustedPattern.test(e.origin)) { render(e.data); }
			});`,
			want: OriginValidationStrong,
		},
		{
			name: "allowlist containing the null origin is reported",
			src: `const allowed = ["null", "https://trusted.com"];
			window.addEventListener("message", function (e) {
				if (allowed.includes(e.origin)) { render(e.data); }
			});`,
			want: OriginValidationNullAccepted,
		},
		{
			name: "inline allowlist without null stays strong",
			src: `window.addEventListener("message", function (e) {
				if (["https://a.com", "https://b.com"].includes(e.origin)) { render(e.data); }
			});`,
			want: OriginValidationStrong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzePostMessageUsage([]byte(tt.src))
			if len(got.Handlers) != 1 {
				t.Fatalf("found %d message handlers, want 1", len(got.Handlers))
			}
			if got.Handlers[0].Validation != tt.want {
				t.Errorf("validation = %v, want %v (evidence %q)",
					got.Handlers[0].Validation, tt.want, got.Handlers[0].Evidence)
			}
		})
	}
}

func TestAnalyzePostMessageFindsHandlerRegistrationForms(t *testing.T) {
	tests := []struct{ name, src string }{
		{"addEventListener", `window.addEventListener("message", function (e) { render(e.data); });`},
		{"onmessage property", `window.onmessage = function (e) { render(e.data); };`},
		{"bare onmessage", `onmessage = function (e) { render(e.data); };`},
		{"jquery on", `$(window).on("message", function (e) { render(e.data); });`},
		{"legacy attachEvent", `window.attachEvent("onmessage", function (e) { render(e.data); });`},
		{"arrow handler", `window.addEventListener("message", (e) => { render(e.data); });`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzePostMessageUsage([]byte(tt.src))
			if len(got.Handlers) != 1 {
				t.Fatalf("found %d message handlers, want 1", len(got.Handlers))
			}
		})
	}
}

func TestAnalyzePostMessageIgnoresOtherEvents(t *testing.T) {
	src := `window.addEventListener("click", function (e) { render(e.data); });
	        document.addEventListener("DOMContentLoaded", function () {});`

	if got := AnalyzePostMessageUsage([]byte(src)); len(got.Handlers) != 0 {
		t.Errorf("found %d handlers for non-message events, want 0", len(got.Handlers))
	}
}
