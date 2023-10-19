package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"net/http"
)

type ActiveModuleOptions struct {
	WorkspaceID uint
	TaskID      uint
	TaskJobID   uint
	Concurrency int
}

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
}

var bypassURLs = []string{
	"http://127.0.0.1",
	"https://127.0.0.1",
	"http://localhost",
	"https://localhost",
	"http://127.0.0.1:80",
	"http://127.0.0.1:443",
	"https://127.0.0.1:80",
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

func AuthBypassScan(history *db.History, options ActiveModuleOptions) {
	auditLog := log.With().Str("audit", "bypass").Str("url", history.URL).Uint("workspace", options.WorkspaceID).Logger()

	if history.StatusCode != 401 || history.StatusCode != 403 {
		auditLog.Warn().Msg("Skipping auth bypass scan because the status code is not 401 or 403")
		return
	}
	if options.Concurrency == 0 {
		options.Concurrency = 5
	}
	p := pool.New().WithMaxGoroutines(options.Concurrency)

	allHeaderTypes := [][]HeaderTest{ipBasedHeaders, urlBasedHeaders, portBasedHeaders, pathBasedHeaders}

	for _, headers := range allHeaderTypes {
		valueCombinations := generateCombinations(headers)

		for _, combination := range valueCombinations {
			comb := combination
			p.Go(func() {
				request, err := http_utils.BuildRequestFromHistoryItem(history)
				if err != nil {
					auditLog.Error().Err(err).Msg("Error creating the request")
					return
				}
				for header, value := range comb {
					request.Header.Set(header, value)
				}
				sendRequestAndCheckBypass(request, history, options, auditLog)
			})
		}
	}
	p.Wait()
	auditLog.Info().Msg("Finished auth bypass scan")
}

func generateCombinations(headers []HeaderTest) []map[string]string {
	if len(headers) == 0 {
		return []map[string]string{}
	}

	currentHeader := headers[0]
	remainingHeaders := headers[1:]

	subCombinations := generateCombinations(remainingHeaders)
	if len(subCombinations) == 0 {
		subCombinations = append(subCombinations, make(map[string]string))
	}

	var combinations []map[string]string
	for _, value := range currentHeader.Values {
		for _, subCombination := range subCombinations {
			newCombination := make(map[string]string)
			for k, v := range subCombination {
				newCombination[k] = v
			}
			newCombination[currentHeader.HeaderName] = value
			combinations = append(combinations, newCombination)
		}
	}
	return combinations
}

func sendRequestAndCheckBypass(request *http.Request, original *db.History, options ActiveModuleOptions, auditLog zerolog.Logger) {
	client := http_utils.CreateHttpClient()
	response, err := client.Do(request)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error during request")
		return
	}

	history, err := http_utils.ReadHttpResponseAndCreateHistory(response, http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         options.WorkspaceID,
		TaskID:              options.TaskID,
		CreateNewBodyStream: false,
	})
	if history.StatusCode != 401 && history.StatusCode != 403 {
		originalHeaders, _ := original.GetResponseHeadersAsString()
		newHeaders, _ := history.GetResponseHeadersAsString()
		details := fmt.Sprintf(`
Original Request:
	URL: %s
	Method: %s
	Status Code: %d
	Response Size: %d bytes
	Response Headers:
	%s

Attempted Bypass with Headers:
	%s

Bypassed Request:
	Status Code: %d
	Response Size: %d bytes
	Response Headers:
	%s
`, original.URL, original.Method, original.StatusCode, original.ResponseBodySize, originalHeaders, request.Header, history.StatusCode, history.ResponseBodySize, newHeaders)
		db.CreateIssueFromHistoryAndTemplate(history, db.ForbiddenBypassCode, details, 80, "", &options.WorkspaceID, &options.TaskID, &options.TaskJobID)
	}
}
