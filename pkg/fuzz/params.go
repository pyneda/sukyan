package fuzz

import (
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"

	"github.com/rs/zerolog/log"
)

// ParameterFuzzer to be used by other scan techniques
type ParameterFuzzer struct {
	Config        FuzzerConfig
	Params        []string
	TestAllParams bool
	client        *http.Client
}

type parameterFuzzerTask struct {
	url     string
	payload payloads.PayloadInterface
}

func (f *ParameterFuzzer) checkConfig() {
	if f.Config.Concurrency == 0 {
		log.Info().Interface("fuzzer", f).Msg("Concurrency is not set, setting 4 as default")
		f.Config.Concurrency = 4
	}
	if f.client == nil {
		f.client = http_utils.CreateHttpClient()
	}
}

// GetExpectedResponses attempts to gather common url response, for differential evaluation. Needs to be improved a lot
func (f *ParameterFuzzer) GetExpectedResponses() (expectedResponses ExpectedResponses) {
	// Get base response
	base, err := http.Get(f.Config.URL)
	if err != nil {
		log.Error().Err(err).Str("url", f.Config.URL).Msg("Error making GET request to gather base response")
	}
	baseExpectedResponse := ExpectedResponse{
		Response: *base,
		Err:      err,
	}
	baseBody, baseSize, err := http_utils.ReadResponseBodyData(base)
	baseExpectedResponse.Body = string(baseBody)
	baseExpectedResponse.BodySize = baseSize
	expectedResponses.Base = baseExpectedResponse
	if base.StatusCode != 200 {
		log.Warn().Int("status", base.StatusCode).Msg("Base url to fuzz does not response with 200, will test anyways")
	}

	// Attempt to get a 404
	notFoundURL, err := lib.Build404URL(f.Config.URL)
	if err != nil {
		log.Error().Err(err).Str("url", f.Config.URL).Msg("There was an error building a url to gather a 404 response")
	} else {
		notFound, err := http.Get(notFoundURL)
		notFoundExpectedResponse := ExpectedResponse{
			Response: *notFound,
			Err:      err,
		}
		if err != nil {
			log.Error().Err(err).Str("url", notFoundURL).Msg("Error making GET request to gather 404 response")
		} else {
			log.Debug().Int("status", notFound.StatusCode).Str("url", notFoundURL).Msg("Gathered 404 response")
		}

		baseBody, baseSize, err := http_utils.ReadResponseBodyData(base)
		baseExpectedResponse.Body = string(baseBody)
		baseExpectedResponse.BodySize = baseSize
		expectedResponses.NotFound = notFoundExpectedResponse
		if notFound.StatusCode != 404 {
			log.Warn().Str("original", f.Config.URL).Str("tested", notFoundURL).Msg("Gathered a non 404 status code attempting to fingerprint not found pages")
		}

	}
	// Get
	return expectedResponses
}

// Run starts the fuzzing job
// func (f *ParameterFuzzer) Run(payloads []string, results chan FuzzResult) {
func (f *ParameterFuzzer) Run(payloads []payloads.PayloadInterface, results chan FuzzResult) {
	var wg sync.WaitGroup
	f.checkConfig()
	// Declare the channels
	totalPendingChannel := make(chan int)
	pendingTasks := make(chan parameterFuzzerTask)

	go f.Monitor(pendingTasks, results, totalPendingChannel)
	// Schedule workers
	for i := 0; i < f.Config.Concurrency; i++ {
		wg.Add(1)
		go f.Worker(&wg, pendingTasks, results, totalPendingChannel)
	}

	go func() {
		// Communicate with workers to send them new fuzzing tasks
		params := lib.GetParametersToTest(f.Config.URL, f.Params, f.TestAllParams)
		log.Warn().Strs("params", params).Msg("Parameters to test in param fuzzer")
		for _, param := range params {
			for _, payload := range payloads {
				fuzzURL, err := lib.BuildURLWithParam(f.Config.URL, param, payload.GetValue(), false)
				if err != nil {
					log.Error().Err(err).Str("param", param).Str("payload", payload.GetValue()).Str("url", f.Config.URL).Msg("Error building url to fuzz")
				} else {
					task := parameterFuzzerTask{
						url:     fuzzURL,
						payload: payload,
					}
					pendingTasks <- task
					totalPendingChannel <- 1
				}

			}
		}
	}()
}

// Monitor checks when the job has finished
func (f *ParameterFuzzer) Monitor(pendingTasks chan parameterFuzzerTask, results chan FuzzResult, totalPendingChannel chan int) {
	count := 0
	for c := range totalPendingChannel {
		log.Debug().Int("count", count).Int("received", c).Msg("Monitor received data from totalPendingChannel")
		count += c
		if count == 0 {
			// Close the channels
			log.Debug().Msg("CrawlMonitor closing all the communication channels")
			close(pendingTasks)
			close(totalPendingChannel)
			close(results)
		}
	}
}

// Worker makes the request and processes the result
func (f *ParameterFuzzer) Worker(wg *sync.WaitGroup, pendingTasks chan parameterFuzzerTask, results chan FuzzResult, totalPendingChannel chan int) {
	for task := range pendingTasks {
		log.Debug().Interface("task", task).Msg("New fuzzer task received by parameter worker")
		var result FuzzResult
		req, _ := http.NewRequest("GET", task.url, nil)
		// if err != nil {
		// 	log.Error().Err(err).Str("url", task.url).Msg("Error creating GET request")
		// 	return
		// }
		response, err := f.client.Do(req)
		result.URL = task.url
		result.Err = err
		result.Payload = task.payload
		if err != nil {
			log.Error().Err(err).Str("url", task.url).Msg("Error making GET request")
		} else {
			result.Response = *response
		}
		results <- result
		totalPendingChannel <- -1
	}
	wg.Done()
}
