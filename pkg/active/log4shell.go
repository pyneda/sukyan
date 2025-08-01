package active

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// Log4ShellInjectionAudit configuration
type Log4ShellInjectionAudit struct {
	URL                 string
	Concurrency         int
	HeuristicRecords    []fuzz.HeuristicRecord
	ExpectedResponses   fuzz.ExpectedResponses
	ExtraHeadersToTest  []string
	InteractionsManager *integrations.InteractionsManager
	WorkspaceID         uint
	TaskID              uint
	TaskJobID           uint
	Mode                scan_options.ScanMode
}

type log4ShellAuditItem struct {
	payload payloads.PayloadInterface
	header  string // should be an injection point interface when implemented
}

// GetDefaultHeadersToTest returns the default headers that are tested in this audit
func (a *Log4ShellInjectionAudit) GetDefaultHeadersToTest() (headers []string) {
	return append(headers, []string{
		"Accept",
		"Accept-Charset",
		"Accept-Datetime",
		"Accept-Encoding",
		"Accept-Language",
		"Access-Control-Request-Headers",
		"Access-Control-Request-Method",
		"Authorization",
		"Authentication",
		"Bearer",
		"Cache-Control",
		"Cf-Connecting_ip",
		"Cf-Connecting-Ip",
		"Client-Ip",
		"Contact",
		"Content-Disposition",
		"Content-Encoding",
		"Content-Type",
		"Cookie",
		"Date",
		"Dnt",
		"Expect",
		"Forwarded",
		"Forwarded-For",
		"Forwarded-For-Ip",
		"Fowarded",
		"From",
		"Host",
		"Hostname",
		"IP",
		"IPaddress",
		"If-Modified-Since",
		"Location",
		"Max-Forwards",
		"Origin",
		"Originating-Ip",
		"Pragma",
		"Proxy-Authorization",
		"Range",
		"Referer",
		"TE",
		"True-Client-IP",
		"True-Client-Ip",
		"Upgrade-Insecure-Requests",
		"upgrade-insecure-requests",
		"User-Agent",
		"Username",
		"Via",
		"Warning",
		"X-Api-Version",
		"X-CSRF-Token",
		"X-Client-IP",
		"X-Client-Ip",
		"X-Druid-Comment",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"X-Forwarded-Protocol",
		"X-Forwarded-Scheme",
		"X-Forwarded-Server",
		"X-HTTP-Host-Override",
		"X-Host",
		"X-Http-Method-Override",
		"X-Leakix",
		"X-Method-Override",
		"X-Original-URL",
		"X-Originating-IP",
		"X-Originating-Ip",
		"X-Real-IP",
		"X-Real-Ip",
		"X-Remote-Addr",
		"X-Remote-IP",
		"X-Remote-Ip",
		"X-Requested-With",
		"X-Rewrite-URL",
		"X-Target-IP",
		"X-Wap-Profile",
	}...)
}

// GetHeadersToTest merges the default headers to test and the provided ExtraHeadersToTest
func (a *Log4ShellInjectionAudit) GetHeadersToTest() (headers []string) {
	headers = a.GetDefaultHeadersToTest()
	for _, header := range a.ExtraHeadersToTest {
		if !lib.SliceContains(headers, header) {
			headers = append(headers, header)
		}
	}
	return headers
}

// Run starts the audit
func (a *Log4ShellInjectionAudit) Run() {
	auditItemsChannel := make(chan log4ShellAuditItem)
	pendingChannel := make(chan int)
	var wg sync.WaitGroup

	// Schedule workers
	for i := 0; i < a.Concurrency; i++ {
		wg.Add(1)
		go a.worker(auditItemsChannel, pendingChannel, &wg)
	}
	// Schedule goroutine to monitor pending tasks
	go a.monitor(auditItemsChannel, pendingChannel)
	log.Info().Str("url", a.URL).Msg("Starting to schedule Log4Shell injection audit items")

	// Add tests to the channel
	for _, header := range a.GetHeadersToTest() {
		payload := payloads.GenerateLog4ShellPayload(a.InteractionsManager)
		pendingChannel <- 1
		auditItemsChannel <- log4ShellAuditItem{
			payload: payload,
			header:  header,
		}
	}
	wg.Wait()
	log.Info().Str("url", a.URL).Msg("All Log4Shell injection audit items completed")
}

func (a *Log4ShellInjectionAudit) worker(auditItems chan log4ShellAuditItem, pendingChannel chan int, wg *sync.WaitGroup) {
	for auditItem := range auditItems {
		a.testItem(auditItem)
		pendingChannel <- -1
	}
	wg.Done()
}

func (a *Log4ShellInjectionAudit) monitor(auditItems chan log4ShellAuditItem, pendingChannel chan int) {
	count := 0
	log.Debug().Str("url", a.URL).Msg("Log4Shell audit monitor started")
	for c := range pendingChannel {
		count += c
		if count == 0 {
			log.Info().Str("url", a.URL).Msg("Log4Shell audit finished, closing communication channels")
			close(auditItems)
			close(pendingChannel)
		}
	}
}

func (a *Log4ShellInjectionAudit) testItem(item log4ShellAuditItem) {
	client := http_utils.CreateHttpClient()
	auditLog := log.With().Str("audit", "log4shell").Interface("auditItem", item).Str("url", a.URL).Logger()
	request, err := http.NewRequest("GET", a.URL, nil)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error creating the request")
		return
	}

	request.Header.Set(item.header, item.payload.GetValue())

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
	isInResponse, err := item.payload.MatchAgainstString(string(history.RawResponse))

	// This might be un-needed
	if isInResponse {
		issueDescription := fmt.Sprintf("While testing for Log4Shell to %s. The following payload `%s` has been sent in the `%s` header and it has been included back in the response.", a.URL, item.payload.GetValue(), item.header)

		issue := db.Issue{
			Title:         "Reflected Log4Shell Payload",
			Description:   issueDescription,
			Code:          "reflected-log4shell-payload",
			Cwe:           20,
			Payload:       item.payload.GetValue(),
			URL:           a.URL,
			StatusCode:    history.StatusCode,
			HTTPMethod:    "GET",
			Request:       history.RawRequest,
			Response:      []byte(history.RawResponse), // body,
			FalsePositive: false,
			Confidence:    75,
			Severity:      "Medium",
			WorkspaceID:   &a.WorkspaceID,
		}
		db.Connection().CreateIssue(issue)

		log.Warn().Str("issue", issue.Title).Str("url", history.URL).Msg("New issue found")
	}

	interactionData := item.payload.GetInteractionData()
	insertionPoint := fmt.Sprintf("%s header", item.header)
	oobTest := db.OOBTest{
		Code:              db.Log4shellCode,
		TestName:          "Log4Shell",
		InteractionDomain: interactionData.InteractionDomain,
		InteractionFullID: interactionData.InteractionFullID,
		Target:            a.URL,
		Payload:           item.payload.GetValue(),
		HistoryID:         &history.ID,
		InsertionPoint:    insertionPoint,
		WorkspaceID:       &a.WorkspaceID,
		TaskID:            &a.TaskID,
		TaskJobID:         &a.TaskJobID,
	}
	db.Connection().CreateOOBTest(oobTest)
}
