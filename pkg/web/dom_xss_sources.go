package web

import (
	"fmt"
	"net/url"
	"strings"
)

// DOMXSSSourceType categorizes DOM XSS sources by their nature
type DOMXSSSourceType int

const (
	SourceTypeURL DOMXSSSourceType = iota
	SourceTypeDocument
	SourceTypeWindow
	SourceTypeStorage
	SourceTypeMessage // postMessage and cross-origin messaging
	SourceTypeHistory // History API (pushState, replaceState, state)
)

// DOMXSSSource represents a potential DOM XSS source
type DOMXSSSource struct {
	Name        string           // JavaScript property name (e.g., "location.hash")
	Type        DOMXSSSourceType // Category of the source
	Priority    int              // Higher = test first (most common vectors)
	Description string           // Human-readable description
}

// DOMXSSSources returns all supported DOM XSS sources ordered by priority
func DOMXSSSources() []DOMXSSSource {
	return []DOMXSSSource{
		// URL-based sources (most common attack vector)
		{Name: "location.hash", Type: SourceTypeURL, Priority: 100, Description: "URL fragment identifier - never sent to server"},
		{Name: "location.search", Type: SourceTypeURL, Priority: 95, Description: "URL query string"},
		{Name: "location.href", Type: SourceTypeURL, Priority: 90, Description: "Full URL"},
		{Name: "location.pathname", Type: SourceTypeURL, Priority: 85, Description: "URL path component"},
		{Name: "document.URL", Type: SourceTypeURL, Priority: 80, Description: "Full document URL"},
		{Name: "document.documentURI", Type: SourceTypeURL, Priority: 75, Description: "Document URI"},
		{Name: "document.baseURI", Type: SourceTypeURL, Priority: 70, Description: "Base URI for relative URLs"},

		// Document sources
		{Name: "document.referrer", Type: SourceTypeDocument, Priority: 60, Description: "Referrer URL"},
		{Name: "document.cookie", Type: SourceTypeDocument, Priority: 55, Description: "Document cookies"},

		// Window sources
		{Name: "window.name", Type: SourceTypeWindow, Priority: 50, Description: "Window name - persists across navigations"},

		// Storage sources
		{Name: "localStorage", Type: SourceTypeStorage, Priority: 45, Description: "Persistent local storage"},
		{Name: "sessionStorage", Type: SourceTypeStorage, Priority: 40, Description: "Session storage"},

		// Message sources (cross-origin communication)
		{Name: "postMessage", Type: SourceTypeMessage, Priority: 65, Description: "Cross-origin message event data"},

		// History API sources
		{Name: "history.state", Type: SourceTypeHistory, Priority: 52, Description: "Browser history state object"},
	}
}

// GetURLBasedSources returns only URL-based sources
func GetURLBasedSources() []DOMXSSSource {
	var sources []DOMXSSSource
	for _, s := range DOMXSSSources() {
		if s.Type == SourceTypeURL {
			sources = append(sources, s)
		}
	}
	return sources
}

// GetStorageSources returns only storage-based sources
func GetStorageSources() []DOMXSSSource {
	var sources []DOMXSSSource
	for _, s := range DOMXSSSources() {
		if s.Type == SourceTypeStorage {
			sources = append(sources, s)
		}
	}
	return sources
}

// InjectPayloadIntoURL injects a payload into the specified URL source
func InjectPayloadIntoURL(baseURL string, source DOMXSSSource, payload string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	switch source.Name {
	case "location.hash":
		// Append payload to fragment
		parsedURL.Fragment = payload
		return parsedURL.String(), nil

	case "location.search":
		// Add payload as query parameter using common parameter names
		q := parsedURL.Query()
		q.Set("query", payload) // Hardcoded common param name that apps read
		parsedURL.RawQuery = q.Encode()
		return parsedURL.String(), nil

	case "location.href", "document.URL", "document.documentURI":
		// For these, we test via hash (most reliable) or query
		parsedURL.Fragment = payload
		return parsedURL.String(), nil

	case "location.pathname":
		// Append payload to path (URL encoded)
		if !strings.HasSuffix(parsedURL.Path, "/") {
			parsedURL.Path += "/"
		}
		parsedURL.Path += url.PathEscape(payload)
		return parsedURL.String(), nil

	case "document.baseURI":
		// Test via hash as baseURI typically reflects the page URL
		parsedURL.Fragment = payload
		return parsedURL.String(), nil

	default:
		return "", fmt.Errorf("unsupported URL source: %s", source.Name)
	}
}

// GetBrowserSetupScript returns JavaScript to set up the source before navigation
func GetBrowserSetupScript(source DOMXSSSource, payload string, storageKey string) string {
	escapedPayload := EscapeJSString(payload)
	escapedKey := EscapeJSString(storageKey)

	switch source.Name {
	case "localStorage":
		return fmt.Sprintf(`localStorage.setItem('%s', '%s');`, escapedKey, escapedPayload)
	case "sessionStorage":
		return fmt.Sprintf(`sessionStorage.setItem('%s', '%s');`, escapedKey, escapedPayload)
	case "window.name":
		return fmt.Sprintf(`window.name = '%s';`, escapedPayload)
	default:
		return ""
	}
}

// String returns a string representation of the source type
func (t DOMXSSSourceType) String() string {
	switch t {
	case SourceTypeURL:
		return "URL"
	case SourceTypeDocument:
		return "Document"
	case SourceTypeWindow:
		return "Window"
	case SourceTypeStorage:
		return "Storage"
	case SourceTypeMessage:
		return "Message"
	case SourceTypeHistory:
		return "History"
	default:
		return "Unknown"
	}
}

// GetMessageSources returns only message-based sources (postMessage)
func GetMessageSources() []DOMXSSSource {
	var sources []DOMXSSSource
	for _, s := range DOMXSSSources() {
		if s.Type == SourceTypeMessage {
			sources = append(sources, s)
		}
	}
	return sources
}

// GetHistorySources returns only history API sources
func GetHistorySources() []DOMXSSSource {
	var sources []DOMXSSSource
	for _, s := range DOMXSSSources() {
		if s.Type == SourceTypeHistory {
			sources = append(sources, s)
		}
	}
	return sources
}
