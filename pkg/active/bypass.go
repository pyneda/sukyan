package active

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type HeaderTest struct {
	HeaderName string
	Values     []string
}

var bypassIPs = []string{
	"127.0.0.1", // Standard loopback
	"localhost", // Localhost domain
	// "0.0.0.0",            // Non-routable meta-address
	"::1", // IPv6 loopback
	// "0000:0000:0000:0000:0000:0000:0000:0001",  // Full IPv6 loopback
	// "0:0:0:0:0:0:0:1",    // Shortened IPv6 loopback
	"127.0.1.1", // Alternative loopback in some systems
	// "10.0.0.0",           // Private IP address range (start)
	// "172.16.0.0",         // Another private IP address range (start)
	// "192.168.0.0",        // Another private IP address range (start)
	"0x7F000001", // Hex representation of 127.0.0.1
	"2130706433", // Decimal representation of 127.0.0.1
	"127.1",      // Short form of 127.0.0.1
}

var ipBasedHeaders = []HeaderTest{
	{"X-Original-URL", bypassIPs},
	{"X-Custom-IP-Authorization", bypassIPs},
	{"X-Forwarded-For", bypassIPs},
	{"X-Originally-Forwarded-For", bypassIPs},
	{"X-Originating-", bypassIPs},
	{"X-Originating-IP", bypassIPs},
	{"True-Client-IP", bypassIPs},
	{"X-WAP-Profile", bypassIPs},
	{"X-Arbitrary", bypassIPs},
	{"X-HTTP-DestinationURL", bypassIPs},
	{"X-Forwarded-Proto", bypassIPs},
	{"Destination", bypassIPs},
	{"X-Remote-IP", bypassIPs},
	{"X-Client-IP", bypassIPs},
	{"X-Host", bypassIPs},
	{"X-Forwarded-Host", bypassIPs},
	{"X-ProxyUser-Ip", bypassIPs},
	{"X-Real-IP", bypassIPs},
}

var bypassURLs = []string{
	"http://127.0.0.1",
	"https://127.0.0.1",
	"http://localhost",
	"https://localhost",
	"http://127.0.0.1:80",
	"https://127.0.0.1:443",
}

var urlBasedHeaders = []HeaderTest{
	{"X-Original-URL", bypassURLs},
	{"X-Forwarded-For", bypassURLs},
	{"X-Originating-", bypassURLs},
	{"X-Arbitrary", bypassURLs},
	{"X-HTTP-DestinationURL", bypassURLs},
	{"X-Forwarded-Proto", bypassURLs},
	{"X-Host", bypassURLs},
	{"X-Forwarded-Host", bypassURLs},
}

var bypassPorts = []string{"80", "443", "8000", "8080", "8443", "8888", "10443"}

var portBasedHeaders = []HeaderTest{
	{"X-Forwarded-Port", bypassPorts},
	{"X-Original-Port", bypassPorts},
	{"X-Real-Port", bypassPorts},
}
var bypassPaths = []string{
	"/",
	"/admin",
}

var pathBasedHeaders = []HeaderTest{
	{"X-Rewrite-URL", bypassPaths},
	{"X-Real-URL", bypassPaths},
}

func ForbiddenBypassScan(history *db.History, options ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "bypass").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	// Check context cancellation
	if options.Ctx != nil {
		select {
		case <-options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping bypass scan")
			return
		default:
		}
	}

	if history.StatusCode != 401 && history.StatusCode != 403 {
		auditLog.Warn().Msg("Skipping auth bypass scan because the status code is not 401 or 403")
		return
	}
	if options.Concurrency == 0 {
		options.Concurrency = 5
	}
	client := options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Get context, defaulting to background if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Create pool with context for cancellation support
	p := pool.New().WithMaxGoroutines(options.Concurrency)

	allHeaderTypes := [][]HeaderTest{ipBasedHeaders, urlBasedHeaders, portBasedHeaders, pathBasedHeaders}
	// header bypass checks
	for _, headers := range allHeaderTypes {
		// Check context before processing each header type
		select {
		case <-ctx.Done():
			auditLog.Info().Msg("Bypass scan cancelled")
			p.Wait()
			return
		default:
		}

		valueCombinations := flattenHeaders(headers)

		for _, combination := range valueCombinations {
			comb := combination
			p.Go(func() {
				// Check context before making request
				select {
				case <-ctx.Done():
					return
				default:
				}

				request, err := http_utils.BuildRequestFromHistoryItem(history)
				if err != nil {
					auditLog.Error().Err(err).Msg("Error creating the request")
					return
				}
				for header, value := range comb {
					request.Header.Set(header, value)
				}
				// Use context for HTTP request
				request = request.WithContext(ctx)
				sendRequestAndCheckBypass(client, request, history, options, auditLog)
			})
		}
	}

	// Check context before URL bypass checks
	select {
	case <-ctx.Done():
		auditLog.Info().Msg("Bypass scan cancelled before URL bypass checks")
		p.Wait()
		return
	default:
	}

	// url bypass checks
	bypassURLs, err := generateBypassURLs(history)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error generating bypass URLs")
		return
	}

	for _, bypassURL := range bypassURLs {
		bURL := bypassURL
		p.Go(func() {
			// Check context before making request
			select {
			case <-ctx.Done():
				return
			default:
			}

			request, err := http_utils.BuildRequestFromHistoryItem(history)
			if err != nil {
				auditLog.Error().Err(err).Msgf("Error creating request for bypass URL: %s", bURL)
				return
			}
			parsed, err := url.Parse(bURL)
			if err != nil {
				auditLog.Error().Err(err).Msgf("Error parsing bypass URL: %s", bURL)
				return
			}
			request.URL = parsed
			// Use context for HTTP request
			request = request.WithContext(ctx)
			sendRequestAndCheckBypass(client, request, history, options, auditLog)
		})
	}
	p.Wait()
	auditLog.Info().Msg("Finished auth bypass scan")
}

// Flatten headers into a slice of individual header-value pairs
func flattenHeaders(headerTests []HeaderTest) []map[string]string {
	var flat []map[string]string
	for _, ht := range headerTests {
		for _, val := range ht.Values {
			flat = append(flat, map[string]string{ht.HeaderName: val})
		}
	}
	return flat
}

// Get the list of bypass URLs based on provided payloads.
func generateBypassURLs(history *db.History) ([]string, error) {
	originalURL, err := url.Parse(history.URL)
	if err != nil {
		return nil, err
	}
	urlPath := originalURL.Path

	if urlPath == "" {
		return nil, nil
	}

	segments := strings.Split(urlPath, "/")
	if len(segments) < 2 {
		return nil, nil
	}
	lastSegment := segments[len(segments)-1]
	basePath := strings.Join(segments[:len(segments)-1], "/")

	var pathPayloads = []string{
		"/%2e/" + lastSegment,
		lastSegment + "/./",
		"/." + lastSegment + "/./",
		lastSegment + "%20/",
		"/%20" + lastSegment + "%20/",
		lastSegment + "%09/",
		"/%09" + lastSegment + "%09/",
		lastSegment + "..;/",
		lastSegment + "?",
		lastSegment + "??",
		"/" + lastSegment + "//",
		lastSegment + "/",
		strings.ToUpper(lastSegment),
		lastSegment + "/.",
		"//" + lastSegment + "//",
		"/./" + lastSegment + "/..",
		";/" + lastSegment,
		".;/" + lastSegment,
		"//;//" + lastSegment,
	}

	var bypassURLs []string
	for _, payload := range pathPayloads {
		newURL := *originalURL
		newPath := basePath + payload
		newURL.Path = newPath
		bypassURLs = append(bypassURLs, newURL.String())
	}

	return bypassURLs, nil
}

func sendRequestAndCheckBypass(client *http.Client, request *http.Request, original *db.History, options ActiveModuleOptions, auditLog zerolog.Logger) {
	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
			CreateNewBodyStream: false,
		},
	})

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Error during request")
		return
	}

	history := executionResult.History
	if history.StatusCode != 400 && history.StatusCode != 401 && history.StatusCode != 403 && history.StatusCode != 404 {
		bypassHeaders := http_utils.HeadersToString(request.Header)

		details := fmt.Sprintf(`
Original Request:
	-	URL: %s
	-	Method: %s
	-	Status Code: %d
	-	Response Size: %d bytes


Attempted the bypass by making a request to %s with the following headers:

%s


Response received:
	-	Status Code: %d
	-	Response Size: %d bytes
`, original.URL, original.Method, original.StatusCode, original.ResponseBodySize, request.URL, bypassHeaders, history.StatusCode, history.ResponseBodySize)

		confidence := 75
		if history.StatusCode >= 200 && history.StatusCode < 300 {
			confidence = 90
		} else if history.StatusCode >= 400 {
			confidence = 40
		}

		db.CreateIssueFromHistoryAndTemplate(history, db.ForbiddenBypassCode, details, confidence, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID, &options.ScanID, &options.ScanJobID)
	}
}
