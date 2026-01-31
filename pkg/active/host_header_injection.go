package active

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

// https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/07-Input_Validation_Testing/17-Testing_for_Host_Header_Injection.html

// HostHeaderInjectionAudit configuration
type HostHeaderInjectionAudit struct {
	Options            ActiveModuleOptions
	URL                string
	HistoryItem        *db.History
	HeuristicRecords   []fuzz.HeuristicRecord
	ExpectedResponses  fuzz.ExpectedResponses
	ExtraHeadersToTest []string
}

type hostHeaderInjectionAuditItem struct {
	payload payloads.PayloadInterface
	header  string // should be an injection point interface when implemented
}

// GetDefaultHeadersToTest returns the default headers that are tested in this audit
func (a *HostHeaderInjectionAudit) GetDefaultHeadersToTest() (headers []string) {
	return append(headers, []string{
		"Host",
		"X-Forwarded-Host",
		"X-Host",
		"X-Forwarded-Server",
		"X-HTTP-Host-Override",
		"X-Original-URL",
		"X-Rewrite-URL",
		"X-Originating-IP",
		"X-Remote-IP",
		"X-Client-IP",
		"X-Forwarded-For",
		"X-Target-IP",
		"X-Remote-Addr",
		"Fowarded",
		"True-Client-IP",
		"Via",
		"X-Real-IP",
		"X-Azure-ClientIP",
		"X-Azure-SocketIP",
	}...)
}

// GetHeadersToTest merges the default headers to test and the provided ExtraHeadersToTest
func (a *HostHeaderInjectionAudit) GetHeadersToTest() (headers []string) {
	headers = a.GetDefaultHeadersToTest()
	for _, header := range a.ExtraHeadersToTest {
		if !lib.SliceContains(headers, header) {
			headers = append(headers, header)
		}
	}
	return headers
}

// Run starts the audit
func (a *HostHeaderInjectionAudit) Run() {
	// Get context, defaulting to background if not provided
	ctx := a.Options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		log.Info().Str("url", a.URL).Msg("Host header injection audit cancelled before starting")
		return
	default:
	}

	p := pool.New().WithMaxGoroutines(a.Options.Concurrency)

	log.Info().Str("url", a.URL).Msg("Starting to schedule Host header injection audit items")

	// Add tests to the channel
schedulingLoop:
	for _, header := range a.GetHeadersToTest() {
		for _, payload := range payloads.GetHostHeaderInjectionPayloads() {
			// Check context before scheduling each item
			select {
			case <-ctx.Done():
				log.Info().Str("url", a.URL).Msg("Host header injection audit cancelled during scheduling")
				break schedulingLoop
			default:
			}

			item := hostHeaderInjectionAuditItem{
				payload: payload,
				header:  header,
			}

			p.Go(func() {
				// Check context inside worker
				select {
				case <-ctx.Done():
					return
				default:
				}
				a.testItem(ctx, item)
			})
		}
	}

	// Wait for all workers to complete
	p.Wait()

	log.Info().Str("url", a.URL).Msg("All host header injection audit items completed")
}

func (a *HostHeaderInjectionAudit) testItem(ctx context.Context, item hostHeaderInjectionAuditItem) {
	// Just basic implementation, by now just check if the payload appended in the host header appears in the response, still should:
	// - Check if response differs when the header appears or not
	// - Use the data gathered in previous steps to compare with the current implementation results
	// - Could use interactsh payloads
	// - Could also probably send all headers at once
	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	auditLog := log.With().Str("audit", "host-header-injection").Interface("auditItem", item).Str("url", a.URL).Logger()

	var request *http.Request
	var err error

	if a.HistoryItem != nil {
		request, err = http_utils.BuildRequestFromHistoryItem(a.HistoryItem)
		if err == nil {
			request = request.WithContext(ctx)
		}
	} else {
		request, err = http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	}

	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating request")
		return
	}

	request.Header.Set(item.header, item.payload.GetValue())

	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         uint(a.Options.WorkspaceID),
			TaskID:              uint(a.Options.TaskID),
			ScanID:              a.Options.ScanID,
			ScanJobID:           a.Options.ScanJobID,
			CreateNewBodyStream: true,
		},
	})

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Error during request")
		return
	}

	history := executionResult.History
	isInResponse, _ := item.payload.MatchAgainstString(string(history.RawResponse))

	if isInResponse {
		details := fmt.Sprintf("A host header injection vulnerability has been detected in %s. The audit test send the following payload `%s` in `%s` header and it has been verified is included back in the response", a.URL, item.payload.GetValue(), item.header)
		db.CreateIssueFromHistoryAndTemplate(history, db.HostHeaderInjectionCode, details, 75, "", &a.Options.WorkspaceID, &a.Options.TaskID, &a.Options.TaskJobID, &a.Options.ScanID, &a.Options.ScanJobID)
	}
}
