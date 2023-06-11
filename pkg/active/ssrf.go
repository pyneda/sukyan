package active

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/rs/zerolog/log"
)

// SSRFAudit configuration
type SSRFAudit struct {
	URL                        string
	Concurrency                int
	Params                     []string
	PayloadsDepth              int
	Platform                   string
	StopAfterSuccess           bool
	OnlyCommonVulnerableParams bool
	HeuristicRecords           []fuzz.HeuristicRecord
	ExpectedResponses          fuzz.ExpectedResponses
	InteractionsManager        *integrations.InteractionsManager
}

// Run starts the audit
func (a *SSRFAudit) Run() {
	// Launching separatelly for each parameter since the payload should use a unique interaction URL
	for _, param := range a.Params {
		a.RunAgainstParameter(param)
	}

}

func (a *SSRFAudit) RunAgainstParameter(parameter string) {
	generatedPayloads := payloads.GenerateSSRFPayloads(a.InteractionsManager)
	var payloads []payloads.PayloadInterface
	for _, p := range generatedPayloads {
		payloads = append(payloads, p)
	}

	log.Info().Int("payloads", len(generatedPayloads)).Str("parameter", parameter).Msg("SSRFAudit starting to run")

	// Create a channel to communicate with the fuzzer
	resultsChannel := make(chan fuzz.FuzzResult)
	// Create a parameter fuzzer
	fuzzer := fuzz.ParameterFuzzer{
		Config: fuzz.FuzzerConfig{
			URL:         a.URL,
			Concurrency: a.Concurrency,
		},
		Params: []string{parameter},
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
func (a *SSRFAudit) ProcessResult(result *fuzz.FuzzResult) {

	// record := fuzz.HeuristicRecord{
	// 	URL:        result.URL,
	// 	StatusCode: result.Response.StatusCode,
	// }
	if result.Err != nil {

	}
	// Process the response
	_, _, err := http_utils.ReadResponseBodyData(&result.Response)
	if err != nil {
		log.Error().Err(err).Interface("result", result.URL).Msg("Error reading response body")
	}
	interactionData := result.Payload.GetInteractionData()
	oobTest := db.OOBTest{
		TestName:          "SSRF",
		InteractionDomain: interactionData.InteractionDomain,
		InteractionFullID: interactionData.InteractionFullID,
		Target:            result.URL,
		Payload:           result.Payload.GetValue(),
	}
	db.Connection.CreateOOBTest(oobTest)
}
