package active

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
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
	ParamsToTest               []string
	StopAfterSuccess           bool
	OnlyCommonVulnerableParams bool
	HeuristicRecords           []fuzz.HeuristicRecord
	ExpectedResponses          fuzz.ExpectedResponses
	InteractionsManager        *integrations.InteractionsManager
}

// Run starts the audit
func (a *SSRFAudit) Run() {
	// Launching separatelly for each parameter since the payload should use a unique interaction URL
	// Due to this, we have to get the parameters to test here, even though the fuzzer does it already
	// Should probably rethink on how to handle oob better
	params := lib.GetParametersToTest(a.URL, a.ParamsToTest, false)
	log.Info().Int("params", len(params)).Msg("SSRFAudit starting to run")
	for _, param := range params {
		log.Warn().Str("param", param).Msg("Launching SSRFAudit against parameter")
		a.RunAgainstParameter(param)
	}
	log.Info().Msg("SSRFAudit finished")

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
	parameters := []string{parameter}
	fuzzer := fuzz.ParameterFuzzer{
		Config: fuzz.FuzzerConfig{
			URL:         a.URL,
			Concurrency: a.Concurrency,
		},
		Params:        parameters,
		TestAllParams: false,
	}
	// Get expected responses for "verification"
	// a.ExpectedResponses = fuzzer.GetExpectedResponses()

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
		log.Error().Err(result.Err).Str("url", result.URL).Msg("Error sending SSRF test request")
	}

	history, err := http_utils.ReadHttpResponseAndCreateHistory(&result.Response, db.SourceScanner)
	if err != nil {
		log.Error().Err(err).Str("url", result.URL).Msg("Error creating history from SSRF test request")
	}
	// historyID := uint(0)
	// if history != nil {
	// 	historyID = history.ID
	// } else {
	// 	log.Warn().Str("url", result.URL).Msg("Could not create history from SSRF test request")
	// }
	interactionData := result.Payload.GetInteractionData()
	oobTest := db.OOBTest{
		Code:              db.SSRFCode,
		TestName:          "Server Side Request Forgery",
		InteractionDomain: interactionData.InteractionDomain,
		InteractionFullID: interactionData.InteractionFullID,
		Target:            result.URL,
		Payload:           result.Payload.GetValue(),
		HistoryID:         &history.ID,
		// This should be improved by providing it into the fuzz task/result
		InsertionPoint: "parameter",
	}
	db.Connection.CreateOOBTest(oobTest)
	log.Debug().Str("url", result.URL).Str("payload", result.Payload.GetValue()).Msg("SSRF payload sent")
}
