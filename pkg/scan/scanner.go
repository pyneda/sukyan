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
	"github.com/pyneda/sukyan/pkg/scan/options"
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
	Mode                options.ScanMode
	MaxRetries          int
	client              *http.Client
	issuesFound         sync.Map
	results             sync.Map
}

type TemplateScannerTask struct {
	history        *db.History
	insertionPoint InsertionPoint
	payload        generation.Payload
	options        options.HistoryItemScanOptions
	retryCount     int
}

type DetectedIssue struct {
	code           db.IssueCode
	insertionPoint InsertionPoint
}

func (di DetectedIssue) String() string {
	return fmt.Sprintf("%s:%s", di.code, di.insertionPoint.String())
}

func (f *TemplateScanner) checkConfig() {
	if f.Concurrency == 0 {
		log.Info().Interface("scanner", f).Msg("Concurrency is not set, setting 4 as default")
		f.Concurrency = 4
	}
	if f.client == nil {
		f.client = http_utils.CreateHttpClient()
	}

	if f.Mode == "" {
		log.Info().Interface("scanner", f).Msg("Mode is not set, setting smart as default")
		f.Mode = options.ScanModeSmart
	}

	if f.MaxRetries == 0 {
		f.MaxRetries = DefaultMaxRetries
		log.Info().Int("max_retries", f.MaxRetries).Msg("MaxRetries not set, using default")
	}
}

// shouldLaunch checks if the generator should be launched according to the launch conditions
func (f *TemplateScanner) shouldLaunch(history *db.History, generator *generation.PayloadGenerator, insertionPoint InsertionPoint, options options.HistoryItemScanOptions) bool {
	if generator == nil || len(generator.Launch.Conditions) == 0 {
		return true
	}
	conditionsMet := 0
	for _, condition := range generator.Launch.Conditions {
		switch condition.Type {
		case generation.Platform:
			if condition.Value == "" {
				log.Debug().Msg("Platform condition has an empty value, skipping")
				continue
			}
			if lib.SliceContains(options.FingerprintTags, condition.Value) {
				log.Debug().Str("fingerprint_tag", condition.Value).Interface("fingerprints", options.Fingerprints).Interface("condition", condition).Msg("Platform condition met by fingerprint tag")
				conditionsMet++
			} else {
				platform := ParsePlatform(condition.Value)
				if platform.MatchesAnyFingerprint(options.Fingerprints) {
					log.Debug().Str("platform", condition.Value).Interface("fingerprints", options.Fingerprints).Interface("condition", condition).Msg("Platform condition met by fingerprint")
					conditionsMet++
				} else {
					log.Debug().Str("platform", condition.Value).Interface("fingerprints", options.Fingerprints).Interface("condition", condition).Msg("Platform condition not met")
				}
			}

		case generation.ScanMode:
			if condition.Value == options.Mode.String() {
				conditionsMet++
			}

		case generation.ParameterValueDataType:
			if condition.Value == string(insertionPoint.ValueType) {
				conditionsMet++
			}

		case generation.ParameterName:
			if lib.SliceContains(condition.ParameterNames, insertionPoint.Name) {
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
	WorkspaceID     uint             `json:"workspace_id" validate:"required,min=0"`
	TaskID          uint             `json:"task_id" validate:"required,min=0"`
	Mode            options.ScanMode `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	FingerprintTags []string         `json:"fingerprint_tags" validate:"omitempty,dive"`
}

// Run starts the fuzzing job
func (f *TemplateScanner) Run(history *db.History, payloadGenerators []*generation.PayloadGenerator, insertionPoints []InsertionPoint, options options.HistoryItemScanOptions) map[string][]TemplateScannerResult {

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
		log.Debug().Str("item", history.URL).Str("method", history.Method).Str("point", insertionPoint.String()).Int("ID", int(history.ID)).Msg("Scanning insertion point")
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
				log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Str("generator", generator.ID).Str("insertion_point", insertionPoint.String()).Msg("Skipping generator as it does not meet the launch conditions")
			}
		}
	}
	log.Debug().Msg("Waiting for all the template scanner tasks to finish")
	wg.Wait()
	totalIssues := 0
	resultsMap := make(map[string][]TemplateScannerResult)
	f.results.Range(func(key, value interface{}) bool {
		if code, ok := key.(string); ok {
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
			}.String())
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
			wg.Done()
			continue
		} else {
			var oobTest db.OOBTest
			if task.payload.InteractionDomain.URL != "" {
				oobTest, err = db.Connection().CreateOOBTest(db.OOBTest{
					Code:              db.IssueCode(task.payload.IssueCode),
					TestName:          "OOB Test",
					InteractionDomain: task.payload.InteractionDomain.URL,
					InteractionFullID: task.payload.InteractionDomain.ID,
					Target:            req.URL.String(),
					Payload:           task.payload.Value,
					InsertionPoint:    task.insertionPoint.String(),
					WorkspaceID:       &f.WorkspaceID,
					TaskID:            &task.options.TaskID,
					TaskJobID:         &task.options.TaskJobID,
				})
				if err != nil {
					taskLog.Error().Str("full_id", task.payload.InteractionDomain.ID).Str("interaction_domain", task.payload.InteractionDomain.URL).Err(err).Msg("Error creating OOB Test")
				} else {
					taskLog.Debug().Uint("id", oobTest.ID).Str("full_id", task.payload.InteractionDomain.ID).Str("interaction_domain", task.payload.InteractionDomain.URL).Msg("Created OOB Test")
				}
			}

			// Calculate timeout based on payload type
			timeout := f.calculateTimeoutForPayload(task.payload)

			// Execute request using the new API
			executionResult := http_utils.ExecuteRequestWithTimeout(req, timeout, http_utils.HistoryCreationOptions{
				Source:              db.SourceScanner,
				WorkspaceID:         f.WorkspaceID,
				TaskID:              task.options.TaskID,
				CreateNewBodyStream: false,
			})

			// Update result with execution details
			result.Duration = executionResult.Duration
			result.Original = task.history
			result.Payload = task.payload
			result.InsertionPoint = task.insertionPoint
			result.Err = executionResult.Err
			result.Result = executionResult.History

			if executionResult.Response != nil {
				result.Response = *executionResult.Response
				result.ResponseData = executionResult.ResponseData
			}

			// Handle errors (including timeouts)
			if executionResult.Err != nil {
				isTimeBased, _ := f.isTimeBasedPayload(task.payload)
				errorCategory := http_utils.CategorizeRequestError(executionResult.Err)
				shouldRetry, shouldEvaluate := f.shouldHandleError(executionResult.Err, executionResult.TimedOut, isTimeBased, errorCategory)

				taskLog.Error().
					Err(executionResult.Err).
					Bool("is_timeout", executionResult.TimedOut).
					Bool("is_time_based", isTimeBased).
					Str("error_category", errorCategory).
					Bool("should_retry", shouldRetry).
					Bool("should_evaluate", shouldEvaluate).
					Str("insertion_point", task.insertionPoint.Name).
					Str("payload_type", task.payload.IssueCode).
					Str("payload_value", task.payload.Value).
					Int("retry_count", task.retryCount).
					Dur("duration", result.Duration).
					Dur("timeout", timeout).
					Msg("Error making request")

				// retry logic for recoverable errors
				if shouldRetry && task.retryCount < f.MaxRetries {
					taskLog.Info().
						Int("retry_count", task.retryCount).
						Int("max_retries", f.MaxRetries).
						Str("error_category", errorCategory).
						Msg("Retrying request due to recoverable error")

					task.retryCount++
					time.Sleep(time.Duration(task.retryCount) * RetryDelayBase) // Progressive delay
					pendingTasks <- task
					// Don't call wg.Done() here - the original wg.Add(1) should only be balanced once when the task truly completes
					continue
				}

				if !shouldEvaluate {
					wg.Done()
					continue
				}
			}

			// Update OOB test history ID if needed
			if task.payload.InteractionDomain.URL != "" && result.Result != nil && result.Result.ID != 0 {
				db.Connection().UpdateOOBTestHistoryID(oobTest.ID, &result.Result.ID)
			}

			if result.Result != nil {
				taskLog.Debug().Str("rawrequest", string(result.Result.RawRequest)).Msg("Request from history created in TemplateScanner")
			}

			// Evaluate the result for vulnerabilities
			vulnerable, details, confidence, issueOverride, err := f.EvaluateResult(result)

			if err != nil {
				taskLog.Error().Err(err).Msg("Error evaluating result")
				wg.Done()
				continue
			}
			issueCode := db.IssueCode(task.payload.IssueCode)
			if issueOverride != "" {
				issueCode = issueOverride
				details = fmt.Sprintf("%s\n\n This issue has been detected looking for %s, but matched a response condition of %s and has been overriden", details, task.payload.IssueCode, issueOverride)
			}

			if vulnerable {
				taskLog.Warn().Msg("Vulnerable")
				// Should handle the additional details and confidence
				var historyForIssue *db.History
				var fullDetails string

				if result.Err != nil && http_utils.IsTimeoutError(result.Err) {
					historyForIssue = result.Result
					fullDetails = fmt.Sprintf("The following payload was inserted in the `%s` %s: %s\n\nRequest timed out after %s.\n\n%s",
						task.insertionPoint.Name, task.insertionPoint.Type, task.payload.Value, result.Duration, details)
				} else {
					historyForIssue = result.Result
					fullDetails = fmt.Sprintf("The following payload was inserted in the `%s` %s: %s\n\n%s", task.insertionPoint.Name, task.insertionPoint.Type, task.payload.Value, details)
				}

				createdIssue, err := db.CreateIssueFromHistoryAndTemplate(historyForIssue, issueCode, fullDetails, confidence, "", &f.WorkspaceID, &task.options.TaskID, &task.options.TaskJobID)
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
					}.String(), true)
				}
			}

		}

		wg.Done()
	}
}

func (f *TemplateScanner) EvaluateResult(result TemplateScannerResult) (bool, string, int, db.IssueCode, error) {
	// Iterate through payload detection methods
	vulnerable := false
	condition := result.Payload.DetectionCondition
	confidence := 0
	var sb strings.Builder
	var issueOverride db.IssueCode

	for _, detectionMethod := range result.Payload.DetectionMethods {
		// Evaluate the detection method
		detectionMethodResult, description, conf, override, err := f.EvaluateDetectionMethod(result, detectionMethod)
		if conf > confidence {
			confidence = conf
		}
		if description != "" {
			sb.WriteString(description + "\n")
		}

		if err != nil {
			return false, "", confidence, "", err
		}

		if detectionMethodResult {
			// Not returning as we want the details of all detection methods
			// if condition == generation.Or {
			// 	return true, sb.String(), confidence, nil
			// }
			vulnerable = true
			if override != "" {
				issueOverride = override
			}
		} else if condition == generation.And {
			return false, "", confidence, "", nil
		}

	}

	return vulnerable, sb.String(), confidence, issueOverride, nil
}

type repeatedHistoryItem struct {
	history  *db.History
	duration time.Duration
	timeout  bool
}

// repeatHistoryItem repeats a history item and returns the new history item and the duration
func (f *TemplateScanner) repeatHistoryItem(history *db.History, timeout time.Duration) (repeatedHistoryItem, error) {
	request, err := http_utils.BuildRequestFromHistoryItem(history)
	if err != nil {
		log.Error().Err(err).Msg("Error building original request from history item to revalidate time based issue")
		return repeatedHistoryItem{}, err
	}

	// Execute request using the new API
	executionResult := http_utils.ExecuteRequestWithTimeout(request, timeout, http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         f.WorkspaceID,
		TaskID:              0, // TODO: Should pass the task id here
		CreateNewBodyStream: false,
	})

	if executionResult.Err != nil {
		log.Error().
			Err(executionResult.Err).
			Bool("is_timeout", executionResult.TimedOut).
			Dur("timeout", timeout).
			Dur("actual_duration", executionResult.Duration).
			Msg("Error making request during revalidation")

		return repeatedHistoryItem{
			timeout:  executionResult.TimedOut,
			duration: executionResult.Duration,
		}, executionResult.Err
	}

	return repeatedHistoryItem{
		history:  executionResult.History,
		duration: executionResult.Duration,
		timeout:  false,
	}, nil
}

// EvaluateDetectionMethod evaluates a detection method and returns a boolean indicating if it matched, a description of the match, the confidence and a possible error
func (f *TemplateScanner) EvaluateDetectionMethod(result TemplateScannerResult, method generation.DetectionMethod) (bool, string, int, db.IssueCode, error) {
	switch m := method.GetMethod().(type) {
	case *generation.OOBInteractionDetectionMethod:
		log.Debug().Msg("OOB Interaction detection method not implemented yet")

	case *generation.ResponseConditionDetectionMethod:
		statusMatch := false
		containsMatch := false
		var sb strings.Builder
		if m.StatusCode != 0 {
			if m.StatusCode == result.Result.StatusCode {
				if m.StatusCodeShouldChange && result.Original.StatusCode == result.Result.StatusCode {
					// If the status code should change and it didn't, it's not a match
					// Main reason is to avoid false positives
					statusMatch = false
				} else {
					sb.WriteString(fmt.Sprintf("Response status code is %d\n", m.StatusCode))
					sb.WriteString(fmt.Sprintf("Original status code is %d\n", result.Original.StatusCode))
					statusMatch = true
				}
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

		return matched, sb.String(), confidence, m.IssueOverride, nil

	case *generation.ReflectionDetectionMethod:
		if strings.Contains(result.ResponseData.RawString, m.Value) {
			log.Info().Msg("Matched Reflection method")
			description := fmt.Sprintf("Response contains the value %s", m.Value)
			return true, description, m.Confidence, "", nil
		}
		return false, "", 0, "", nil
	case *generation.BrowserEventsDetectionMethod:
		log.Warn().Msg("Browser Events detection method not implemented yet")
		return false, "", 0, "", nil
	case *generation.TimeBasedDetectionMethod:
		durationIsHigher := m.CheckIfResultDurationIsHigher(result.Duration)
		requestTimedOut := result.Err != nil && http_utils.IsTimeoutError(result.Err)

		if durationIsHigher || requestTimedOut {
			var sb strings.Builder
			defaultDelay := 30
			attempts := 8
			finalConfidence := m.Confidence
			confidenceIncrement := 20
			confidenceDecrement := 40
			originalTrueCount := 0
			payloadTrueCount := 0

			if requestTimedOut {
				sb.WriteString(fmt.Sprintf("Request timed out after %s, which may indicate the sleep payload of %s worked\n\n", result.Duration, m.Sleep))
			} else {
				sb.WriteString(fmt.Sprintf("Response took %s, which is greater than the sleep time injected in the payload of %s\n\n", result.Duration, m.Sleep))
			}
			// var originalResults []bool
			// var payloadResults []bool
			expectedSleepDuratoin := m.ParseSleepDuration(m.Sleep)
			sb.WriteString("Revalidation results:\n")
			sb.WriteString("=============================\n")

			for i := 1; i < attempts; i++ {

				delay := time.Duration(defaultDelay*i) * time.Second

				// For revalidation, use a timeout that's longer than the expected sleep duration but still reasonable to prevent hanging
				revalidationTimeout := expectedSleepDuratoin + 2*time.Minute
				if revalidationTimeout < 1*time.Minute {
					revalidationTimeout = 1 * time.Minute
				}
				if revalidationTimeout > 5*time.Minute {
					revalidationTimeout = 5 * time.Minute
				}

				originalResult, err := f.repeatHistoryItem(result.Original, revalidationTimeout)
				if err != nil {
					if http_utils.IsTimeoutError(err) {
						// NOTE: If original times out, it might indicate network issues, so we continue but note it
						sb.WriteString(fmt.Sprintf("Attempt %d: Original request timed out after %s (timeout: %s)\n", i, originalResult.duration, revalidationTimeout))
						finalConfidence -= confidenceDecrement
					} else {
						sb.WriteString(fmt.Sprintf("Attempt %d: Error making request for original history item: %s\n", i, err.Error()))
					}
					sb.WriteString(fmt.Sprintf(" * Sleeping for %s seconds.\n", delay))
					time.Sleep(delay)
					continue
				}
				withPayloadResult, err := f.repeatHistoryItem(result.Result, revalidationTimeout)
				if err != nil {
					if http_utils.IsTimeoutError(err) {
						sb.WriteString(fmt.Sprintf("Attempt %d: Payload request timed out after %s (timeout: %s)\n", i, withPayloadResult.duration, revalidationTimeout))
						if !originalResult.timeout {
							payloadTrueCount++
							finalConfidence += confidenceIncrement
							sb.WriteString(" * Payload request timed out while original didn't\n")
						}
					} else {
						sb.WriteString(fmt.Sprintf("Attempt %d: Error making request for history item with payload: %s\n", i, err.Error()))
					}
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

				if originalResult.duration > withPayloadResult.duration || withPayloadResult.duration < expectedSleepDuratoin {
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
				return true, sb.String(), 100, "", nil
			}

			if finalConfidence > 50 {
				log.Debug().Msgf("System is vulnerable with %d%% confidence", finalConfidence)
				return true, sb.String(), finalConfidence, "", nil
			} else {
				log.Debug().Msgf("System is not vulnerable with %d%% confidence", finalConfidence)
				return false, "", finalConfidence, "", nil
			}

		}
		return false, "", 0, "", nil
	case *generation.ResponseCheckDetectionMethod:
		switch m.Check {
		case generation.DatabaseErrorCondition:
			result := passive.SearchDatabaseErrors(result.ResponseData.RawString)
			if result != nil {
				log.Info().Interface("database_error", result).Msg("Matched DatabaseErrorCondition")
				description := fmt.Sprintf("Database error was returned in response:\n - Database: %s\n - Error: %s", result.DatabaseName, result.MatchStr)
				return true, description, m.Confidence, m.IssueOverride, nil
			}
		case generation.XPathErrorCondition:
			result := passive.SearchXPathErrors(result.ResponseData.RawString)
			if result != "" {
				log.Info().Str("xpath_error", result).Msg("Matched XPathErrorCondition")
				description := fmt.Sprintf("XPath error was returned in response:\n - Error: %s", result)
				return true, description, m.Confidence, m.IssueOverride, nil
			}
		}
		return false, "", 0, m.IssueOverride, nil
	}
	return false, "", 0, "", nil
}

// isTimeBasedPayload checks if a payload contains time-based detection methods
// and returns the expected sleep duration if found
func (f *TemplateScanner) isTimeBasedPayload(payload generation.Payload) (bool, time.Duration) {
	for _, method := range payload.DetectionMethods {
		if timeBased := method.TimeBased; timeBased != nil {
			expectedDuration := timeBased.ParseSleepDuration(timeBased.Sleep)
			if expectedDuration > 0 {
				return true, expectedDuration
			}
		}
	}
	return false, 0
}

// calculateTimeoutForPayload calculates an appropriate timeout for a payload
func (f *TemplateScanner) calculateTimeoutForPayload(payload generation.Payload) time.Duration {
	isTimeBased, expectedDuration := f.isTimeBasedPayload(payload)

	if isTimeBased {
		// For time-based payloads, add buffer to expected sleep duration
		timeout := time.Duration(float64(expectedDuration) * 2.0)

		// Set reasonable bounds: minimum 30s, maximum 5 minutes
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}
		if timeout > 5*time.Minute {
			timeout = 5 * time.Minute
		}

		log.Debug().
			Dur("expected_sleep", expectedDuration).
			Dur("calculated_timeout", timeout).
			Str("payload", payload.Value).
			Msg("Calculated timeout for time-based payload")

		return timeout
	}

	return 2 * time.Minute
}

// shouldHandleError determines if an error should be retried or evaluated
func (f *TemplateScanner) shouldHandleError(err error, isTimeout bool, isTimeBased bool, errorCategory string) (shouldRetry bool, shouldEvaluate bool) {
	if isTimeBased && isTimeout {
		return false, true // Don't retry, but evaluate for time-based vulnerabilities
	}

	if isTimeout && !isTimeBased {
		return true, false // Retry timeouts for non-time-based tests
	}

	// Categorize other errors
	switch errorCategory {
	case http_utils.ErrorCategoryConnectionClosedEOF, http_utils.ErrorCategoryConnectionReset, http_utils.ErrorCategoryConnectionBrokenPipe:
		return true, false

	case http_utils.ErrorCategoryConnectionRefused, http_utils.ErrorCategoryNetworkUnreachable, http_utils.ErrorCategoryHostUnreachable, http_utils.ErrorCategoryDNSResolution:
		return false, false

	case http_utils.ErrorCategoryTLSError, http_utils.ErrorCategoryCertificateError, http_utils.ErrorCategoryProtocolError, http_utils.ErrorCategoryMalformedResponse:
		return false, false

	case http_utils.ErrorCategoryURLControlCharacter, http_utils.ErrorCategoryURLInvalid:
		return false, false

	case http_utils.ErrorCategoryServerError:
		return true, false

	default:
		return false, false
	}
}

// Error handling constants
const (
	DefaultMaxRetries = 2
	RetryDelayBase    = 10 * time.Second
)
