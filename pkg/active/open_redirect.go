package active

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

const openRedirecTestDomain = "sukyan.com"

var metaRefreshPattern = regexp.MustCompile(`(?i)<meta[^>]*http-equiv\s*=\s*["']?refresh["']?[^>]*content\s*=\s*["']?\d+\s*;\s*url\s*=\s*["']?([^"'\s>]+)`)

var schemeWithoutAuthority = regexp.MustCompile(`(?i)^(https?):/?([^/].*)$`)

// browserNormalizedLocation rewrites a redirect target the way a browser's URL parser
// would before it resolves it. net/url follows RFC 3986, browsers follow WHATWG, and
// they disagree on exactly the shapes attackers use: `\` is a path separator for
// http(s), leading slash runs collapse, and `https:/host` still names an authority.
// Without this, `/\evil.com` resolves to the *request's own* host under RFC 3986 and
// the redirect looks same-origin when a browser would leave the site.
func browserNormalizedLocation(location string) string {
	v := strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\r':
			return -1
		case '\\':
			return '/'
		}
		return r
	}, strings.TrimSpace(location))

	if strings.HasPrefix(v, "//") {
		v = "//" + strings.TrimLeft(v, "/")
	}
	if m := schemeWithoutAuthority.FindStringSubmatch(v); m != nil {
		v = m[1] + "://" + m[2]
	}
	return v
}

// redirectsOffOrigin reports whether location, resolved against requestURL, points to a
// different host. It handles absolute, protocol-relative and relative locations uniformly
// via url.ResolveReference, so a same-origin relative redirect that merely contains the test
// domain as a substring (e.g. in a path segment) is not mistaken for an actual open redirect.
func redirectsOffOrigin(requestURL, location string) bool {
	if location == "" {
		return false
	}
	base, err := url.Parse(requestURL)
	if err != nil {
		return false
	}
	loc, err := url.Parse(browserNormalizedLocation(location))
	if err != nil {
		return false
	}
	resolved := base.ResolveReference(loc)
	if resolved.Hostname() == "" {
		return false
	}
	return !strings.EqualFold(resolved.Hostname(), base.Hostname())
}

func OpenRedirectScan(history *db.History, options ActiveModuleOptions, insertionPoints []scan.InsertionPoint) (bool, error) {
	auditLog := log.With().Str("audit", "open-redirect").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		auditLog.Info().Msg("Open redirect scan cancelled before starting")
		return false, ctx.Err()
	default:
	}

	payloads := []string{
		"https://" + openRedirecTestDomain,
		"//" + openRedirecTestDomain,
		"https%3A%2F%2F" + openRedirecTestDomain,
		"//%5c" + openRedirecTestDomain,
	}

	scanInsertionPoints := []scan.InsertionPoint{}
	switch options.ScanMode {

	case scan_options.ScanModeFuzz:
		scanInsertionPoints = insertionPoints

	default:
		headers, err := history.GetResponseHeadersAsMap()
		if err != nil {
			auditLog.Error().Err(err).Msg("Failed to get response headers")
			return false, err
		}
		locations := headers["Location"]
		for _, insertionPoint := range insertionPoints {
			if lib.SliceContains(locations, insertionPoint.Value) || lib.SliceContains(locations, insertionPoint.OriginalData) || scan.IsCommonOpenRedirectParameter(insertionPoint.Name) || insertionPoint.ValueType == lib.TypeURL {
				auditLog.Info().Str("insertionPoint", insertionPoint.Value).Msg("Found an interesting insertion point to test for open redirect")
				scanInsertionPoints = append(scanInsertionPoints, insertionPoint)
			}
		}

	}

	if len(scanInsertionPoints) == 0 {
		auditLog.Info().Msg("No interesting insertion points to test for open redirect")
		return false, nil
	}
	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	for _, insertionPoint := range scanInsertionPoints {
		for _, payload := range payloads {
			select {
			case <-ctx.Done():
				auditLog.Info().Msg("Open redirect scan cancelled during testing")
				return false, ctx.Err()
			default:
			}

			auditLog.Info().Str("insertionPoint", insertionPoint.Value).Str("payload", payload).Msg("Testing insertion point for open redirect")
			builders := []scan.InsertionPointBuilder{
				{
					Point:   insertionPoint,
					Payload: payload,
				},
			}
			req, err := scan.CreateRequestFromInsertionPoints(history, builders)
			if err != nil {
				auditLog.Error().Err(err).Msg("Failed to create request from insertion points")
				continue
			}

			req = req.WithContext(ctx)

			executionResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
				Client:        client,
				CreateHistory: true,
				HistoryCreationOptions: http_utils.HistoryCreationOptions{
					Source:              db.SourceScanner,
					WorkspaceID:         options.WorkspaceID,
					TaskID:              options.TaskID,
					TaskJobID:           options.TaskJobID,
					ScanID:              options.ScanID,
					ScanJobID:           options.ScanJobID,
					CreateNewBodyStream: true,
				},
			})
			if executionResult.Err != nil {
				auditLog.Error().Err(executionResult.Err).Msg("Failed to send request")
				continue
			}

			new := executionResult.History

			if new.StatusCode >= 300 && new.StatusCode < 400 {
				newLocation := executionResult.Response.Header.Get("Location")
				// newLocation == payload is checked without a host comparison: every payload we
				// send is itself absolute or protocol-relative, so an exact, byte-for-byte echo
				// of it already proves the response points off-origin.
				if newLocation != "" && (newLocation == payload || redirectsOffOrigin(new.URL, newLocation)) {
					auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("payload", payload).Msg("Open redirect found via Location header")

					details := fmt.Sprintf("Using the payload %s in the insertion point %s, the server redirected the request to %s.", payload, insertionPoint.String(), newLocation)
					db.CreateIssueFromHistoryAndTemplate(new, db.OpenRedirectCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)

					return true, nil
				}
			}

			responseBody, err := new.ResponseBody()
			if err == nil && len(responseBody) > 0 {
				if matches := metaRefreshPattern.FindSubmatch(responseBody); len(matches) > 1 {
					redirectURL := string(matches[1])
					if redirectsOffOrigin(new.URL, redirectURL) {
						auditLog.Info().Str("insertionPoint", insertionPoint.String()).Str("payload", payload).Str("redirectURL", redirectURL).Msg("Open redirect found via meta refresh")

						details := fmt.Sprintf("Using the payload %s in the insertion point %s, the server returned a meta refresh tag redirecting to %s.", payload, insertionPoint.String(), redirectURL)
						db.CreateIssueFromHistoryAndTemplate(new, db.OpenRedirectCode, details, 90, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)

						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}
