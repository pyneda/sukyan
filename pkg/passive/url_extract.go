package passive

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"net/url"
	"strings"
)

func ExtractURLs(response string) []string {
	return urlRegex.FindAllString(response, -1)
}

type ExtractedURLS struct {
	Web    []string
	NonWeb []string
}

// ExtractAndAnalyzeURLS extracts urls from a response and analyzes them. It separates web and non web urls and if relative URLs are found, it makes them absolute based on the extractedFromURL parameter it also fixes other cases like //example.com
func ExtractAndAnalyzeURLS(response string, extractedFromURL string) ExtractedURLS {
	urls := ExtractURLs(response)
	webURLs := make([]string, 0)
	nonWebURLs := make([]string, 0)

	base, err := url.Parse(extractedFromURL)
	if err != nil {
		log.Error().Err(err).Msg("Could not parse base URL")
		return ExtractedURLS{}
	}

	for _, rawURL := range urls {
		// Remove quotation marks from the URL
		cleanedURL := strings.Trim(rawURL, "'\"")

		// Check if the URL starts with "./"
		if strings.HasPrefix(cleanedURL, "./") {
			// Construct the absolute URL by combining the extractedFromURL and cleanedURL
			absoluteURL := fmt.Sprintf("%s/%s", extractedFromURL, strings.TrimLeft(cleanedURL, "./"))
			webURLs = append(webURLs, absoluteURL)
			continue
		}

		// Check if the URL is relative
		if strings.HasPrefix(cleanedURL, "//") || !strings.Contains(cleanedURL, "://") {
			u, err := url.Parse(cleanedURL)
			if err != nil {
				log.Error().Str("url", cleanedURL).Err(err).Msg("ExtractAndAnalyzeURLS could not parse relative URL")
				continue
			}
			// Resolve the relative URL against the base URL
			absoluteURL := base.ResolveReference(u)
			webURLs = append(webURLs, absoluteURL.String())
		} else {
			parsed, err := url.Parse(cleanedURL)
			if err != nil {
				log.Error().Err(err).Msg("Could not parse absolute URL")
				continue
			}
			if parsed.Scheme == "http" || parsed.Scheme == "https" {
				webURLs = append(webURLs, cleanedURL)
			} else {
				nonWebURLs = append(nonWebURLs, cleanedURL)
			}
		}
	}

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
}
