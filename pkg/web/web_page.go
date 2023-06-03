package web

import (
	"net/url"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/web/cookies"

	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
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
	Forms           []Form
	Buttons         []Button
	Iframes         []Iframe
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

// LogPageData logs web page data
func (wp *WebPage) LogPageData() {
	log.Info().Int("status_code", wp.StatusCode).Int("forms_count", len(wp.Forms)).Int("anchors_count", len(wp.Anchors)).Str("url", wp.URL).Str("response_url", wp.ResponseURL).Str("mime-type", wp.MimeType).Str("security_state", wp.SecurityState).Msg("Page details")
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
