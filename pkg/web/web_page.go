package web

import (
	"net/url"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/web/cookies"

	"github.com/go-rod/rod/lib/proto"
)

type WebPage struct {
	URL             string
	ResponseURL     string
	RemoteIPAddress string
	Port            int
	Headers         map[string]string
	StatusCode      int
	MimeType        string
	Body            string
	Anchors         []string
	Cookies         []cookies.Cookie
	Issues          []db.Issue
	SecurityState   string
	SecurityDetails *proto.NetworkSecurityDetails
	// isError                bool
	// isUserAgentReflected   bool
	// isUserAgentDependant   bool
	// hasForms               bool
	// usesAjax               bool
	// usesWebsockets         bool
	// hasDynamicParams       bool
	// hasReflectedParams     bool
	// usesUnsafeJsSources    bool
	// hasFileUpload          bool
	// outputsConsoleMessages bool
}

// HasParameters checks if a web page has parameters
func (wp *WebPage) HasParameters() (bool, error) {
	parsedURL, err := url.ParseRequestURI(wp.URL)
	if err != nil {
		return false, err
	}
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return false, err
	}

	if len(query) > 0 {
		return true, nil
	}
	return false, nil
}
