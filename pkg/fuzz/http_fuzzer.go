package fuzz

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
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
	AvoidRepeatedIssues bool
	WorkspaceID         uint
	client              *http.Client
	issuesFound         sync.Map
}

type HttpFuzzerTask struct {
	history        *db.History
	insertionPoint InsertionPoint
	payload        generation.Payload
}

type DetectedIssue struct {
	code           db.IssueCode
	insertionPoint InsertionPoint
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
				log.Error().Err(err).Interface("generator", generator).Msg("Failed to build payloads")
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
		if f.AvoidRepeatedIssues {
			_, ok := f.issuesFound.Load(DetectedIssue{
				code:           db.IssueCode(task.payload.IssueCode),
				insertionPoint: task.insertionPoint,
			})
			if ok {
				taskLog.Debug().Msg("Skipping task as an issue for this insertion point with this code for this history item has already been found")
				wg.Done()
				continue
			}
		}
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

			newHistory, err := http_utils.CreateHistoryFromHttpResponse(response, responseData, db.SourceScanner, f.WorkspaceID)
			taskLog.Debug().Str("rawrequest", string(newHistory.RawRequest)).Msg("Request from history created in http fuzzer")
			result.Duration = time.Since(startTime)
			result.Result = newHistory
			result.Err = err
			result.Response = *response
			result.Payload = task.payload
			result.InsertionPoint = task.insertionPoint
			result.Original = task.history
			result.ResponseData = responseData
			vulnerable, details, confidence, err := f.EvaluateResult(result)
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
				fullDetails := fmt.Sprintf("The following payload was inserted in the `%s` %s: %s\n\n%s", task.insertionPoint.Name, task.insertionPoint.Type, task.payload.Value, details)
				db.CreateIssueFromHistoryAndTemplate(newHistory, issueCode, fullDetails, confidence, "", &f.WorkspaceID)
				// Avoid repeated issues: could also provide a issue type `variant` and handle the insertion point
				if f.AvoidRepeatedIssues {
					f.issuesFound.Store(DetectedIssue{
						code:           db.IssueCode(issueCode),
						insertionPoint: task.insertionPoint,
					}, true)
				}
			}

		}

		wg.Done()
	}
}

func (f *HttpFuzzer) EvaluateResult(result HistoryFuzzResult) (bool, string, int, error) {
	// Iterate through payload detection methods
	vulnerable := false
	condition := result.Payload.DetectionCondition
	confidence := 0
	var sb strings.Builder
	for _, detectionMethod := range result.Payload.DetectionMethods {
		// Evaluate the detection method
		detectionMethodResult, description, conf, err := f.EvaluateDetectionMethod(result, detectionMethod)
		if conf > confidence {
			confidence = conf
		}
		if description != "" {
			sb.WriteString(description + "\n")
		}
		if err != nil {
			return false, "", confidence, err
		}

		if detectionMethodResult {
			// Not returning as we want the details of all detection methods
			// if condition == generation.Or {
			// 	return true, sb.String(), confidence, nil
			// }
			vulnerable = true
		} else if condition == generation.And {
			return false, "", confidence, nil
		}

	}

	return vulnerable, sb.String(), confidence, nil
}

// EvaluateDetectionMethod evaluates a detection method and returns a boolean indicating if it matched, a description of the match, the confidence and a possible error
func (f *HttpFuzzer) EvaluateDetectionMethod(result HistoryFuzzResult, method generation.DetectionMethod) (bool, string, int, error) {
	switch m := method.GetMethod().(type) {
	case *generation.OOBInteractionDetectionMethod:
		log.Debug().Msg("OOB Interaction detection method not implemented yet")

	case *generation.ResponseConditionDetectionMethod:
		statusMatch := false
		containsMatch := false
		var sb strings.Builder
		if m.StatusCode != 0 {
			if m.StatusCode == result.Result.StatusCode {
				sb.WriteString(fmt.Sprintf("Response status code is %d\n", m.StatusCode))
				statusMatch = true
			}
		} else {
			// If no status is defined, assume it's matched
			statusMatch = true
		}

		if m.Contains != "" {
			matchAgainst := result.ResponseData.RawString
			if m.Part == generation.Body {
				matchAgainst = string(result.ResponseData.Body)
			} else if m.Part == generation.Headers {
				headersString, err := result.Result.GetResponseHeadersAsString()
				if err == nil {
					matchAgainst = headersString
				} else {
					log.Error().Err(err).Msg("Error getting response headers as string, using raw response. This might create false positives.")
				}
			}
			if strings.Contains(matchAgainst, m.Contains) {
				sb.WriteString(fmt.Sprintf("Response contains the value: %s\n", m.Contains))
				containsMatch = true
			}
		} else {
			// If no contains is defined, assume it's matched
			containsMatch = true
		}
		confidence := 0
		matched := statusMatch && containsMatch
		if matched {
			confidence = m.Confidence
		}

		return matched, sb.String(), confidence, nil

	case *generation.ReflectionDetectionMethod:
		if strings.Contains(result.ResponseData.RawString, m.Value) {
			log.Info().Msg("Matched Reflection method")
			description := fmt.Sprintf("Response contains the value %s", m.Value)
			return true, description, m.Confidence, nil
		}
		return false, "", 0, nil
	case *generation.BrowserEventsDetectionMethod:
		log.Warn().Msg("Browser Events detection method not implemented yet")
		return false, "", 0, nil
	case *generation.TimeBasedDetectionMethod:
		sleepInt, err := strconv.Atoi(m.Sleep)
		if err != nil {
			log.Error().Err(err).Str("sleep", m.Sleep).Interface("result", result).Msg("Error converting sleep string to int")
			return false, "", 0, err
		}
		// TODO: Improve this, the units should probably be defined in the templates
		var sleepDuration time.Duration
		var unit string
		if sleepInt > 1000 {
			sleepDuration = time.Duration(sleepInt) * time.Millisecond
			unit = "ms"
		} else {
			sleepDuration = time.Duration(sleepInt) * time.Second
			unit = "s"
		}

		if result.Duration >= sleepDuration {
			log.Info().Str("duration", result.Duration.String()).Str("sleep", sleepDuration.String()).Str("unit", unit).Msg("Matched Time Based method")
			description := fmt.Sprintf("Response took %s, which is greater than the sleep time injected in the payload of %s", result.Duration, sleepDuration)
			return true, description, m.Confidence, nil
		}
		return false, "", 0, nil
	case *generation.ResponseCheckDetectionMethod:
		if m.Check == generation.DatabaseErrorCondition {
			result := passive.SearchDatabaseErrors(result.ResponseData.RawString)
			if result != nil {
				log.Info().Interface("database_error", result).Msg("Matched DatabaseErrorCondition")
				description := fmt.Sprintf("Database error was returned in response:\n - Database: %s\n - Error: %s", result.DatabaseName, result.MatchStr)
				return true, description, m.Confidence, nil
			}
		} else if m.Check == generation.XPathErrorCondition {
			result := passive.SearchXPathErrors(result.ResponseData.RawString)
			if result != "" {
				log.Info().Str("xpath_error", result).Msg("Matched XPathErrorCondition")
				description := fmt.Sprintf("XPath error was returned in response:\n - Error: %s", result)
				return true, description, m.Confidence, nil
			}
		}
		return false, "", 0, nil
	}
	return false, "", 0, nil
}
