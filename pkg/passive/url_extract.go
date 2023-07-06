package passive

import (
	"github.com/rs/zerolog/log"
	"mvdan.cc/xurls/v2"
	"net/url"
	"path"
	"strings"
)

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
		if strings.Contains(rawURL, "loader.js") {
			log.Info().Str("raw_url", rawURL).Msg("Resolving URL")

		}
		if isRelative(rawURL) {
			// Resolve the relative URL against the directory
			absoluteURL, err := resolveRelative(rawURL, base)
			if err != nil {
				log.Error().Err(err).Msg("Could not resolve relative URL")
				continue
			}
			if strings.Contains(rawURL, "loader.js") {
				log.Info().Str("raw_url", rawURL).Str("absolute_url", absoluteURL).Msg("Resolved relative URL")

			}
			webURLs = append(webURLs, absoluteURL)
		} else if strings.HasPrefix(rawURL, "//") {
			// Check if the URL is protocol-relative
			absoluteURL := base.Scheme + ":" + rawURL
			webURLs = append(webURLs, absoluteURL)
		} else if strings.HasPrefix(rawURL, "/") {
			// Check if the URL is absolute to the domain
			absoluteURL := base.Scheme + "://" + base.Host + rawURL
			webURLs = append(webURLs, absoluteURL)
		} else if u, err := url.Parse(rawURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			// Check if the URL is an absolute HTTP or HTTPS URL
			webURLs = append(webURLs, rawURL)
		} else if strings.Contains(rawURL, "://") {
			// Check if the URL is an absolute non-HTTP/non-HTTPS URL
			nonWebURLs = append(nonWebURLs, rawURL)
		} else {
			log.Info().Str("raw_url", rawURL).Msg("Could not determine URL type")
		}
	}

	// if len(webURLs) > 0 {
	// 	log.Debug().Str("url", extractedFromURL).Int("web_urls", len(webURLs)).Msg("Found web URLs")
	// }

	return ExtractedURLS{Web: webURLs, NonWeb: nonWebURLs}
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
