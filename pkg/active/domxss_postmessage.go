package active

import (
	"fmt"

	"github.com/pyneda/sukyan/pkg/web"
)

// buildPostMessageScripts renders the payload into the message shapes a handler
// is likely to read: a bare string, an object under the property names handlers
// commonly pick, and a JSON string.
//
// Each script is an arrow function because rod wraps non-function JS as
// `function() { return <js> }`. A bare statement block becomes a SyntaxError
// there and nothing is ever posted.
func buildPostMessageScripts(payloadValue string) []string {
	escaped := web.EscapeJSString(payloadValue)

	return []string{
		fmt.Sprintf(`() => {
			try {
				window.postMessage('%s', '*');
			} catch(e) {}
		}`, escaped),
		fmt.Sprintf(`() => {
			try {
				window.postMessage({data: '%s', message: '%s', content: '%s', html: '%s', text: '%s', value: '%s'}, '*');
			} catch(e) {}
		}`, escaped, escaped, escaped, escaped, escaped, escaped),
		fmt.Sprintf(`() => {
			try {
				window.postMessage(JSON.stringify({payload: '%s'}), '*');
			} catch(e) {}
		}`, escaped),
	}
}
