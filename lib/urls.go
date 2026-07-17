package lib

import (
	"fmt"
	"math/rand"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// GetParametersToTest returns a list of parameters to test based on the provided path and params
func GetParametersToTest(path string, params []string, testAllParams bool) (parametersToTest []string) {
	parametersToTest = append(parametersToTest, params...)

	if !testAllParams && len(params) > 0 {
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

		if Contains(params, key) {
			continue
		} else if testAllParams || len(params) == 0 {
			// If provided by params[], we ignore as already added
			parametersToTest = append(parametersToTest, key)
		}
	}
	return parametersToTest
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// EncodeQueryValuePreservingPct percent-encodes a payload for safe placement in a
// URL query-string value. It encodes every byte that is illegal or ambiguous in a
// query component (space, control chars, <, >, ", ', &, =, ;, +, %, ...) so the
// request line is valid and the target decodes back to the exact intended bytes.
//
// Injection metacharacters that are already legal in a query component
// ({ } $ # * ( ) / | : etc.) are left raw to preserve payload fidelity, and an
// existing valid %XX percent-triplet is passed through unchanged so payloads that
// deliberately ship pre-encoded sequences (e.g. CRLF %0D%0A, null %00) are not
// double-encoded.
func EncodeQueryValuePreservingPct(payload string) string {
	var sb strings.Builder
	for i := 0; i < len(payload); i++ {
		c := payload[i]
		if c == '%' && i+2 < len(payload) && isHexDigit(payload[i+1]) && isHexDigit(payload[i+2]) {
			sb.WriteString(payload[i : i+3])
			i += 2
			continue
		}
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			strings.IndexByte("-._~", c) >= 0 ||
			strings.IndexByte("{}$#*()!|:,/@", c) >= 0 {
			sb.WriteByte(c)
			continue
		}
		fmt.Fprintf(&sb, "%%%02X", c)
	}
	return sb.String()
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

// ExtractOrigin extracts the origin from a URL, converting ws/wss schemes to http/https.
func ExtractOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := u.Scheme
	if scheme == "ws" {
		scheme = "http"
	} else if scheme == "wss" {
		scheme = "https"
	}
	return scheme + "://" + u.Host
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

// URLComponents contains the parsed components of a URL.
type URLComponents struct {
	Host   string
	Port   int
	Path   string
	UseTLS bool
}

// ParseURLComponents extracts host, port, path, and TLS info from a URL in a single parse.
func ParseURLComponents(u string) (URLComponents, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return URLComponents{}, err
	}

	components := URLComponents{
		Host:   parsedURL.Hostname(),
		UseTLS: parsedURL.Scheme == "https",
	}

	// Determine port
	if parsedURL.Port() != "" {
		port, err := strconv.Atoi(parsedURL.Port())
		if err != nil {
			return URLComponents{}, err
		}
		components.Port = port
	} else if components.UseTLS {
		components.Port = 443
	} else {
		components.Port = 80
	}

	// Determine path
	if parsedURL.Path == "" {
		components.Path = "/"
	} else {
		components.Path = parsedURL.Path
	}

	return components, nil
}

// GetHostFromURL extracts the host from the given URL.
func GetHostFromURL(u string) (string, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return parsedURL.Hostname(), nil
}

// GetPortFromURL extracts the port from the given URL.
// Returns the explicit port if specified, or the default port based on scheme (443 for https, 80 for http).
func GetPortFromURL(u string) (int, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return 0, err
	}
	if parsedURL.Port() != "" {
		port, err := strconv.Atoi(parsedURL.Port())
		if err != nil {
			return 0, err
		}
		return port, nil
	}
	if parsedURL.Scheme == "https" {
		return 443, nil
	}
	return 80, nil
}

// GetPathFromURL extracts the path from the given URL.
// Returns "/" if no path is specified.
func GetPathFromURL(u string) (string, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	if parsedURL.Path == "" {
		return "/", nil
	}
	return parsedURL.Path, nil
}

// IsTLS returns true if the URL uses HTTPS scheme.
func IsTLS(u string) bool {
	return strings.HasPrefix(strings.ToLower(u), "https://")
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
	return NormalizeURLPathWithProvidedString(urlStr, "X")
}

func NormalizeURLPathWithProvidedString(urlStr string, providedString string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	segments := strings.Split(u.Path, "/")
	if len(segments) > 1 {
		last := segments[len(segments)-1]
		if last != "" && IsLikelyDynamicPathSegment(last) {
			segments[len(segments)-1] = providedString
		}
	}
	u.Path = strings.Join(segments, "/")

	return u.String(), nil
}

// IsLikelyDynamicPathSegment reports whether a path segment looks like a variable
// identifier (numeric id, UUID, or a long hash) rather than a fixed route name.
// It is used to decide whether two URLs sharing the same path prefix are the same
// logical endpoint for scan-scheduling dedup. Fixed route words such as "render",
// "echo" or "safe-render" are treated as distinct endpoints and must not be merged.
func IsLikelyDynamicPathSegment(segment string) bool {
	if segment == "" {
		return false
	}

	if _, err := strconv.Atoi(segment); err == nil {
		return true
	}

	if uuidPattern.MatchString(segment) {
		return true
	}

	// Long all-hex segments (>= 16 chars) are almost always hashes/opaque ids.
	// Short hex-looking words (e.g. "cafe", "beef") are left as fixed route names.
	if len(segment) >= 16 && isHexString(segment) {
		return true
	}

	// A segment mixing letters, digits and separators where digits are present
	// (e.g. "v1.2.3", "2024-01-30", "a1b2c3d4e5f6") is treated as dynamic.
	if len(segment) >= 8 && containsDigit(segment) && isHexOrSeparators(segment) {
		return true
	}

	return false
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

func isHexOrSeparators(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isHexDigit(c) && c != '.' && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func containsDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
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

// JoinURLPath joins a base URL and a URL path.
func JoinURLPath(baseURL, urlPath string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/" + strings.TrimPrefix(urlPath, "/")
	}
	u.Path = path.Join(u.Path, urlPath)
	return u.String()
}

func ResolveURL(baseURL, relativeURL string) (string, error) {
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

// IsRelativeURL Function to check if URL is relative
func IsRelativeURL(url string) bool {
	return strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") || (!strings.HasPrefix(url, "/") && !strings.Contains(url, "://") && !strings.HasPrefix(url, "mailto:"))
}

// IsAbsoluteURL Function to check if URL is absolute
func IsAbsoluteURL(url string) bool {
	return strings.Contains(url, "://")
}
