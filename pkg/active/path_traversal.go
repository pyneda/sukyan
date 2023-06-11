package active

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"strings"

	"github.com/rs/zerolog/log"
)

// PathTraversalAudit configuration
type PathTraversalAudit struct {
	URL                        string
	Concurrency                int
	Params                     []string
	PayloadsDepth              int
	Platform                   string
	StopAfterSuccess           bool
	OnlyCommonVulnerableParams bool
	HeuristicRecords           []fuzz.HeuristicRecord
	ExpectedResponses          fuzz.ExpectedResponses
}

// Run starts the audit
func (a *PathTraversalAudit) Run() {
	// Could allow to provide a fixed payload list + option to merge with generated or only use the list
	generatedPayloads := payloads.GetPathTraversalPayloads(a.PayloadsDepth, a.Platform)
	log.Info().Int("payloads", len(generatedPayloads)).Msg("PathTraversalAudit starting to run")
	var payloads []payloads.PayloadInterface
	// Convert payloads to interface (really required?)
	for _, p := range generatedPayloads {
		payloads = append(payloads, p)
	}
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
	// Get expected responses for "verification"
	a.ExpectedResponses = fuzzer.GetExpectedResponses()

	// Schedule the fuzzer
	fuzzer.Run(payloads, resultsChannel)

	// Receives the results from the channel and creates a goroutine to process them
	for result := range resultsChannel {
		log.Info().Str("url", result.URL).Int("status", result.Response.StatusCode).Msg("Received fuzz result")
		a.ProcessResult(&result)
	}
}

// ProcessResult processes a result to verify if it's vulnerable or not, this logic could be extracted to a differential analysis function
func (a *PathTraversalAudit) ProcessResult(result *fuzz.FuzzResult) {
	// Should check how to handle errors better
	grepStrings := []string{"root", "www-data", "Ubuntu", "Linux", "Debian", "CentOS", "; for 16-bit app support", `C:\`, "OLEMessaging="}
	var matchedStrings []string
	var matchedStringsInExpectedResults []string
	var confidence int

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
	// Check if some of the grep strings is in the response body (always, not just on 200)
	for _, grepString := range grepStrings {
		if strings.Contains(body, grepString) {
			matchedStrings = append(matchedStrings, grepString)
			if strings.Contains(a.ExpectedResponses.Base.Body, grepString) {
				matchedStringsInExpectedResults = append(matchedStringsInExpectedResults, grepString)
			}
		}
	}
	record.Matched = matchedStrings

	if result.Response.StatusCode == a.ExpectedResponses.Base.Response.StatusCode { // compare with expected response
		if bodySize == a.ExpectedResponses.Base.BodySize || strings.EqualFold(body, a.ExpectedResponses.Base.Body) {
			if len(matchedStrings) > 0 && len(matchedStrings) > len(matchedStringsInExpectedResults) {
				// Confidence if almost same response but has more matched strings than the original
				confidence = 30
			} else {
				// Confidence if everything is the same
				confidence = 0
			}
		} else {
			if len(matchedStrings) > 0 && len(matchedStrings) > len(matchedStringsInExpectedResults) {
				confidence = 60
			} else {
				// Confidence if everything is the same
				confidence = 5
			}
		}
	} else {
		// Compare when status code is different than expected
		if len(matchedStrings) > 0 && len(matchedStrings) > len(matchedStringsInExpectedResults) {
			confidence = 80
		} else {
			confidence = 10
		}
	}

	if confidence > 50 {
		issueDescription := fmt.Sprintf("A path traversal vulnerability has been detected. The differential analy found the following matces in the received response `%s`\nThe ones found in the original are: `%s`\nThis might be a False Positive.", matchedStrings, matchedStringsInExpectedResults)
		issue := db.Issue{
			Title:         "Path Traversal",
			Description:   issueDescription,
			Code:          "path-traversal",
			Cwe:           22,
			Payload:       result.Payload.GetValue(),
			URL:           result.URL,
			StatusCode:    result.Response.StatusCode,
			HTTPMethod:    "GET",
			Request:       "Not implemented",
			Response:      body,
			FalsePositive: false,
			Confidence:    confidence,
			Severity:      "High",
		}
		db.Connection.CreateIssue(issue)
		log.Error().Str("payload", result.Payload.GetValue()).Strs("matches", matchedStrings).Strs("originalMatches", matchedStringsInExpectedResults).Int("confidence", confidence).Str("url", result.URL).Msg("New path traversal vulnerability added to database")
	} else if confidence > 25 {
		log.Error().Str("payload", result.Payload.GetValue()).Strs("matches", matchedStrings).Strs("originalMatches", matchedStringsInExpectedResults).Int("confidence", confidence).Str("url", result.URL).Msg("Possible path traversal or FP which would need review")
	} else {
		log.Debug().Str("payload", result.Payload.GetValue()).Strs("matches", matchedStrings).Strs("originalMatches", matchedStringsInExpectedResults).Int("confidence", confidence).Str("url", result.URL).Msg("Path traversal with lower confidence than 25")
	}
	// Append the heuristic record, not used by now, but should/could be
	a.HeuristicRecords = append(a.HeuristicRecords, record)
}
