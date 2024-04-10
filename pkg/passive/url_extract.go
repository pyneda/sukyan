package passive

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"mvdan.cc/xurls/v2"
)

type ExtractedURLS struct {
	Web    []string
	NonWeb []string
}

func ExtractURLsFromHistoryItem(history *db.History) ExtractedURLS {
	responseLinks := ExtractAndAnalyzeURLS(string(history.ResponseBody), history.URL)
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
				if urlType == "web" {
					webURLs = append(webURLs, absoluteURL)
				} else if urlType == "non-web" {
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
		if urlType == "web" {
			webURLs = append(webURLs, absoluteURL)
		} else if urlType == "non-web" {
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
	merged := make([]string, 0, len(arr1)+len(arr2))
	seen := make(map[string]bool)

	for _, s := range arr1 {
		rawURL := strings.Trim(s, "'\"")
		if strings.HasPrefix(rawURL, "tel:") {
			continue
		}
		if !seen[rawURL] {
			merged = append(merged, rawURL)
			seen[rawURL] = true
		}
	}

	for _, s := range arr2 {
		rawURL := strings.Trim(s, "'\"")
		if strings.HasPrefix(rawURL, "tel:") {
			continue
		}
		if !seen[rawURL] {
			merged = append(merged, rawURL)
			seen[rawURL] = true
		}
	}

	return merged
}

func analyzeURL(rawURL string, base *url.URL) (string, string, error) {
	if isRelative(rawURL) {
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

func resolveURL(baseURL, relativeURL string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	rel, err := url.Parse(relativeURL)
	if err != nil {
		return "", err
	}

	resolvedURL := base.ResolveReference(rel)
	return resolvedURL.String(), nil
}

// Function to check if URL is relative
func isRelative(url string) bool {
	return strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") || (!strings.HasPrefix(url, "/") && !strings.Contains(url, "://") && !strings.HasPrefix(url, "mailto:"))
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

// Function to get directory path from a URL
func getDirectoryPath(extractedFromURL string) (string, error) {
	dir, _ := path.Split(extractedFromURL)
	if strings.HasSuffix(extractedFromURL, "/") {
		dir = extractedFromURL
	} else if path.Ext(extractedFromURL) != "" {
		dir, _ = path.Split(extractedFromURL)
	} else if strings.Contains(path.Base(extractedFromURL), ".") {
		dir, _ = path.Split(extractedFromURL)
	} else {
		dir = extractedFromURL
	}
	dir = strings.TrimSuffix(dir, "/")
	return dir, nil
}

// Function to check if URL is absolute
func isAbsolute(url string) bool {
	return strings.Contains(url, "://")
}
