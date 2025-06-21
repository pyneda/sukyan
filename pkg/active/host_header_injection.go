package active

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"

	"github.com/rs/zerolog/log"
)

// TODO: Refactor required to work with History items, simpler concurrency and maybe even move to a YAML template

// https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/07-Input_Validation_Testing/17-Testing_for_Host_Header_Injection.html

// HostHeaderInjectionAudit configuration
type HostHeaderInjectionAudit struct {
	URL                string
	Concurrency        int
	HeuristicRecords   []fuzz.HeuristicRecord
	ExpectedResponses  fuzz.ExpectedResponses
	ExtraHeadersToTest []string
	WorkspaceID        uint
	TaskID             uint
	TaskJobID          uint
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
	auditItemsChannel := make(chan hostHeaderInjectionAuditItem)
	pendingChannel := make(chan int)
	var wg sync.WaitGroup

	// Schedule workers
	for i := 0; i < a.Concurrency; i++ {
		wg.Add(1)
		go a.worker(auditItemsChannel, pendingChannel, &wg)
	}
	// Schedule goroutine to monitor pending tasks
	go a.monitor(auditItemsChannel, pendingChannel)
	log.Info().Str("url", a.URL).Msg("Starting to schedule Host header injection audit items")

	// Add tests to the channel
	for _, header := range a.GetHeadersToTest() {
		for _, payload := range payloads.GetHostHeaderInjectionPayloads() {
			pendingChannel <- 1
			auditItemsChannel <- hostHeaderInjectionAuditItem{
				payload: payload,
				header:  header,
			}
		}
	}
	wg.Wait()
	log.Info().Str("url", a.URL).Msg("All host header injection audit items completed")
}

func (a *HostHeaderInjectionAudit) worker(auditItems chan hostHeaderInjectionAuditItem, pendingChannel chan int, wg *sync.WaitGroup) {
	for auditItem := range auditItems {
		a.testItem(auditItem)
		pendingChannel <- -1
	}
	wg.Done()
}

func (a *HostHeaderInjectionAudit) monitor(auditItems chan hostHeaderInjectionAuditItem, pendingChannel chan int) {
	count := 0
	log.Debug().Str("url", a.URL).Msg("Host header audit monitor started")
	for c := range pendingChannel {
		count += c
		if count == 0 {
			log.Info().Str("url", a.URL).Msg("Host header audit finished, closing communication channels")
			close(auditItems)
			close(pendingChannel)
		}
	}
}

func (a *HostHeaderInjectionAudit) testItem(item hostHeaderInjectionAuditItem) {
	// Just basic implementation, by now just check if the payload appended in the host header appears in the response, still should:
	// - Check if response differs when the header appears or not
	// - Use the data gathered in previous steps to compare with the current implementation results
	// - Could use interactsh payloads
	// - Could also probably send all headers at once
	client := http_utils.CreateHttpClient()
	auditLog := log.With().Str("audit", "host-header-injection").Interface("auditItem", item).Str("url", a.URL).Logger()
	request, err := http.NewRequest("GET", a.URL, nil)
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
			WorkspaceID:         uint(a.WorkspaceID),
			TaskID:              uint(a.TaskID),
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
		db.CreateIssueFromHistoryAndTemplate(history, db.HostHeaderInjectionCode, details, 75, "", &a.WorkspaceID, &a.TaskID, &a.TaskJobID)
	}
}
