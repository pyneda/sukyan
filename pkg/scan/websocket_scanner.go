package scan

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

type WebSocketScanner struct {
	Concurrency         int
	InteractionsManager *integrations.InteractionsManager
	AvoidRepeatedIssues bool
	WorkspaceID         uint
	issuesFound         sync.Map
	results             sync.Map
}

type WebSocketScannerResult struct {
	Original *db.WebSocketMessage
	Result   *db.WebSocketMessage
	Err      error
	Payload  generation.Payload
	Duration time.Duration
	Issue    *db.Issue
}

type WebSocketScannerTask struct {
	message        *db.WebSocketMessage
	payload        generation.Payload
	insertionPoint InsertionPoint
	options        options.HistoryItemScanOptions
}

func (f *WebSocketScanner) checkConfig() {
	if f.Concurrency == 0 {
		log.Info().Interface("scanner", f).Msg("Concurrency is not set, setting 4 as default")
		f.Concurrency = 4
	}
}

// shouldLaunch checks if the generator should be launched according to the launch conditions
func (f *WebSocketScanner) shouldLaunch(message *db.WebSocketMessage, generator *generation.PayloadGenerator, insertionPoint InsertionPoint, options options.HistoryItemScanOptions) bool {
	if generator.Launch.Conditions == nil || len(generator.Launch.Conditions) == 0 {
		return true
	}
	conditionsMet := 0
	for _, condition := range generator.Launch.Conditions {
		switch condition.Type {
		case generation.Platform:
			if lib.SliceContains(options.FingerprintTags, condition.Value) {
				conditionsMet++
			} else {
				platform := ParsePlatform(condition.Value)
				if platform.MatchesAnyFingerprint(options.Fingerprints) {
					conditionsMet++
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
			if condition.ResponseCondition.CheckWebsocketMessage(message) {
				conditionsMet++
			}
		}
	}

	if generator.Launch.Operator == generation.Or {
		return conditionsMet > 0
	}

	return conditionsMet == len(generator.Launch.Conditions)
}

func (f *WebSocketScanner) Run(message *db.WebSocketMessage, payloadGenerators []*generation.PayloadGenerator, insertionPoints []InsertionPoint, options options.HistoryItemScanOptions) map[string][]WebSocketScannerResult {

	var wg sync.WaitGroup
	f.checkConfig()
	// Declare the channels
	pendingTasks := make(chan WebSocketScannerTask, f.Concurrency)
	defer close(pendingTasks)

	// Schedule workers
	for i := 0; i < f.Concurrency; i++ {
		go f.worker(&wg, pendingTasks)
	}
	for _, insertionPoint := range insertionPoints {
		for _, generator := range payloadGenerators {
			if f.shouldLaunch(message, generator, insertionPoint, options) {
				payloads, err := generator.BuildPayloads(*f.InteractionsManager)
				if err != nil {
					log.Error().Err(err).Interface("generator", generator).Msg("Failed to build payloads")
					continue
				}
				for _, payload := range payloads {
					wg.Add(1)
					task := WebSocketScannerTask{
						message: message,
						payload: payload,
						options: options,
					}
					pendingTasks <- task
				}
			} else {
				log.Debug().Str("message", fmt.Sprintf("%v", message)).Msg("Skipping generator as it does not meet the launch conditions")
			}
		}
	}
	log.Debug().Msg("Waiting for all the WebSocket scanner tasks to finish")
	wg.Wait()
	totalIssues := 0
	resultsMap := make(map[string][]WebSocketScannerResult)
	f.results.Range(func(key, value interface{}) bool {
		if code, ok := key.(string); ok {
			if result, ok := value.(WebSocketScannerResult); ok {
				if _, exists := resultsMap[code]; !exists {
					resultsMap[code] = make([]WebSocketScannerResult, 0)
				}
				resultsMap[code] = append(resultsMap[code], result)
				totalIssues++
			}
		}
		return true
	})
	log.Info().Int("total_issues", totalIssues).Msg("Finished running WebSocket scanner against message")

	return resultsMap
}

func (f *WebSocketScanner) worker(wg *sync.WaitGroup, pendingTasks chan WebSocketScannerTask) {
	for task := range pendingTasks {
		taskLog := log.With().Str("payload", task.payload.Value).Logger()
		taskLog.Debug().Interface("task", task).Msg("New WebSocket scanner task received")
		if f.AvoidRepeatedIssues {
			_, ok := f.issuesFound.Load(DetectedIssue{
				code:           db.IssueCode(task.payload.IssueCode),
				insertionPoint: task.insertionPoint,
			}.String())
			if ok {
				taskLog.Debug().Msg("Skipping task as an issue for this payload with this code for this message has already been found")
				wg.Done()
				continue
			}
		}
		var result WebSocketScannerResult

		// Fuzzing logic: Send the WebSocket message with the payload
		startTime := time.Now()
		responseMessage, err := f.fuzzWebSocketMessage(task.message, task.payload.Value)
		if err != nil {
			taskLog.Error().Err(err).Msg("Error sending WebSocket message")
			wg.Done()
			continue
		}
		result.Duration = time.Since(startTime)
		result.Result = responseMessage
		result.Payload = task.payload
		result.Original = task.message
		result.Err = err

		vulnerable, details, confidence, err := f.EvaluateResult(result)
		if err != nil {
			taskLog.Error().Err(err).Msg("Error evaluating result")
			wg.Done()
			continue
		}
		issueCode := db.IssueCode(task.payload.IssueCode)

		if vulnerable {
			taskLog.Warn().Msg("Vulnerable")
			fullDetails := fmt.Sprintf("The following payload was used: %s\n\n%s", task.payload.Value, details)
			createdIssue, err := db.CreateIssueFromHistoryAndTemplate(nil, issueCode, fullDetails, confidence, "", &f.WorkspaceID, &task.options.TaskID, &task.options.TaskJobID)
			if err != nil {
				taskLog.Error().Str("code", string(issueCode)).Interface("result", result).Err(err).Msg("Error creating issue")
			} else if createdIssue.ID != 0 {
				result.Issue = &createdIssue
				f.results.Store(createdIssue.Code, result)
			}
			if f.AvoidRepeatedIssues {
				f.issuesFound.Store(DetectedIssue{
					code: issueCode,
				}.String(), true)
			}
		}

		wg.Done()
	}
}

func (f *WebSocketScanner) fuzzWebSocketMessage(message *db.WebSocketMessage, payload string) (*db.WebSocketMessage, error) {
	// Implement the logic to send the WebSocket message with the fuzzed payload
	// This is a placeholder implementation
	fuzzedMessage := *message
	fuzzedMessage.PayloadData = payload
	// Send the fuzzed WebSocket message and receive the response
	// You need to implement the actual WebSocket communication here
	return &fuzzedMessage, nil
}

func (f *WebSocketScanner) EvaluateResult(result WebSocketScannerResult) (bool, string, int, error) {
	// Evaluate the result to determine if the payload caused a vulnerability
	// This is a placeholder implementation
	vulnerable := false
	details := ""
	confidence := 0

	if strings.Contains(result.Result.PayloadData, result.Payload.Value) {
		vulnerable = true
		details = "Payload found in response"
		confidence = 100
	}

	return vulnerable, details, confidence, nil
}
