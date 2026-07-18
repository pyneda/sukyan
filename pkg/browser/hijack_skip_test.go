package browser

import "testing"

func TestShouldSkipHijackHost(t *testing.T) {
	tests := map[string]bool{
		// Tracker/analytics/embed hosts that SHOULD be skipped.
		"www.google-analytics.com":   true,
		"google-analytics.com":       true,
		"www.googletagmanager.com":   true,
		"pagead2.googlesyndication.com": true,
		"stats.g.doubleclick.net":    true,
		"static.hotjar.com":          true,
		"mc.yandex.ru":               true,
		"connect.facebook.net":       true,
		"www.facebook.com":           true,
		"analytics.tiktok.com":       true,
		"127.0.0.2":                  true,
		"127.0.0.2:8080":             true,

		// Application/CDN/library hosts that MUST NOT be skipped, even though
		// they share a brand substring with a tracker.
		"ajax.googleapis.com":     false,
		"fonts.googleapis.com":    false,
		"www.gstatic.com":         false,
		"apis.google.com":         false,
		"maps.googleapis.com":     false,
		"storage.googleapis.com":  false,
		"cdn.jsdelivr.net":        false,
		"unpkg.com":               false,
		"example.com":             false,
		"myfacebookclone.com":     false, // not a facebook.com subdomain
		"notyandex.example.com":   false,
		"":                        false,
	}
	for host, want := range tests {
		if got := shouldSkipHijackHost(host); got != want {
			t.Errorf("shouldSkipHijackHost(%q) = %v, want %v", host, got, want)
		}
	}
}
