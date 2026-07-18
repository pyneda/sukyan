package passive

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
)

// cspReportURIRegex captures the endpoints declared by the report-uri and
// report-to CSP directives, which the generic URL extractor misses because the
// target is not quoted.
var cspReportURIRegex = regexp.MustCompile(`(?i)report-(?:uri|to)\s+([^;'"]+)`)

// extractURLsFromMetaTags parses http-equiv meta tags whose content attribute
// encodes a URL that the generic extractor cannot see: meta refresh redirects
// and CSP report endpoints. URLs are resolved against base.
func extractURLsFromMetaTags(body string, base *url.URL) ExtractedURLS {
	webURLs := make([]string, 0)
	nonWebURLs := make([]string, 0)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(body)))
	if err != nil {
		log.Debug().Err(err).Msg("Failed to parse HTML for meta tag URL extraction")
		return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
	}

	add := func(raw string) {
		for field := range strings.FieldsSeq(raw) {
			field = strings.Trim(field, `'"`)
			if field == "" {
				continue
			}
			absoluteURL, urlType, err := analyzeURL(field, base)
			if err != nil {
				continue
			}
			switch urlType {
			case "web":
				webURLs = append(webURLs, absoluteURL)
			case "non-web":
				nonWebURLs = append(nonWebURLs, absoluteURL)
			}
		}
	}

	doc.Find("meta[http-equiv]").Each(func(i int, s *goquery.Selection) {
		httpEquiv, _ := s.Attr("http-equiv")
		content, hasContent := s.Attr("content")
		if !hasContent {
			return
		}
		switch {
		case strings.EqualFold(httpEquiv, "refresh"):
			if match := refreshURLRegex.FindStringSubmatch(content); match != nil {
				add(match[1])
			}
		case strings.EqualFold(httpEquiv, "content-security-policy"):
			for _, match := range cspReportURIRegex.FindAllStringSubmatch(content, -1) {
				add(match[1])
			}
		}
	})

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
}
