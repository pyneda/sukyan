package active

import (
	"context"
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

// HTTPMethodsAudit configuration
type HTTPMethodsAudit struct {
	Options     ActiveModuleOptions
	HistoryItem *db.History
}

type httpMethodsAudiItem struct {
	method string
}

func (a *HTTPMethodsAudit) GetMethodsToTest() (headers []string) {
	return append(headers, []string{
		// "GET",
		"POST",
		"PUT",
		"DELETE",
		"OPTIONS",
		"HEAD",
		"TRACE",
		"CONNECT",
		"PATCH",
	}...)
}

// Run starts the audit
func (a *HTTPMethodsAudit) Run() {
	// Get context, defaulting to background if not provided
	ctx := a.Options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		log.Info().Str("url", a.HistoryItem.URL).Msg("HTTP methods audit cancelled before starting")
		return
	default:
	}

	p := pool.New().WithMaxGoroutines(a.Options.Concurrency)

	log.Info().Str("url", a.HistoryItem.URL).Msg("Starting to schedule HTTPMethods injection audit items")

	// Add tests to the channel
	for _, method := range a.GetMethodsToTest() {
		if method != a.HistoryItem.Method {
			// Check context before scheduling each item
			select {
			case <-ctx.Done():
				log.Info().Str("url", a.HistoryItem.URL).Msg("HTTP methods audit cancelled during scheduling")
				return
			default:
			}

			item := httpMethodsAudiItem{
				method: method,
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
	p.Wait()
	log.Debug().Str("url", a.HistoryItem.URL).Msg("All HTTPMethods audit items completed")
}

func (a *HTTPMethodsAudit) testItem(ctx context.Context, item httpMethodsAudiItem) {
	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}
	auditLog := log.With().Str("audit", "httpMethods").Interface("auditItem", item).Str("url", a.HistoryItem.URL).Uint("workspace", a.Options.WorkspaceID).Logger()
	request, err := http_utils.BuildRequestFromHistoryItem(a.HistoryItem)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}
	request.Method = item.method
	request = request.WithContext(ctx)

	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         a.Options.WorkspaceID,
			TaskID:              a.Options.TaskID,
			ScanID:              a.Options.ScanID,
			ScanJobID:           a.Options.ScanJobID,
			CreateNewBodyStream: false,
		},
	})

	if executionResult.Err != nil {
		auditLog.Error().Err(executionResult.Err).Msg("Error during request")
		return
	}

	history := executionResult.History
	if history.StatusCode != 405 && history.StatusCode != 404 {
		// Should improve the issue template and probably all all the instances in the same issue
		issue := db.FillIssueFromHistoryAndTemplate(
			history,
			db.HttpMethodsCode,
			fmt.Sprintf("Received a %d status code making an %s request.", history.StatusCode, history.Method),
			80,
			"",
			&a.Options.WorkspaceID,
			&a.Options.TaskID,
			&a.Options.TaskJobID,
			&a.Options.ScanID,
			&a.Options.ScanJobID,
		)
		issue.Title = fmt.Sprintf("%s: %s", issue.Title, history.Method)
		db.Connection().CreateIssue(*issue)
		log.Warn().Str("issue", issue.Title).Str("url", history.URL).Uint("workspace", a.Options.WorkspaceID).Msg("New issue found")

	}
}
