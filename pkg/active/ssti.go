package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"

	"github.com/rs/zerolog/log"
)

// SSTIAudit configuration
type SSTIAudit struct {
	URL         string
	Concurrency int
	Params      []string
	// Syntaxes                 []TemplateLanguageSyntax
	StopAfterSuccess           bool
	OnlyCommonVulnerableParams bool
	ExpectedResponses          fuzz.ExpectedResponses
}

// Run starts the audit
func (a *SSTIAudit) Run() {
	generatedPayloads := payloads.GenerateSSTIPayloads()
	var payloads []payloads.PayloadInterface
	// Convert payloads to interface (really required?)
	for _, p := range generatedPayloads {
		payloads = append(payloads, p)
	}

	log.Info().Int("payloads", len(generatedPayloads)).Msg("SSTIAudit starting to run")

	// Create a channel to communicate with the fuzzer
	resultsChannel := make(chan fuzz.FuzzResult) // , 1000)
	// Create a parameter fuzzer
	fuzzer := fuzz.ParameterFuzzer{
		Config: fuzz.FuzzerConfig{
			URL:         a.URL,
			Concurrency: a.Concurrency,
		},
		Params: a.Params,
	}
	// Get expected responses
	a.ExpectedResponses = fuzzer.GetExpectedResponses()

	// Schedule the fuzzer
	fuzzer.Run(payloads, resultsChannel)

	// Receives the results from the channel and creates a goroutine to process them
	for result := range resultsChannel {
		log.Info().Str("url", result.URL).Int("status", result.Response.StatusCode).Msg("Received SSTI fuzz result")
		a.ProcessResult(&result)
	}
}

// ProcessResult processes a result to verify if it's vulnerable or not, this logic could be extracted to a differential analysis function
func (a *SSTIAudit) ProcessResult(result *fuzz.FuzzResult) {
	var matchedStrings []string
	var matchedStringsInExpectedResults []string
	confidence := 0
	isVulnerable := false

	record := fuzz.HeuristicRecord{
		URL:        result.URL,
		StatusCode: result.Response.StatusCode,
	}
	if result.Err != nil {

	}
	// Process the response
	body, bodySize, err := http_utils.ReadResponseBodyData(&result.Response)
	if err != nil {
		log.Error().Err(err).Interface("record", record).Msg("Error reading response body")
	}
	record.BodySize = bodySize
	// record.Matched = result.Payload.Re
	isInResponse, err := result.Payload.MatchAgainstString(body)
	log.Info().Str("payload", result.Payload.GetValue()).Str("url", result.URL).Bool("match", isInResponse).Int("status", result.Response.StatusCode).Msg("Evaluating SSTI test result")

	if err != nil {
		log.Warn().Err(err).Msg("Error matching SSTI payload against string")
	}
	if isInResponse {
		isVulnerable = true
		isInDefaultResponse, err := result.Payload.MatchAgainstString(a.ExpectedResponses.Base.Body)
		if err != nil {
			log.Warn().Err(err).Msg("Error checking if SSTI vulnerability is a false positive")
		}
		if isInDefaultResponse {
			confidence = 25
		} else {
			confidence = 75
		}
		// When we can mutate the payload to verify the content changes as expected, confidence between 90-100

	}

	if isVulnerable {
		log.Error().Int("confidence", confidence).Str("url", result.URL).Msg("SSTI vulnerability found")
		issueDescription := fmt.Sprintf("A SSTI vulnerability has been detected in %s.", result.URL)
		issue := db.Issue{
			Title:         "SSTI",
			Description:   issueDescription,
			Code:          "ssti",
			Cwe:           94,
			Payload:       "Not included yet for SSTI",
			URL:           result.URL,
			StatusCode:    result.Response.StatusCode,
			HTTPMethod:    "GET",
			Request:       "Not implemented",
			Response:      body,
			FalsePositive: false,
			Confidence:    confidence,
		}
		db.Connection.CreateIssue(issue)
		log.Error().Strs("matches", matchedStrings).Strs("originalMatches", matchedStringsInExpectedResults).Int("confidence", confidence).Str("url", result.URL).Msg("New path traversal vulnerability added to database")
	}
	// Append the heuristic record, not used by now, but should/could be
	// a.HeuristicRecords = append(a.HeuristicRecords, record)
}
