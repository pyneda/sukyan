package active

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// csppPayloadSeparators lists where a payload can be appended to a URL. The
// fragment is always tested: it is never sent to the server, so a hash-sourced
// pollution cannot be reached any other way.
func csppPayloadSeparators(rawURL string) []string {
	if strings.Contains(stripURLFragment(rawURL), "?") {
		return []string{"&", "#"}
	}
	return []string{"?", "#"}
}

// buildCSPPTestURL appends payload to base at separator. A fragment payload
// replaces whatever fragment the URL already carries instead of appending a
// second one, which the page would read as part of the first.
func buildCSPPTestURL(base, separator, payload string) string {
	if separator == "#" {
		return stripURLFragment(base) + "#" + payload
	}
	return base + separator + payload
}

// stripURLFragment removes the fragment from a URL. Requests never carry one,
// so history lookups have to be keyed on the stripped form.
func stripURLFragment(rawURL string) string {
	if i := strings.Index(rawURL, "#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

// navigateForPayload loads url, forcing a full document load even when only the
// fragment differs from the current location. A fragment-only navigation is a
// same-document navigation: the document is not re-fetched and never re-parses,
// so the page would still be holding the previous payload's state.
func navigateForPayload(page *rod.Page, url string, timeout time.Duration) error {
	if err := page.Timeout(timeout).Navigate("about:blank"); err != nil {
		return err
	}
	if err := page.Timeout(timeout).WaitLoad(); err != nil {
		return err
	}
	return page.Timeout(timeout).Navigate(url)
}
