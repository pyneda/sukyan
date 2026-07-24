package active

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

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

// hostHeaderInjectionFinding is a single confirmed reflection. An endpoint that
// echoes arbitrary headers reflects every one we try, so findings are collected
// and reported as one issue instead of one issue per header.
type hostHeaderInjectionFinding struct {
	header  string
	payload string
	history *db.History
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

	var (
		mu       sync.Mutex
		findings []hostHeaderInjectionFinding
	)

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
				if finding := a.testItem(ctx, item); finding != nil {
					mu.Lock()
					findings = append(findings, *finding)
					mu.Unlock()
				}
			})
		}
	}

	// Wait for all workers to complete
	p.Wait()

	a.reportFindings(findings)

	log.Info().Str("url", a.URL).Int("reflected_headers", len(findings)).Msg("All host header injection audit items completed")
}

func (a *HostHeaderInjectionAudit) testItem(ctx context.Context, item hostHeaderInjectionAuditItem) *hostHeaderInjectionFinding {
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
		return nil
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
		return nil
	}

	history := executionResult.History
	isInResponse, _ := item.payload.MatchAgainstString(string(history.RawResponse))

	if !isInResponse {
		return nil
	}

	return &hostHeaderInjectionFinding{
		header:  item.header,
		payload: item.payload.GetValue(),
		history: history,
	}
}

// reportFindings emits a single consolidated issue for the endpoint, listing
// every header that was reflected and attaching each supporting request.
func (a *HostHeaderInjectionAudit) reportFindings(findings []hostHeaderInjectionFinding) {
	if len(findings) == 0 {
		return
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].header < findings[j].header })

	var sb strings.Builder
	fmt.Fprintf(&sb, "The following %d header(s) were reflected back in the response:\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&sb, " - `%s`: payload `%s`\n", f.header, f.payload)
	}

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		findings[0].history, db.HostHeaderInjectionCode, sb.String(), 75, "",
		&a.Options.WorkspaceID, &a.Options.TaskID, &a.Options.TaskJobID,
		&a.Options.ScanID, &a.Options.ScanJobID,
	)
	if err != nil {
		log.Error().Err(err).Str("url", a.URL).Msg("Failed to create host header injection issue")
		return
	}

	if len(findings) > 1 {
		extra := make([]*db.History, 0, len(findings)-1)
		for _, f := range findings[1:] {
			extra = append(extra, f.history)
		}
		if err := issue.AppendHistories(extra); err != nil {
			log.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link additional host header injection histories")
		}
	}
}
