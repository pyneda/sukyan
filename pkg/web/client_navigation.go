package web

import (
	"net/url"
	"strings"

	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
)

// ResolveClientRoute normalizes a captured client-side navigation URL into a
// fetchable absolute URL. rawURL is an already-absolute URL as observed in the
// browser (location.href or an anchor's resolved href). baseURI is
// document.baseURI (honors <base href>). It returns the fetchable URL and true,
// or "" and false when the input yields no meaningful server-fetchable route.
func ResolveClientRoute(rawURL, baseURI string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}

	frag := u.Fragment
	if frag == "" {
		u.Fragment = ""
		return u.String(), true
	}

	if strings.HasPrefix(frag, "!") || strings.HasPrefix(frag, "/") {
		route := strings.TrimPrefix(frag, "!")
		route = strings.TrimPrefix(route, "/")
		if route == "" {
			u.Fragment = ""
			return u.String(), true
		}
		base, err := url.Parse(baseURI)
		if err != nil {
			return "", false
		}
		ref, err := url.Parse(route)
		if err != nil {
			return "", false
		}
		return base.ResolveReference(ref).String(), true
	}

	u.Fragment = ""
	return u.String(), true
}

// ClientNavigationHookScript hooks the History API and hash routing so client-side
// navigations (which emit no network request) are captured. Each navigation's URL is
// resolved to an absolute string in-browser (honoring <base href> and the current
// document) and appended to window.__sukyanClientNav, deduped within the page.
const ClientNavigationHookScript = `
(function() {
    if (window.__sukyanClientNavReady) return;
    window.__sukyanClientNav = [];
    var seen = {};

    function record(u) {
        try {
            if (!u) return;
            var a = document.createElement('a');
            a.href = u;
            var abs = a.href;
            if (!abs || seen[abs]) return;
            seen[abs] = true;
            window.__sukyanClientNav.push(abs);
        } catch (e) {}
    }

    var origPushState = history.pushState;
    history.pushState = function(state, title, url) {
        if (url != null) record(url);
        return origPushState.apply(history, arguments);
    };

    var origReplaceState = history.replaceState;
    history.replaceState = function(state, title, url) {
        if (url != null) record(url);
        return origReplaceState.apply(history, arguments);
    };

    window.addEventListener('popstate', function() { record(location.href); });
    window.addEventListener('hashchange', function() { record(location.href); });

    window.__sukyanClientNavReady = true;
})();
`

// DrainClientNavigations reads and clears the page's captured client navigation
// buffer, resolves each entry via ResolveClientRoute, and returns the deduped
// fetchable absolute URLs. It never returns an error; on any page-eval failure it
// logs at debug and returns nil.
func DrainClientNavigations(page *rod.Page) []string {
	result, err := page.Eval(`() => {
		if (!window.__sukyanClientNav) return { urls: [], baseURI: document.baseURI };
		var out = window.__sukyanClientNav.slice();
		window.__sukyanClientNav = [];
		return { urls: out, baseURI: document.baseURI };
	}`)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to drain client navigations")
		return nil
	}

	baseURI := result.Value.Get("baseURI").Str()
	arr := result.Value.Get("urls").Arr()
	if len(arr) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(arr))
	var urls []string
	for _, v := range arr {
		raw := v.Str()
		if raw == "" {
			continue
		}
		resolved, ok := ResolveClientRoute(raw, baseURI)
		if !ok {
			continue
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		urls = append(urls, resolved)
	}
	return urls
}
