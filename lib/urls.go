package lib

import (
	"fmt"
	"math/rand"
	"net/url"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
)

// GetParametersToTest returns a list of parameters to test based on the provided path and params
func GetParametersToTest(path string, params []string, testAllParams bool) (parametersToTest []string) {
	parametersToTest = append(parametersToTest, params...)

	if testAllParams == false && len(params) > 0 {
		return parametersToTest
	}
	parsedURL, err := url.ParseRequestURI(path)
	if err != nil {
		log.Error().Err(err).Str("url", path).Msg("Invalid url")
	}
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Warn().Str("url", path).Msg("Could not parse url query")
	}
	for key := range query {

		if Contains(params, key) == true {
			continue
		} else if testAllParams || len(params) == 0 {
			// If provided by params[], we ignore as already added
			parametersToTest = append(parametersToTest, key)
		}
	}
	return parametersToTest
}

// BuildURLWithParam builds a URL with the provided parameter and payload
func BuildURLWithParam(original string, param string, payload string, urlEncode bool) (string, error) {
	parsedURL, err := url.ParseRequestURI(original)
	if err != nil {
		return "", err
	}
	values, _ := url.ParseQuery(parsedURL.RawQuery)
	if urlEncode {
		values.Set(param, payload)
	} else {
		values.Del(param)
	}

	parsedURL.RawQuery = values.Encode()
	testurl := parsedURL.String()
	if !urlEncode {
		if len(values) == 0 {
			testurl = parsedURL.String() + "?" + param + "=" + payload
		} else {
			testurl = parsedURL.String() + "&" + param + "=" + payload
		}
	}
	return testurl, nil
}

// Build404URL Adds a randomly generated path to the URL to fingerprint 404 errors
func Build404URL(original string) (string, error) {
	u, err := url.Parse(original)
	if err != nil {
		return "", err
	}
	length := rand.Intn(50)
	length = length + 10
	u.Path = path.Join(u.Path, GenerateRandomString(length))
	u.Path = path.Join(u.Path, GenerateRandomString(length))
	result := u.String()
	return result, err
}

// GetURLWithoutQueryString returns the base URL from the given URL by removing the query string
func GetURLWithoutQueryString(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	parsedURL.RawQuery = ""
	return parsedURL.String(), nil
}

func IsRootURL(urlStr string) (bool, error) {
	parsedURL, err := url.Parse(urlStr)
	parsedURL.RawQuery = ""
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return false, fmt.Errorf("invalid URL")
	}

	isRoot := strings.Trim(parsedURL.Path, "/") == "" && parsedURL.RawQuery == ""

	return isRoot, nil
}

// GetParentURL returns the parent URL for the given URL. If the given URL
// is already a parent URL, the function returns true as the second return value.
func GetParentURL(urlStr string) (string, bool, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", false, err
	}

	parentURL := parsedURL
	parentURL.Path = path.Dir(parsedURL.Path)

	isParentURL := parentURL.Path == "." || parentURL.Path == "/"

	return parentURL.String(), isParentURL, nil
}

// CalculateURLDepth calculates the depth of a URL.
// Returns -1 if the URL is invalid.
func CalculateURLDepth(rawURL string) int {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return -1
	}
	path := parsedURL.Path
	if path == "" || path == "/" {
		return 0
	}
	segments := strings.Split(path, "/")
	depth := 0
	for _, segment := range segments {
		if segment != "" {
			depth++
		}
	}
	return depth
}

// GetBaseURL extracts the base URL from a URL string.
func GetBaseURL(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	baseURL := u.Scheme + "://" + u.Host

	return baseURL, nil
}

// GetUniqueBaseURLs parses a list of URLs and returns a slice of unique base URLs.
func GetUniqueBaseURLs(urls []string) ([]string, error) {
	baseURLs := make([]string, len(urls))
	for i, rawurl := range urls {
		baseURL, err := GetBaseURL(rawurl)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL %q: %w", rawurl, err)
		}
		baseURLs[i] = baseURL
	}

	return GetUniqueItems(baseURLs), nil
}

func GetLastPathSegment(rawurl string) (string, error) {
	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		return "", err
	}
	pathSegments := strings.Split(parsedURL.Path, "/")
	for i := len(pathSegments) - 1; i >= 0; i-- {
		if pathSegments[i] != "" {
			return pathSegments[i], nil
		}
	}
	return "", nil
}

// GetHostFromURL extracts the host from the given URL.
func GetHostFromURL(u string) (string, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return parsedURL.Hostname(), nil
}

// NormalizeURLParams normalizes the URL parameters by appending an "X" to each value.
func NormalizeURLParams(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	queryParams := u.Query()

	for key, values := range queryParams {
		for i := range values {
			values[i] = "X"
		}
		queryParams[key] = values
	}

	u.RawQuery = queryParams.Encode()

	return u.String(), nil
}

// NormalizeURLPath normalizes the URL path by replacing the last segment with "X".
func NormalizeURLPath(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	segments := strings.Split(u.Path, "/")
	if len(segments) > 1 {
		segments[len(segments)-1] = "X"
	}
	u.Path = strings.Join(segments, "/")

	return u.String(), nil
}

// NormalizeURL normalizes the URL by adding an "X" to the last path segment and replacing the query parameter values with "X".
func NormalizeURL(urlStr string) (string, error) {
	normalizedPathURL, err := NormalizeURLPath(urlStr)
	if err != nil {
		return "", err
	}
	normalizedFullURL, err := NormalizeURLParams(normalizedPathURL)
	if err != nil {
		return "", err
	}
	return normalizedFullURL, nil
}
