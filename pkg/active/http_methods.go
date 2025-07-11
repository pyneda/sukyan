package active

import (
	"fmt"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// HTTPMethodsAudit configuration
type HTTPMethodsAudit struct {
	HistoryItem *db.History
	Concurrency int
	WorkspaceID uint
	TaskID      uint
	TaskJobID   uint
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
	auditItemsChannel := make(chan httpMethodsAudiItem)
	pendingChannel := make(chan int)
	var wg sync.WaitGroup

	// Schedule workers
	for i := 0; i < a.Concurrency; i++ {
		wg.Add(1)
		go a.worker(auditItemsChannel, pendingChannel, &wg)
	}
	// Schedule goroutine to monitor pending tasks
	go a.monitor(auditItemsChannel, pendingChannel)
	log.Info().Str("url", a.HistoryItem.URL).Msg("Starting to schedule HTTPMethods injection audit items")

	// Add tests to the channel
	for _, method := range a.GetMethodsToTest() {
		if method != a.HistoryItem.Method {
			pendingChannel <- 1
			auditItemsChannel <- httpMethodsAudiItem{
				method: method,
			}
		}

	}
	wg.Wait()
	log.Debug().Str("url", a.HistoryItem.URL).Msg("All HTTPMethods audit items completed")
}

func (a *HTTPMethodsAudit) worker(auditItems chan httpMethodsAudiItem, pendingChannel chan int, wg *sync.WaitGroup) {
	for auditItem := range auditItems {
		a.testItem(auditItem)
		pendingChannel <- -1
	}
	wg.Done()
}

func (a *HTTPMethodsAudit) monitor(auditItems chan httpMethodsAudiItem, pendingChannel chan int) {
	count := 0
	log.Debug().Str("url", a.HistoryItem.URL).Msg("HTTPMethods audit monitor started")
	for c := range pendingChannel {
		count += c
		if count == 0 {
			log.Debug().Str("url", a.HistoryItem.URL).Msg("HTTPMethods audit finished, closing communication channels")
			close(auditItems)
			close(pendingChannel)
		}
	}
}

func (a *HTTPMethodsAudit) testItem(item httpMethodsAudiItem) {
	client := http_utils.CreateHttpClient()
	auditLog := log.With().Str("audit", "httpMethods").Interface("auditItem", item).Str("url", a.HistoryItem.URL).Uint("workspace", a.WorkspaceID).Logger()
	request, err := http_utils.BuildRequestFromHistoryItem(a.HistoryItem)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}
	request.Method = item.method

	executionResult := http_utils.ExecuteRequest(request, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         a.WorkspaceID,
			TaskID:              a.TaskID,
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
			&a.WorkspaceID,
			&a.TaskID,
			&a.TaskJobID,
		)
		issue.Title = fmt.Sprintf("%s: %s", issue.Title, history.Method)
		db.Connection().CreateIssue(*issue)
		log.Warn().Str("issue", issue.Title).Str("url", history.URL).Uint("workspace", a.WorkspaceID).Msg("New issue found")

	}
}
