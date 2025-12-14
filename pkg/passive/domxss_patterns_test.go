package passive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindDOMXSSSources(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedCount  int
		shouldContain  []string
		shouldNotMatch bool
	}{
		{
			name: "location.hash access",
			input: `<script>
				var hash = location.hash;
				document.getElementById('output').innerHTML = hash;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"location."},
		},
		{
			name: "location bracket notation",
			input: `<script>
				var loc = window["location"]["hash"];
			</script>`,
			expectedCount: 1, // window["location"] matches the bracket notation pattern
			shouldContain: []string{"window[\"location\"]"},
		},
		{
			name: "URLSearchParams usage",
			input: `<script>
				const urlParams = new URLSearchParams(location.search);
				const query = urlParams.get('query');
			</script>`,
			expectedCount: 2, // URLSearchParams and location.
			shouldContain: []string{"URLSearchParams", "location."},
		},
		{
			name: "document.referrer access",
			input: `<script>
				var ref = document.referrer;
				console.log(ref);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"document.referrer"},
		},
		{
			name: "localStorage access",
			input: `<script>
				var data = localStorage.getItem('user');
				var session = sessionStorage["token"];
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"localStorage.", "sessionStorage["},
		},
		{
			name: "postMessage listener",
			input: `<script>
				window.addEventListener("message", function(e) {
					document.body.innerHTML = e.data;
				});
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"addEventListener(\"message\""},
		},
		{
			name: "jQuery message handler",
			input: `<script>
				$(window).on('message', function(e) {
					$('#output').html(e.originalEvent.data);
				});
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".on('message'"},
		},
		{
			name: "history.state access",
			input: `<script>
				var state = history.state;
				window.addEventListener('popstate', function(e) {
					renderPage(e.state);
				});
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"history.state", "popstate"},
		},
		{
			name: "new URL constructor",
			input: `<script>
				const url = new URL(location.href);
				const param = url.searchParams.get('redirect');
			</script>`,
			expectedCount: 2, // new URL and location.
			shouldContain: []string{"new URL(", "location."},
		},
		{
			name: "window.name access",
			input: `<script>
				var name = window.name;
				eval(name);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"window.name"},
		},
		{
			name: "document.cookie access",
			input: `<script>
				var cookies = document.cookie;
				document.write(cookies);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"document.cookie"},
		},
		{
			name: "opener access",
			input: `<script>
				if (opener) {
					opener.postMessage(data, '*');
				}
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"opener"},
		},
		{
			name: "no sources - safe code",
			input: `<script>
				console.log("Hello World");
				var x = 1 + 2;
			</script>`,
			shouldNotMatch: true,
		},
		{
			name: "hashchange event",
			input: `<script>
				window.addEventListener('hashchange', function() {
					loadContent(location.hash);
				});
			</script>`,
			expectedCount: 2, // hashchange and location.
			shouldContain: []string{"hashchange", "location."},
		},
		{
			name: "FormData constructor",
			input: `<script>
				var formData = new FormData(document.getElementById('form'));
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"FormData("},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := FindDOMXSSSources(tt.input)

			if tt.shouldNotMatch {
				assert.Empty(t, matches, "Expected no matches for safe code")
				return
			}

			assert.GreaterOrEqual(t, len(matches), tt.expectedCount,
				"Expected at least %d matches, got %d: %v", tt.expectedCount, len(matches), matches)

			for _, expected := range tt.shouldContain {
				found := false
				for _, match := range matches {
					if contains(match, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find pattern containing '%s' in matches: %v", expected, matches)
			}
		})
	}
}

func TestFindDOMXSSSinks(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedCount  int
		shouldContain  []string
		shouldNotMatch bool
	}{
		{
			name: "innerHTML assignment",
			input: `<script>
				document.getElementById('output').innerHTML = userInput;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".innerHTML ="},
		},
		{
			name: "outerHTML assignment",
			input: `<script>
				element.outerHTML = data;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".outerHTML ="},
		},
		{
			name: "document.write call",
			input: `<script>
				document.write(content);
				document.writeln(moreContent);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"document.write(", "document.writeln("},
		},
		{
			name: "eval call",
			input: `<script>
				eval(userCode);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"eval("},
		},
		{
			name: "setTimeout with string",
			input: `<script>
				setTimeout(callback, 1000);
				setInterval(repeater, 500);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"setTimeout(", "setInterval("},
		},
		{
			name: "new Function constructor",
			input: `<script>
				var fn = new Function('return ' + code);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"new Function("},
		},
		{
			name: "location.href assignment",
			input: `<script>
				location.href = redirectUrl;
				window.location = target;
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"location.href =", "location ="},
		},
		{
			name: "location.assign and replace",
			input: `<script>
				location.assign(url);
				location.replace(newUrl);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"location.assign(", "location.replace("},
		},
		{
			name: "src attribute assignment",
			input: `<script>
				img.src = userUrl;
				script.src = scriptUrl;
			</script>`,
			expectedCount: 1, // Deduped to single match
			shouldContain: []string{".src ="},
		},
		{
			name: "href attribute assignment",
			input: `<script>
				link.href = destination;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".href ="},
		},
		{
			name: "insertAdjacentHTML",
			input: `<script>
				element.insertAdjacentHTML('beforeend', html);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"insertAdjacentHTML("},
		},
		{
			name: "appendChild",
			input: `<script>
				container.appendChild(newElement);
				parent.insertBefore(newNode, refNode);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{".appendChild(", ".insertBefore("},
		},
		{
			name: "setAttribute with dangerous attributes",
			input: `<script>
				element.setAttribute("onclick", handler);
				element.setAttribute('src', url);
				element.setAttribute("href", link);
			</script>`,
			expectedCount: 3,
			shouldContain: []string{"setAttribute(\"onclick", "setAttribute('src", "setAttribute(\"href"},
		},
		{
			name: "event handler properties",
			input: `<script>
				element.onclick = handler;
				element.onerror = errorHandler;
				element.onload = loadHandler;
			</script>`,
			expectedCount: 3,
			shouldContain: []string{".onclick =", ".onerror =", ".onload ="},
		},
		{
			name: "React dangerouslySetInnerHTML",
			input: `<div dangerouslySetInnerHTML={{ __html: content }} />`,
			expectedCount: 2, // dangerouslySetInnerHTML and __html:
			shouldContain: []string{"dangerouslySetInnerHTML", "__html:"},
		},
		{
			name: "Vue v-html directive",
			input: `<template>
				<div v-html="userContent"></div>
			</template>`,
			expectedCount: 1,
			shouldContain: []string{"v-html="},
		},
		{
			name: "Angular bypassSecurityTrust",
			input: `<script>
				this.sanitizer.bypassSecurityTrustHtml(html);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"bypassSecurityTrust"},
		},
		{
			name: "Angular innerHTML binding",
			input: `<div [innerHTML]="content"></div>`,
			expectedCount: 1,
			shouldContain: []string{"[innerHTML]="},
		},
		{
			name: "AngularJS ng-bind-html",
			input: `<div ng-bind-html="trustedHtml"></div>`,
			expectedCount: 1,
			shouldContain: []string{"ng-bind-html"},
		},
		{
			name: "jQuery html method",
			input: `<script>
				$('#output').html(data);
				jQuery('.content').append(userHtml);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{".html(", ".append("},
		},
		{
			name: "jQuery globalEval",
			input: `<script>
				$.globalEval(code);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".globalEval("},
		},
		{
			name: "createContextualFragment",
			input: `<script>
				var fragment = range.createContextualFragment(html);
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"createContextualFragment("},
		},
		{
			name: "DOMParser parseFromString",
			input: `<script>
				var parser = new DOMParser();
				var doc = parser.parseFromString(html, 'text/html');
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".parseFromString("},
		},
		{
			name: "indirect eval pattern",
			input: `<script>
				(1, eval)(code);
				window["eval"](code);
			</script>`,
			expectedCount: 2,
			shouldContain: []string{"eval)(", "window[\"eval\"]"},
		},
		{
			name: "iframe srcdoc",
			input: `<script>
				iframe.srcdoc = htmlContent;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".srcdoc ="},
		},
		{
			name: "bracket notation innerHTML",
			input: `<script>
				element["innerHTML"] = content;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{"[\"innerHTML\"]"},
		},
		{
			name: "no sinks - safe code",
			input: `<script>
				console.log("Hello World");
				var x = document.getElementById('safe');
				x.textContent = userInput;
			</script>`,
			shouldNotMatch: true,
		},
		{
			name: "form action assignment",
			input: `<script>
				form.action = redirectUrl;
			</script>`,
			expectedCount: 1,
			shouldContain: []string{".action ="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := FindDOMXSSSinks(tt.input)

			if tt.shouldNotMatch {
				assert.Empty(t, matches, "Expected no matches for safe code")
				return
			}

			assert.GreaterOrEqual(t, len(matches), tt.expectedCount,
				"Expected at least %d matches, got %d: %v", tt.expectedCount, len(matches), matches)

			for _, expected := range tt.shouldContain {
				found := false
				for _, match := range matches {
					if contains(match, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find pattern containing '%s' in matches: %v", expected, matches)
			}
		})
	}
}

func TestHasDOMXSSIndicators(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectSources  bool
		expectSinks    bool
	}{
		{
			name: "vulnerable code - source to sink",
			input: `<script>
				var hash = location.hash;
				document.getElementById('output').innerHTML = hash;
			</script>`,
			expectSources: true,
			expectSinks:   true,
		},
		{
			name: "only sources - no sinks",
			input: `<script>
				var params = new URLSearchParams(location.search);
				console.log(params.get('query'));
			</script>`,
			expectSources: true,
			expectSinks:   false,
		},
		{
			name: "only sinks - no sources",
			input: `<script>
				var staticContent = "<p>Hello</p>";
				element.innerHTML = staticContent;
			</script>`,
			expectSources: false,
			expectSinks:   true,
		},
		{
			name: "safe code - no sources or sinks",
			input: `<script>
				var x = 1 + 2;
				console.log(x);
				document.getElementById('output').textContent = "safe";
			</script>`,
			expectSources: false,
			expectSinks:   false,
		},
		{
			name: "postMessage to innerHTML",
			input: `<script>
				window.addEventListener('message', function(e) {
					document.getElementById('output').innerHTML = e.data;
				});
			</script>`,
			expectSources: true,
			expectSinks:   true,
		},
		{
			name: "localStorage to eval",
			input: `<script>
				var code = localStorage.getItem('code');
				eval(code);
			</script>`,
			expectSources: true,
			expectSinks:   true,
		},
		{
			name: "jQuery DOM XSS pattern",
			input: `<script>
				var hash = location.hash.substring(1);
				$('#content').html(hash);
			</script>`,
			expectSources: true,
			expectSinks:   true,
		},
		{
			name: "React vulnerable pattern",
			input: `
				const url = new URL(window.location.href);
				const content = url.searchParams.get('content');
				return <div dangerouslySetInnerHTML={{ __html: content }} />;
			`,
			expectSources: true,
			expectSinks:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasSources, hasSinks := HasDOMXSSIndicators(tt.input)

			assert.Equal(t, tt.expectSources, hasSources,
				"Sources detection mismatch for '%s'", tt.name)
			assert.Equal(t, tt.expectSinks, hasSinks,
				"Sinks detection mismatch for '%s'", tt.name)
		})
	}
}

func TestAdvancedHasSourcesOrSinks(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectSources  bool
		expectSinks    bool
	}{
		{
			name:           "text too short",
			input:          "short",
			expectSources:  false,
			expectSinks:    false,
		},
		{
			name:           "obfuscated location access",
			input:          `window["location"]["hash"]`,
			expectSources:  true,
			expectSinks:    false,
		},
		{
			name:           "obfuscated eval",
			input:          `window["eval"](userInput)`,
			expectSources:  true, // window["eval"] matches source pattern too
			expectSinks:    true,
		},
		{
			name:           "indirect eval",
			input:          `var result = (1, eval)(userProvidedCode)`, // Pattern matches [01], needs to be > 20 chars
			expectSources:  false,
			expectSinks:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasSources, hasSinks := AdvancedHasSourcesOrSinks(tt.input)

			assert.Equal(t, tt.expectSources, hasSources,
				"Sources detection mismatch for '%s'", tt.name)
			assert.Equal(t, tt.expectSinks, hasSinks,
				"Sinks detection mismatch for '%s'", tt.name)
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
