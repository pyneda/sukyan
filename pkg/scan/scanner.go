package scan

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
)

type TemplateScannerResult struct {
	Original       *db.History
	Result         *db.History
	Response       http.Response
	ResponseData   http_utils.FullResponseData
	Err            error
	Payload        generation.Payload
	InsertionPoint InsertionPoint
	Duration       time.Duration
	Issue          *db.Issue
}

type TemplateScanner struct {
	Concurrency         int
	InteractionsManager *integrations.InteractionsManager
	AvoidRepeatedIssues bool
	WorkspaceID         uint
	client              *http.Client
	issuesFound         sync.Map
	results             sync.Map
}

type TemplateScannerTask struct {
	history        *db.History
	insertionPoint InsertionPoint
	payload        generation.Payload
	options        HistoryItemScanOptions
}

type DetectedIssue struct {
	code           db.IssueCode
	insertionPoint InsertionPoint
}

func (f *TemplateScanner) checkConfig() {
	if f.Concurrency == 0 {
		log.Info().Interface("scanner", f).Msg("Concurrency is not set, setting 4 as default")
		f.Concurrency = 4
	}
	if f.client == nil {
		f.client = http_utils.CreateHttpClient()
	}

}

// shouldLaunch checks if the generator should be launched according to the launch conditions
func (f *TemplateScanner) shouldLaunch(history *db.History, generator *generation.PayloadGenerator, insertionPoint InsertionPoint, options HistoryItemScanOptions) bool {
	if generator.Launch.Conditions == nil || len(generator.Launch.Conditions) == 0 {
		return true
	}
	conditionsMet := 0
	for _, condition := range generator.Launch.Conditions {
		switch condition.Type {
		case generation.Platform:
			if lib.SliceContains(options.FingerprintTags, condition.Value) {
				conditionsMet++
			}

		case generation.ScanMode:
			if condition.Value == options.Mode.String() {
				conditionsMet++
			}

		case generation.ParameterValueDataType:
			if condition.Value == string(insertionPoint.ValueType) {
				conditionsMet++
			}

		case generation.ResponseCondition:
			if condition.ResponseCondition.Check(history) {
				conditionsMet++
			}
		}
	}

	if generator.Launch.Operator == generation.Or {
		return conditionsMet > 0
	}

	return conditionsMet == len(generator.Launch.Conditions)
}

type FuzzItemOptions struct {
	WorkspaceID     uint     `json:"workspace_id" validate:"required,min=0"`
	TaskID          uint     `json:"task_id" validate:"required,min=0"`
	Mode            ScanMode `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	FingerprintTags []string `json:"fingerprint_tags" validate:"omitempty,dive"`
}

// Run starts the fuzzing job
func (f *TemplateScanner) Run(history *db.History, payloadGenerators []*generation.PayloadGenerator, insertionPoints []InsertionPoint, options HistoryItemScanOptions) map[db.IssueCode][]TemplateScannerResult {

	var wg sync.WaitGroup
	f.checkConfig()
	// Declare the channels
	pendingTasks := make(chan TemplateScannerTask, f.Concurrency)
	defer close(pendingTasks)

	// Schedule workers
	for i := 0; i < f.Concurrency; i++ {
		go f.worker(&wg, pendingTasks)
	}

	for _, insertionPoint := range insertionPoints {
		log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msgf("Scanning insertion point: %s", insertionPoint)
		for _, generator := range payloadGenerators {
			if f.shouldLaunch(history, generator, insertionPoint, options) {
				payloads, err := generator.BuildPayloads(*f.InteractionsManager)
				if err != nil {
					log.Error().Err(err).Interface("generator", generator).Msg("Failed to build payloads")
					continue
				}
				for _, payload := range payloads {
					wg.Add(1)
					task := TemplateScannerTask{
						history:        history,
						payload:        payload,
						insertionPoint: insertionPoint,
						options:        options,
					}
					pendingTasks <- task
				}
			} else {
				log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Str("insertion_point", insertionPoint.String()).Msgf("Skipping generator %s as it does not meet the launch conditions", generator.ID)
			}
		}
	}
	log.Debug().Msg("Waiting for all the template scanner tasks to finish")
	wg.Wait()
	totalIssues := 0
	resultsMap := make(map[db.IssueCode][]TemplateScannerResult)
	f.results.Range(func(key, value interface{}) bool {
		if code, ok := key.(db.IssueCode); ok {
			if result, ok := value.(TemplateScannerResult); ok {
				if _, exists := resultsMap[code]; !exists {
					resultsMap[code] = make([]TemplateScannerResult, 0)
				}
				resultsMap[code] = append(resultsMap[code], result)
				totalIssues++
			}
		}
		return true
	})
	log.Info().Int("total_issues", totalIssues).Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msg("Finished running template scanner against history item")

	return resultsMap
}

// worker makes the request and processes the result
func (f *TemplateScanner) worker(wg *sync.WaitGroup, pendingTasks chan TemplateScannerTask) {
	for task := range pendingTasks {
		taskLog := log.With().Str("method", task.history.Method).Str("param", task.insertionPoint.Name).Str("payload", task.payload.Value).Str("url", task.history.URL).Logger()
		taskLog.Debug().Interface("task", task).Msg("New template scanner task received by parameter worker")
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
		var result TemplateScannerResult
		builders := []InsertionPointBuilder{
			{
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
				wg.Done()
				continue
			}
			responseData, _, err := http_utils.ReadFullResponse(response, false)
			if err != nil {
				taskLog.Error().Err(err).Msg("Error reading response body, skipping")
				wg.Done()
				continue
			}
			result.Duration = time.Since(startTime)
			options := http_utils.HistoryCreationOptions{
				Source:              db.SourceScanner,
				WorkspaceID:         f.WorkspaceID,
				TaskID:              task.options.TaskID,
				CreateNewBodyStream: false,
			}
			newHistory, err := http_utils.CreateHistoryFromHttpResponse(response, responseData, options)
			taskLog.Debug().Str("rawrequest", string(newHistory.RawRequest)).Msg("Request from history created in TemplateScanner")
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
				wg.Done()
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
					WorkspaceID:       &f.WorkspaceID,
					TaskID:            &task.options.TaskID,
					TaskJobID:         &task.options.TaskJobID,
				}
				db.Connection.CreateOOBTest(oobTest)
				taskLog.Debug().Interface("oobTest", oobTest).Msg("Created OOB Test")
			}

			if vulnerable {
				taskLog.Warn().Msg("Vulnerable")
				// Should handle the additional details and confidence
				fullDetails := fmt.Sprintf("The following payload was inserted in the `%s` %s: %s\n\n%s", task.insertionPoint.Name, task.insertionPoint.Type, task.payload.Value, details)
				// taskLog.Warn().Interface("newHistory", newHistory).Str("issue", string(issueCode)).Str("details", fullDetails).Int("confidence", confidence).Uint("wksp", f.WorkspaceID).Msg("Creating issue")
				createdIssue, err := db.CreateIssueFromHistoryAndTemplate(newHistory, issueCode, fullDetails, confidence, "", &f.WorkspaceID, &task.options.TaskID, &task.options.TaskJobID)
				if err != nil {
					taskLog.Error().Str("code", string(issueCode)).Interface("result", result).Err(err).Msg("Error creating issue")
				} else if createdIssue.ID != 0 {
					result.Issue = &createdIssue
					f.results.Store(createdIssue.Code, result)
				}
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

func (f *TemplateScanner) EvaluateResult(result TemplateScannerResult) (bool, string, int, error) {
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

type repeatedHistoryItem struct {
	history  *db.History
	duration time.Duration
}

// repeatHistoryItem repeats a history item and returns the new history item and the duration
func (f *TemplateScanner) repeatHistoryItem(history *db.History) (repeatedHistoryItem, error) {
	request, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		log.Error().Err(err).Msg("Error building original request from history item to revalidate time based issue")
		return repeatedHistoryItem{}, err
	}
	startTime := time.Now()
	response, err := http_utils.SendRequest(f.client, request)
	if err != nil {
		log.Error().Err(err).Msg("Error making request")
		return repeatedHistoryItem{}, err
	}
	responseData, _, err := http_utils.ReadFullResponse(response, false)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body, skipping")
		return repeatedHistoryItem{}, err
	}
	duration := time.Since(startTime)

	options := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         f.WorkspaceID,
		TaskID:              0, // TODO: Should pass the task id here
		CreateNewBodyStream: false,
	}
	newHistory, _ := http_utils.CreateHistoryFromHttpResponse(response, responseData, options)
	return repeatedHistoryItem{
		history:  newHistory,
		duration: duration,
	}, nil
}

// EvaluateDetectionMethod evaluates a detection method and returns a boolean indicating if it matched, a description of the match, the confidence and a possible error
func (f *TemplateScanner) EvaluateDetectionMethod(result TemplateScannerResult, method generation.DetectionMethod) (bool, string, int, error) {
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
		if m.CheckIfResultDurationIsHigher(result.Duration) {
			var sb strings.Builder
			defaultDelay := 30
			attempts := 8
			finalConfidence := m.Confidence
			confidenceIncrement := 20
			confidenceDecrement := 25
			originalTrueCount := 0
			payloadTrueCount := 0
			sb.WriteString(fmt.Sprintf("Response took %s, which is greater than the sleep time injected in the payload of %s\n\n", result.Duration, m.Sleep))
			// var originalResults []bool
			// var payloadResults []bool

			sb.WriteString("Revalidation results:\n")
			sb.WriteString("=============================\n")

			for i := 1; i < attempts; i++ {

				delay := time.Duration(defaultDelay*i) * time.Second

				originalResult, err := f.repeatHistoryItem(result.Original)
				if err != nil {
					sb.WriteString(fmt.Sprintf("Attempt %d: Error making request for original history item\n", i))
					sb.WriteString(fmt.Sprintf(" * Sleeping for %s seconds.\n", delay))
					time.Sleep(delay)
					continue
				}
				withPayloadResult, err := f.repeatHistoryItem(result.Result)
				if err != nil {
					sb.WriteString(fmt.Sprintf("Attempt %d: Error making request for history item with payload\n", i))
					sb.WriteString(fmt.Sprintf(" * Sleeping for %s seconds.\n", delay))
					time.Sleep(delay)
					continue
				}
				originalIsHigher := m.CheckIfResultDurationIsHigher(originalResult.duration)
				if originalIsHigher {
					originalTrueCount++
					finalConfidence -= confidenceDecrement
				}
				withPayloadIsHigher := m.CheckIfResultDurationIsHigher(withPayloadResult.duration)
				if withPayloadIsHigher {
					payloadTrueCount++
					finalConfidence += confidenceIncrement
				}

				if originalResult.duration > withPayloadResult.duration {
					finalConfidence -= confidenceDecrement
				}
				// originalResults = append(originalResults, originalIsHigher)
				// payloadResults = append(payloadResults, withPayloadIsHigher)
				sb.WriteString(fmt.Sprintf("Attempt %d:\n - Original took %s\n - With payload took %s\n\n", i, originalResult.duration, withPayloadResult.duration))
				if originalIsHigher {
					sb.WriteString(fmt.Sprintf(" * Sleeping for %s seconds.\n", delay))
					log.Debug().Msg("While revalidating time based issue, both the original and the payload requests took longer than the sleep time. Sleeping for 30 seconds and trying again")
					time.Sleep(delay)
				}
			}

			if finalConfidence > 100 {
				finalConfidence = 100
			} else if finalConfidence < 0 {
				finalConfidence = 0
			}

			if originalTrueCount == 0 && payloadTrueCount > attempts/2 {
				return true, sb.String(), 100, nil
			}

			if finalConfidence > 50 {
				log.Debug().Msgf("System is vulnerable with %d%% confidence", finalConfidence)
				return true, sb.String(), finalConfidence, nil
			} else {
				log.Debug().Msgf("System is not vulnerable with %d%% confidence", finalConfidence)
				return false, "", finalConfidence, nil
			}

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
