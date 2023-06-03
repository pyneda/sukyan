package active

import (
	"fmt"
	"net/http"
	"sync"
	"sukyan/db"
	"sukyan/lib"
	"sukyan/pkg/fuzz"
	"sukyan/pkg/http_utils"
	"sukyan/pkg/payloads"

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
	return append(headers, []string{"Host", "X-Forwarded-Host"}...)
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
	// - Check if response differs when the hear appears or not
	// - Check with current request URL host
	// - Use the data gathered in previous steps to compare with the current implementation results
	auditLog := log.With().Str("audit", "host-header-injection").Interface("auditItem", item).Str("url", a.URL).Logger()
	response, err := http.Get(a.URL)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error during request")
		return
	}
	body, _, err := http_utils.ReadResponseBodyData(response)
	if err != nil {
		auditLog.Error().Err(err).Msg("Error reading response body data")
	}
	isInBody, err := item.payload.MatchAgainstString(body)
	if isInBody {
		issueDescription := fmt.Sprintf("A host header injection vulnerability has been detected in %s. The audit test send the following payload `%s` to `%s` header and it has been verified is included back in the response", a.URL, item.payload.GetValue(), item.header)
		issue := db.Issue{
			Title:         "Host Header Injection",
			Description:   issueDescription,
			Code:          "host-header-injection",
			Cwe:           20,
			Payload:       item.payload.GetValue(),
			URL:           a.URL,
			StatusCode:    response.StatusCode,
			HTTPMethod:    "GET",
			Request:       "Not implemented",
			Response:      body,
			FalsePositive: false,
			Confidence:    75,
		}
		log.Warn().Interface("issue", issue).Msg("New issue found")
		db.Connection.CreateIssue(issue)
	}
}
