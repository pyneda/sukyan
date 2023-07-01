package fuzz

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type HistoryFuzzResult struct {
	Original       *db.History
	Result         *db.History
	Response       http.Response
	ResponseData   http_utils.FullResponseData
	Err            error
	Payload        generation.Payload
	InsertionPoint InsertionPoint
	Duration       time.Duration
}

type HttpFuzzer struct {
	Concurrency         int
	InteractionsManager *integrations.InteractionsManager
	client              *http.Client
}

type HttpFuzzerTask struct {
	history        *db.History
	insertionPoint InsertionPoint
	payload        generation.Payload
}

func (f *HttpFuzzer) checkConfig() {
	if f.Concurrency == 0 {
		log.Info().Interface("fuzzer", f).Msg("Concurrency is not set, setting 4 as default")
		f.Concurrency = 4
	}
	if f.client == nil {
		f.client = http_utils.CreateHttpClient()
	}
}

// Run starts the fuzzing job
func (f *HttpFuzzer) Run(history *db.History, payloadGenerators []*generation.PayloadGenerator, insertionPoints []InsertionPoint) {

	var wg sync.WaitGroup
	f.checkConfig()
	// Declare the channels
	pendingTasks := make(chan HttpFuzzerTask, f.Concurrency)
	defer close(pendingTasks)

	// Schedule workers
	for i := 0; i < f.Concurrency; i++ {
		go f.worker(&wg, pendingTasks)
	}

	for _, insertionPoint := range insertionPoints {
		log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msgf("Scanning insertion point: %s", insertionPoint)
		for _, generator := range payloadGenerators {
			payloads, err := generator.BuildPayloads(*f.InteractionsManager)
			if err != nil {
				log.Error().Err(err).Msg("Failed to build payloads")
				continue
			}
			for _, payload := range payloads {
				wg.Add(1)
				task := HttpFuzzerTask{
					history:        history,
					payload:        payload,
					insertionPoint: insertionPoint,
				}
				pendingTasks <- task
			}
		}
	}
	log.Debug().Msg("Waiting for all the fuzzing tasks to finish")
	wg.Wait()
	log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msg("Finished fuzzing history item")
}

// worker makes the request and processes the result
func (f *HttpFuzzer) worker(wg *sync.WaitGroup, pendingTasks chan HttpFuzzerTask) {
	for task := range pendingTasks {
		taskLog := log.With().Str("method", task.history.Method).Str("param", task.insertionPoint.Name).Str("payload", task.payload.Value).Str("url", task.history.URL).Logger()
		taskLog.Debug().Interface("task", task).Msg("New fuzzer task received by parameter worker")
		var result HistoryFuzzResult
		builders := []InsertionPointBuilder{
			InsertionPointBuilder{
				Point:   task.insertionPoint,
				Payload: task.payload.Value,
			},
		}

		req, err := CreateRequestFromInsertionPoints(task.history, builders)
		if err != nil {
			taskLog.Error().Err(err).Msg("Error building request from insertion points")
			result.Err = err
		} else {
			startTime := time.Now()
			response, err := http_utils.SendRequest(f.client, req)
			if err != nil {
				taskLog.Error().Err(err).Msg("Error making request")
			}
			responseData, err := http_utils.ReadFullResponse(response)
			if err != nil {
				taskLog.Error().Err(err).Msg("Error reading response body, skipping")
				continue
			}

			newHistory, err := http_utils.CreateHistoryFromHttpResponse(response, responseData, db.SourceScanner)
			taskLog.Debug().Str("rawrequest", string(newHistory.RawRequest)).Msg("Request from history created in http fuzzer")
			result.Duration = time.Since(startTime)
			result.Result = newHistory
			result.Err = err
			result.Response = *response
			result.Payload = task.payload
			result.InsertionPoint = task.insertionPoint
			result.Original = task.history
			result.ResponseData = responseData
			vulnerable, err := f.EvaluateResult(result)
			if err != nil {
				taskLog.Error().Err(err).Msg("Error evaluating result")
				continue
			}
			issueCode := db.IssueCode(task.payload.IssueCode)

			if task.payload.InteractionDomain.URL != "" {
				oobTest := db.OOBTest{
					Code:              issueCode,
					TestName:          "Fuzz Test",
					InteractionDomain: task.payload.InteractionDomain.URL,
					InteractionFullID: task.payload.InteractionDomain.ID,
					Target:            newHistory.URL,
					Payload:           task.payload.Value,
					HistoryID:         &newHistory.ID,
					InsertionPoint:    task.insertionPoint.String(),
				}
				db.Connection.CreateOOBTest(oobTest)
				taskLog.Debug().Interface("oobTest", oobTest).Msg("Created OOB Test")
			}

			if vulnerable {
				taskLog.Warn().Msg("Vulnerable")
				// Should handle the additional details and confidence
				db.CreateIssueFromHistoryAndTemplate(newHistory, issueCode, "", 50)
			}

		}

		wg.Done()
	}
}

func (f *HttpFuzzer) EvaluateResult(result HistoryFuzzResult) (bool, error) {
	// Iterate through payload detection methods
	vulnerable := false
	condition := result.Payload.DetectionCondition
	for _, detectionMethod := range result.Payload.DetectionMethods {
		// Evaluate the detection method
		detectionMethodResult, err := f.EvaluateDetectionMethod(result, detectionMethod)
		if err != nil {
			return false, err
		}
		if detectionMethodResult {
			if condition == generation.Or {
				return true, nil
			}
			vulnerable = true
		} else if condition == generation.And {
			return false, nil
		}

	}

	return vulnerable, nil
}

func (f *HttpFuzzer) EvaluateDetectionMethod(result HistoryFuzzResult, method generation.DetectionMethod) (bool, error) {
	switch m := method.GetMethod().(type) {
	case *generation.OOBInteractionDetectionMethod:
		log.Debug().Msg("OOB Interaction detection method not implemented yet")

	case *generation.ResponseConditionDetectionMethod:
		statusMatch := false
		containsMatch := false
		if m.StatusCode != 0 {
			if m.StatusCode == result.Result.StatusCode {
				statusMatch = true
			}
		} else {
			// If no status is defined, assume it's matched
			statusMatch = true
		}

		if m.Contains != "" {
			if strings.Contains(result.ResponseData.RawString, m.Contains) {
				containsMatch = true
			}
		} else {
			// If no contains is defined, assume it's matched
			containsMatch = true
		}
		return statusMatch && containsMatch, nil

	case *generation.ReflectionDetectionMethod:
		if strings.Contains(result.ResponseData.RawString, m.Value) {
			log.Info().Msg("Matched Reflection method")
			return true, nil
		}
		return false, nil
	case *generation.BrowserEventsDetectionMethod:
		log.Warn().Msg("Browser Events detection method not implemented yet")
		return false, nil
	case *generation.TimeBasedDetectionMethod:
		responseDuration := result.Duration * time.Second
		sleepInt, err := strconv.Atoi(m.Sleep)
		if err != nil {
			log.Error().Err(err).Msg("Error converting sleep string to int")
			return false, err
		}
		sleepDuration := time.Duration(sleepInt) * time.Second
		if responseDuration >= sleepDuration {
			log.Info().Msg("Matched Time Based method")
			return true, nil
		}
		return false, nil
	case *generation.ResponseCheckDetectionMethod:
		log.Warn().Msg("Response Check detection method not implemented yet")
		return false, nil
	}
	return false, nil
}
