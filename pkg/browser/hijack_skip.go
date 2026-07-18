package browser

import "strings"

// trackerHostSuffixes are third-party analytics, advertising and social-embed
// domains whose requests add noise without contributing crawlable surface.
// Matching is by hostname suffix so only these domains (and their subdomains)
// are skipped. Content-delivery and library hosts that share a brand name -
// such as googleapis.com, gstatic.com or fonts.googleapis.com - are deliberately
// NOT listed, because they serve application code (framework runtimes, fonts)
// that pages need in order to render and route.
var trackerHostSuffixes = []string{
	"google-analytics.com",
	"googletagmanager.com",
	"googlesyndication.com",
	"googleadservices.com",
	"doubleclick.net",
	"hotjar.com",
	"mc.yandex.ru",
	"connect.facebook.net",
	"facebook.com",
	"pinterest.com",
	"instagram.com",
	"tiktok.com",
	"analytics.tiktok.com",
	"ads-twitter.com",
	"static.ads-twitter.com",
}

// skipHijackHosts are exact hosts that must never be processed, independent of
// any domain-suffix logic (e.g. a reserved probe address).
var skipHijackHosts = map[string]bool{
	"127.0.0.2": true,
}

// shouldSkipHijackHost reports whether requests to the given host should be
// dropped from crawl/scan processing because they are known tracking or embed
// endpoints rather than crawlable application resources.
func shouldSkipHijackHost(host string) bool {
	if host == "" {
		return false
	}
	if idx := strings.IndexByte(host, ':'); idx >= 0 {
		host = host[:idx]
	}
	host = strings.ToLower(host)
	if skipHijackHosts[host] {
		return true
	}
	for _, suffix := range trackerHostSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
