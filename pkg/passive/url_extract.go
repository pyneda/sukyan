package passive

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"mvdan.cc/xurls/v2"
)

type ExtractedURLS struct {
	Web    []string
	NonWeb []string
}

const maxInt = int(^uint(0) >> 1)

func ExtractURLsFromHistoryItem(history *db.History) ExtractedURLS {
	body, err := history.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Uint("history_id", history.ID).Msg("Failed to get response body")

	}
	responseLinks := ExtractAndAnalyzeURLS(string(body), history.URL)
	headers, err := history.GetResponseHeadersAsMap()
	if err != nil {
		return responseLinks
	}
	headersLinks := ExtractURLsFromHeaders(headers, history.URL)
	return mergeExtractedURLs(responseLinks, headersLinks)
}

func mergeExtractedURLs(a, b ExtractedURLS) ExtractedURLS {
	mergedWebURLs := mergeURLs(a.Web, b.Web)
	mergedNonWebURLs := mergeURLs(a.NonWeb, b.NonWeb)

	return ExtractedURLS{
		Web:    mergedWebURLs,
		NonWeb: mergedNonWebURLs,
	}
}

func ExtractURLsFromHeaders(headers map[string][]string, extractedFromURL string) ExtractedURLS {
	webURLs := make([]string, 0)
	nonWebURLs := make([]string, 0)
	base, err := url.Parse(extractedFromURL)
	if err != nil {
		log.Error().Err(err).Msg("Could not parse base URL")
		return ExtractedURLS{}
	}
	for _, header := range headers {
		for _, headerValue := range header {
			for _, rawURL := range ExtractURLs(fmt.Sprintf("'%v'", headerValue)) {
				absoluteURL, urlType, err := analyzeURL(rawURL, base)
				if err != nil {
					log.Error().Err(err).Str("part", "headers").Str("url", rawURL).Msg("Could not analyze URL")
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
	}

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
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
		absoluteURL, urlType, err := analyzeURL(rawURL, base)
		if err != nil {
			log.Error().Err(err).Str("part", "response").Str("url", rawURL).Msg("Could not analyze URL")
			continue
		}
		switch urlType {
		case "web":
			webURLs = append(webURLs, absoluteURL)
		case "non-web":
			nonWebURLs = append(nonWebURLs, absoluteURL)
		}
	}

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
}

func ExtractURLs(response string) []string {
	quoted := extractQuotedURLs(response)
	generic := extractURLsGeneric(response)
	return mergeURLs(quoted, generic)
}

func extractQuotedURLs(response string) []string {
	return urlRegex.FindAllString(response, -1)
}

func extractURLsGeneric(response string) []string {
	rx := xurls.Strict()
	return rx.FindAllString(response, -1)
}

func mergeURLs(arr1, arr2 []string) []string {
	var totalLength int
	if len(arr1) > maxInt-len(arr2) {
		log.Warn().Msg("Potential integer overflow detected when merging URL lists. Limiting capacity.")
		totalLength = maxInt
	} else {
		totalLength = len(arr1) + len(arr2)
	}
	merged := make([]string, 0, totalLength)
	seen := make(map[string]bool)

	for _, array := range [][]string{arr1, arr2} {
		for _, s := range array {
			rawURL := strings.Trim(s, "'\"")
			if strings.HasPrefix(rawURL, "tel:") || seen[rawURL] {
				continue
			}
			if len(merged) >= totalLength {
				log.Warn().Msg("Reached maximum safe capacity of URL list.")
				break
			}
			if !seen[rawURL] {
				merged = append(merged, rawURL)
				seen[rawURL] = true
			}
		}
	}
	return merged
}

func analyzeURL(rawURL string, base *url.URL) (string, string, error) {
	if lib.IsRelativeURL(rawURL) {
		absoluteURL, err := resolveRelative(rawURL, base)
		if err != nil {
			return "", "", err
		}
		return absoluteURL, "web", nil
	} else if strings.HasPrefix(rawURL, "//") {
		absoluteURL := base.Scheme + ":" + rawURL
		return absoluteURL, "web", nil
	} else if strings.HasPrefix(rawURL, "/") {
		absoluteURL := base.Scheme + "://" + base.Host + rawURL
		return absoluteURL, "web", nil
	} else if u, err := url.Parse(rawURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return rawURL, "web", nil
	} else if strings.Contains(rawURL, "://") {
		return rawURL, "non-web", nil
	} else if strings.HasPrefix(rawURL, "mailto:") {
		return rawURL, "non-web", nil
	} else {
		return "", "", fmt.Errorf("could not determine URL type")
	}
}

// Function to resolve relative URLs
func resolveRelative(rawURL string, base *url.URL) (string, error) {
	if strings.HasPrefix(rawURL, "/") {
		return base.Scheme + "://" + base.Host + rawURL, nil
	}

	if strings.HasPrefix(rawURL, "../") {
		pathParts := strings.Split(base.Path, "/")
		pathParts = pathParts[:len(pathParts)-1]
		newPath := strings.Join(pathParts, "/")
		return base.Scheme + "://" + base.Host + newPath + rawURL[2:], nil
	}

	if strings.HasPrefix(rawURL, "./") {
		relativePath := rawURL[2:]
		if path.Ext(base.Path) != "" {
			return base.Scheme + "://" + base.Host + path.Join(path.Dir(base.Path), relativePath), nil
		} else {
			return base.Scheme + "://" + base.Host + path.Join(base.Path, relativePath), nil
		}
	} else {
		// If the base path has an extension, it's a file and we need to get the directory part
		dir := base.Path
		if path.Ext(base.Path) != "" {
			dir = path.Dir(base.Path)
		}
		// Ensure that dir ends with a slash
		if !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		return base.Scheme + "://" + base.Host + dir + rawURL, nil
	}
}
