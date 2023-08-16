package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/rs/zerolog/log"
)

// https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/07-Input_Validation_Testing/17-Testing_for_Host_Header_Injection.html

// HostHeaderInjectionAudit configuration
type HostHeaderInjectionAudit struct {
	URL                string
	Concurrency        int
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
		fmt.Println("Error:", err)
		return
	}

	request.Header.Set(item.header, item.payload.GetValue())
	requestDump, _ := httputil.DumpRequestOut(request, true)

	response, err := client.Do(request)

	if err != nil {
		auditLog.Error().Err(err).Msg("Error during request")
		return
	}
	responseDump, err := httputil.DumpResponse(response, true)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error reading response body data")
	}
	isInResponse, err := item.payload.MatchAgainstString(string(responseDump))
	// isInBody, err := item.payload.MatchAgainstString(body)
	// isInHeaders := false
	// for header, values := range response.Header {
	// 	for _, value := range values {
	// 		isIncluded, _ := item.payload.MatchAgainstString(value)
	// 		if isIncluded {
	// 			isInHeaders = true
	// 			log.Info().Str("header", header).Str("value", value).Msg("Host header injection detected")
	// 			break
	// 		}
	// 	}
	// }
	// if isInBody || isInHeaders {
	if isInResponse {
		issueDescription := fmt.Sprintf("A host header injection vulnerability has been detected in %s. The audit test send the following payload `%s` in `%s` header and it has been verified is included back in the response", a.URL, item.payload.GetValue(), item.header)

		issue := db.Issue{
			Title:         "Host Header Injection",
			Description:   issueDescription,
			Code:          "host-header-injection",
			Cwe:           20,
			Payload:       item.payload.GetValue(),
			URL:           a.URL,
			StatusCode:    response.StatusCode,
			HTTPMethod:    "GET",
			Request:       requestDump,
			Response:      responseDump, // body,
			FalsePositive: false,
			Confidence:    75,
			Severity:      "Medium",
			WorkspaceID: &a.WorkspaceID

		}
		log.Warn().Str("issue", issue.Title).Str("url", a.URL).Msg("New issue found")
		db.Connection.CreateIssue(issue)
	}
}
