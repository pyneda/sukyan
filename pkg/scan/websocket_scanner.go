package scan

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// WebSocketScannerResult contains all the information about a WebSocket scan
type WebSocketScannerResult struct {
	OriginalConnection *db.WebSocketConnection // The original WebSocket connection we're testing

	// Message context
	OriginalMessages   []db.WebSocketMessage // All original messages in the connection
	TargetMessageIndex int                   // Index of the message we're fuzzing

	// Fuzzing results
	ModifiedMessage  *db.WebSocketMessage  // The message we sent with payload
	ResponseMessages []db.WebSocketMessage // Messages received after sending modified message

	// Evaluation data
	Payload           generation.Payload
	InsertionPoint    InsertionPoint
	ObservationWindow time.Duration // How long we observed for responses
	ElapsedTime       time.Duration // Total time of the test execution

	// Results
	Issue *db.Issue
	Err   error
}

// WebSocketScanner is the main scanner for WebSocket connections
type WebSocketScanner struct {
	Concurrency         int
	InteractionsManager *integrations.InteractionsManager
	AvoidRepeatedIssues bool
	WorkspaceID         uint
	Mode                options.ScanMode
	ObservationWindow   time.Duration // How long to wait for responses after sending a modified message
	issuesFound         sync.Map
	results             sync.Map
}

// WebSocketScanOptions defines the options for a WebSocket scan
type WebSocketScanOptions struct {
	WorkspaceID       uint
	TaskID            uint
	TaskJobID         uint
	Mode              options.ScanMode
	FingerprintTags   []string
	ReplayMessages    bool          // Whether to replay previous messages to establish context
	ObservationWindow time.Duration // How long to wait for responses
}

// WebSocketScannerTask represents a single fuzzing task
type WebSocketScannerTask struct {
	connection         *db.WebSocketConnection
	targetMessageIndex int
	insertionPoint     InsertionPoint
	payload            generation.Payload
	options            WebSocketScanOptions
}

// MessageBuilder represents how to build a WebSocket message with a payload
type MessageBuilder struct {
	Message db.WebSocketMessage
	Point   InsertionPoint
	Payload string
}

// checkConfig sets default configuration values if not specified
func (s *WebSocketScanner) checkConfig() {
	if s.Concurrency == 0 {
		log.Info().Interface("scanner", s).Msg("Concurrency is not set, setting 4 as default")
		s.Concurrency = 4
	}
	if s.ObservationWindow == 0 {
		log.Info().Interface("scanner", s).Msg("ObservationWindow is not set, setting 5s as default")
		s.ObservationWindow = 5 * time.Second
	}

	if s.Mode == "" {
		log.Info().Interface("scanner", s).Msg("Scan mode is not set, setting smart as default")
		s.Mode = options.ScanModeSmart
	}
}

// shouldLaunch checks if the generator should be launched according to the launch conditions
func (s *WebSocketScanner) shouldLaunch(conn *db.WebSocketConnection, generator *generation.PayloadGenerator, insertionPoint InsertionPoint, options WebSocketScanOptions) bool {

	if generator.Launch.Conditions == nil || len(generator.Launch.Conditions) == 0 {
		return true
	}

	conditionsMet := 0

	// NOTE: Some adjustments might be needed here for websocket
	for _, condition := range generator.Launch.Conditions {
		switch condition.Type {
		case generation.Platform:
			if lib.SliceContains(options.FingerprintTags, condition.Value) {
				conditionsMet++
			} else {
				platform := ParsePlatform(condition.Value)
				if platform.MatchesAnyFingerprint(nil) {
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

		}
	}

	if generator.Launch.Operator == generation.Or {
		return conditionsMet > 0
	}

	return conditionsMet == len(generator.Launch.Conditions)
}

// Run starts the WebSocket scanning job
func (s *WebSocketScanner) Run(
	connection *db.WebSocketConnection,
	originalMessages []db.WebSocketMessage,
	targetMessageIndex int,
	payloadGenerators []*generation.PayloadGenerator,
	insertionPoints []InsertionPoint,
	options WebSocketScanOptions) map[string][]WebSocketScannerResult {

	var wg sync.WaitGroup
	s.checkConfig()

	// Use provided observation window if specified
	if options.ObservationWindow > 0 {
		s.ObservationWindow = options.ObservationWindow
	}

	pendingTasks := make(chan WebSocketScannerTask, s.Concurrency)
	defer close(pendingTasks)

	for i := 0; i < s.Concurrency; i++ {
		go s.worker(&wg, pendingTasks)
	}

	// Make sure targetMessageIndex is valid
	if targetMessageIndex < 0 || targetMessageIndex >= len(originalMessages) {
		log.Error().
			Int("target_index", targetMessageIndex).
			Int("messages_count", len(originalMessages)).
			Msg("Invalid target message index")
		return nil
	}

	// Create tasks for each insertion point and payload combination
	for _, insertionPoint := range insertionPoints {
		log.Debug().
			Str("connection", connection.URL).
			Int("targetMsg", targetMessageIndex).
			Str("point", insertionPoint.String()).
			Msg("Scanning WebSocket insertion point")

		for _, generator := range payloadGenerators {
			if s.shouldLaunch(connection, generator, insertionPoint, options) {
				payloads, err := generator.BuildPayloads(*s.InteractionsManager)
				if err != nil {
					log.Error().Err(err).Interface("generator", generator).Msg("Failed to build payloads")
					continue
				}

				for _, payload := range payloads {
					wg.Add(1)
					task := WebSocketScannerTask{
						connection:         connection,
						targetMessageIndex: targetMessageIndex,
						payload:            payload,
						insertionPoint:     insertionPoint,
						options:            options,
					}
					pendingTasks <- task
				}
			} else {
				log.Debug().
					Str("connection", connection.URL).
					Int("target_index", targetMessageIndex).
					Str("generator", generator.ID).
					Str("insertion_point", insertionPoint.String()).
					Msg("Skipping generator as it does not meet launch conditions")
			}
		}
	}

	log.Debug().Msg("Waiting for all WebSocket scanner tasks to finish")
	wg.Wait()

	// Collect results
	totalIssues := 0
	resultsMap := make(map[string][]WebSocketScannerResult)
	s.results.Range(func(key, value interface{}) bool {
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

	log.Info().
		Int("total_issues", totalIssues).
		Str("connection", connection.URL).
		Msg("Finished running WebSocket scanner")

	return resultsMap
}

// worker processes WebSocket scanning tasks
func (s *WebSocketScanner) worker(wg *sync.WaitGroup, pendingTasks chan WebSocketScannerTask) {
	for task := range pendingTasks {
		taskLog := log.With().
			Str("connection", task.connection.URL).
			Int("target_msg", task.targetMessageIndex).
			Str("param", task.insertionPoint.Name).
			Str("payload", task.payload.Value).
			Logger()

		taskLog.Debug().Interface("task", task).Msg("New WebSocket scanner task received")

		// Skip if we've already found this issue (if avoiding duplicates)
		if s.AvoidRepeatedIssues {
			_, ok := s.issuesFound.Load(DetectedIssue{
				code:           db.IssueCode(task.payload.IssueCode),
				insertionPoint: task.insertionPoint,
			}.String())
			if ok {
				taskLog.Debug().Msg("Skipping task as issue already found for this insertion point")
				wg.Done()
				continue
			}
		}

		// Initialize the result
		var result WebSocketScannerResult
		result.OriginalConnection = task.connection
		result.TargetMessageIndex = task.targetMessageIndex
		result.Payload = task.payload
		result.InsertionPoint = task.insertionPoint
		result.ObservationWindow = s.ObservationWindow

		// Load original messages
		messages, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
			ConnectionID: task.connection.ID,
		})
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to load WebSocket messages")
			result.Err = err
			wg.Done()
			continue
		}
		result.OriginalMessages = messages

		if task.targetMessageIndex < 0 || task.targetMessageIndex >= len(messages) {
			taskLog.Error().
				Int("target_index", task.targetMessageIndex).
				Int("messages_count", len(messages)).
				Msg("Invalid target message index")
			result.Err = fmt.Errorf("invalid target message index: %d", task.targetMessageIndex)
			wg.Done()
			continue
		}

		startTime := time.Now()

		// Create a WebSocket connection
		dialer, err := createWebSocketDialer(task.connection)
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to create WebSocket dialer")
			result.Err = err
			wg.Done()
			continue
		}

		// Parse the URL
		u, err := url.Parse(task.connection.URL)
		if err != nil {
			taskLog.Error().Err(err).Str("url", task.connection.URL).Msg("Failed to parse WebSocket URL")
			result.Err = err
			wg.Done()
			continue
		}

		// Get request headers
		headers, err := task.connection.GetRequestHeadersAsMap()
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to get request headers")
			result.Err = err
			wg.Done()
			continue
		}

		// Convert headers to http.Header
		httpHeaders := http.Header{}
		for key, values := range headers {
			// Skip WebSocket-specific headers that the client will set automatically
			if strings.EqualFold(key, "Connection") ||
				strings.EqualFold(key, "Sec-WebSocket-Key") ||
				strings.EqualFold(key, "Sec-WebSocket-Version") ||
				strings.EqualFold(key, "Sec-WebSocket-Protocol") ||
				strings.EqualFold(key, "Sec-WebSocket-Extensions") ||
				strings.EqualFold(key, "Upgrade") {
				continue
			}
			for _, value := range values {
				httpHeaders.Set(key, value)
			}
		}

		// Connect to the WebSocket server
		client, _, err := dialer.Dial(u.String(), httpHeaders)
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to connect to WebSocket server")
			result.Err = err
			wg.Done()
			continue
		}
		defer client.Close()

		// Replay previous messages if needed to establish context
		if task.options.ReplayMessages && task.targetMessageIndex > 0 {
			err = replayPreviousMessages(client, messages, task.targetMessageIndex)
			if err != nil {
				taskLog.Error().Err(err).Msg("Failed to replay previous messages")
				result.Err = err
				wg.Done()
				continue
			}
		}

		// Prepare the modified message with payload
		originalMessage := messages[task.targetMessageIndex]
		modifiedMessage, err := CreateModifiedWebSocketMessage(
			&originalMessage,
			task.insertionPoint,
			task.payload.Value,
		)

		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to inject payload into message")
			result.Err = err
			wg.Done()
			continue
		}
		result.ModifiedMessage = modifiedMessage

		// Setup message collection
		responseMessages := make([]db.WebSocketMessage, 0)
		responseChan := make(chan db.WebSocketMessage, 100)
		doneCollecting := make(chan struct{})

		// Start collecting responses
		go func() {
			defer close(doneCollecting)

			for {
				select {
				case msg := <-responseChan:
					responseMessages = append(responseMessages, msg)
				case <-time.After(s.ObservationWindow):
					return
				}
			}
		}()

		// Start goroutine to read from the WebSocket
		go func() {
			for {
				messageType, message, err := client.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err,
						websocket.CloseGoingAway,
						websocket.CloseNormalClosure,
						websocket.CloseAbnormalClosure) {
						taskLog.Error().Err(err).Msg("WebSocket read error")
					}
					return
				}

				// Create a WebSocketMessage from the received message
				wsMessage := db.WebSocketMessage{
					ConnectionID: task.connection.ID,
					Opcode:       float64(messageType),
					Mask:         false,
					PayloadData:  string(message),
					Timestamp:    time.Now(),
					Direction:    db.MessageReceived,
				}

				responseChan <- wsMessage
			}
		}()

		// Send the modified message
		var messageType int
		if modifiedMessage.Opcode == 1 {
			messageType = websocket.TextMessage
		} else {
			messageType = websocket.BinaryMessage
		}

		err = client.WriteMessage(messageType, []byte(modifiedMessage.PayloadData))
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to send modified WebSocket message")
			result.Err = err
			wg.Done()
			continue
		} else {
			taskLog.Info().Str("payload", modifiedMessage.PayloadData).Msg("Sent modified WebSocket message")
		}

		// Wait for the observation window to complete
		<-doneCollecting

		taskLog.Info().Int("responses", len(responseMessages)).Str("payload", modifiedMessage.PayloadData).
			Msg("Finished collecting WebSocket responses")
		// Record results
		result.ResponseMessages = responseMessages
		result.ElapsedTime = time.Since(startTime)

		// Evaluate result
		vulnerable, details, confidence, err := s.EvaluateResult(result)
		if err != nil {
			taskLog.Error().Err(err).Msg("Error evaluating WebSocket scan result")
			wg.Done()
			continue
		}

		// Handle OOB test if needed
		if task.payload.InteractionDomain.URL != "" {
			oobTest := db.OOBTest{
				Code:              db.IssueCode(task.payload.IssueCode),
				TestName:          "WebSocket Fuzz Test",
				InteractionDomain: task.payload.InteractionDomain.URL,
				InteractionFullID: task.payload.InteractionDomain.ID,
				Target:            task.connection.URL,
				Payload:           task.payload.Value,
				InsertionPoint:    task.insertionPoint.String(),
				WorkspaceID:       &s.WorkspaceID,
				TaskID:            &task.options.TaskID,
				TaskJobID:         &task.options.TaskJobID,
			}
			db.Connection().CreateOOBTest(oobTest)
			taskLog.Debug().Interface("oobTest", oobTest).Msg("Created OOB Test")
		}

		// Create issue if vulnerable
		if vulnerable {
			taskLog.Warn().Str("code", task.payload.IssueCode).Msg("Vulnerability found in WebSocket message, creating issue")

			issueCode := db.IssueCode(task.payload.IssueCode)
			fullDetails := fmt.Sprintf(
				"The following payload was inserted in the `%s` %s of WebSocket message #%d: %s\n\n%s",
				task.insertionPoint.Name,
				task.insertionPoint.Type,
				task.targetMessageIndex,
				task.payload.Value,
				details)

			createdIssue, err := db.CreateIssueFromWebSocketMessage(
				modifiedMessage,
				issueCode,
				fullDetails,
				confidence,
				"",
				&s.WorkspaceID,
				&task.options.TaskID,
				&task.options.TaskJobID,
				&task.connection.ID)

			if err != nil {
				taskLog.Error().Str("code", string(issueCode)).Err(err).Msg("Error creating issue")
			} else if createdIssue.ID != 0 {
				result.Issue = &createdIssue
				s.results.Store(string(createdIssue.Code), result)
			}

			// Store issue key to avoid duplicates
			if s.AvoidRepeatedIssues {
				s.issuesFound.Store(DetectedIssue{
					code:           issueCode,
					insertionPoint: task.insertionPoint,
				}.String(), true)
			}
		}

		wg.Done()
	}
}

// EvaluateResult evaluates all detection methods for a WebSocket scan result
func (s *WebSocketScanner) EvaluateResult(result WebSocketScannerResult) (bool, string, int, error) {
	// Iterate through payload detection methods
	vulnerable := false
	condition := result.Payload.DetectionCondition
	confidence := 0
	var sb strings.Builder

	for _, detectionMethod := range result.Payload.DetectionMethods {
		// Evaluate the detection method
		detectionMethodResult, description, conf, err := s.EvaluateDetectionMethod(result, detectionMethod)

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
			vulnerable = true
			// If condition is OR, we can return immediately
			if condition == generation.Or {
				return true, sb.String(), confidence, nil
			}
		} else if condition == generation.And {
			// If condition is AND and one method failed, the whole test fails
			return false, "", confidence, nil
		}
	}

	return vulnerable, sb.String(), confidence, nil
}

// EvaluateDetectionMethod evaluates a single detection method against WebSocket responses
func (s *WebSocketScanner) EvaluateDetectionMethod(result WebSocketScannerResult, method generation.DetectionMethod) (bool, string, int, error) {
	switch m := method.GetMethod().(type) {
	case *generation.OOBInteractionDetectionMethod:
		// OOB detection is handled externally by the interaction manager
		log.Debug().Msg("OOB Interaction detection method validation handled by interaction manager")
		return false, "OOB Interaction detection will be validated by interaction callbacks", m.Confidence, nil

	case *generation.ResponseConditionDetectionMethod:
		return s.evaluateResponseCondition(result, m)

	case *generation.ReflectionDetectionMethod:
		return s.evaluateReflection(result, m)

	case *generation.TimeBasedDetectionMethod:
		return s.evaluateTimeBased(result, m)

	case *generation.ResponseCheckDetectionMethod:
		return s.evaluateResponseCheck(result, m)

	case *generation.BrowserEventsDetectionMethod:
		log.Warn().Msg("Browser Events detection method not implemented for WebSocket scanning")
		return false, "", 0, nil

	default:
		return false, "", 0, fmt.Errorf("unsupported detection method type for WebSocket scanning")
	}
}

// evaluateResponseCondition checks for specific content in WebSocket response messages
func (s *WebSocketScanner) evaluateResponseCondition(result WebSocketScannerResult, method *generation.ResponseConditionDetectionMethod) (bool, string, int, error) {
	var sb strings.Builder

	if len(result.ResponseMessages) == 0 {
		return false, "No response messages received", 0, nil
	}

	// Check for content match in any response message
	for i, msg := range result.ResponseMessages {
		if method.Contains != "" {
			if strings.Contains(msg.PayloadData, method.Contains) {
				sb.WriteString(fmt.Sprintf("Response message #%d contains the value: %s\n",
					i, method.Contains))
				return true, sb.String(), method.Confidence, nil
			}
		}
	}

	// If contains is not specified, check if we received ANY response (we did if we got here)
	if method.Contains == "" {
		sb.WriteString(fmt.Sprintf("Received %d response messages\n", len(result.ResponseMessages)))
		return true, sb.String(), method.Confidence, nil
	}

	return false, "", 0, nil
}

// evaluateReflection checks if a payload is reflected in any response message
func (s *WebSocketScanner) evaluateReflection(result WebSocketScannerResult, method *generation.ReflectionDetectionMethod) (bool, string, int, error) {
	for i, msg := range result.ResponseMessages {
		if strings.Contains(msg.PayloadData, method.Value) {
			description := fmt.Sprintf("WebSocket response message #%d contains the reflected value: %s",
				i, method.Value)
			return true, description, method.Confidence, nil
		}
	}
	return false, "", 0, nil
}

// evaluateTimeBased checks if the scan execution took longer than expected
func (s *WebSocketScanner) evaluateTimeBased(result WebSocketScannerResult, method *generation.TimeBasedDetectionMethod) (bool, string, int, error) {
	if method.CheckIfResultDurationIsHigher(result.ElapsedTime) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Test execution took %s, which is greater than the expected payload sleep time of %s\n",
			result.ElapsedTime, method.Sleep))

		// NOTE: Additional validation could be done here, similar to the http template scanner

		return true, sb.String(), method.Confidence, nil
	}
	return false, "", 0, nil
}

// evaluateResponseCheck checks for error patterns in WebSocket response messages
func (s *WebSocketScanner) evaluateResponseCheck(result WebSocketScannerResult, method *generation.ResponseCheckDetectionMethod) (bool, string, int, error) {
	for i, msg := range result.ResponseMessages {
		if method.Check == generation.DatabaseErrorCondition {
			errorResult := passive.SearchDatabaseErrors(msg.PayloadData)
			if errorResult != nil {
				description := fmt.Sprintf("Database error in response message #%d:\n - Database: %s\n - Error: %s",
					i, errorResult.DatabaseName, errorResult.MatchStr)
				return true, description, method.Confidence, nil
			}
		} else if method.Check == generation.XPathErrorCondition {
			errorResult := passive.SearchXPathErrors(msg.PayloadData)
			if errorResult != "" {
				description := fmt.Sprintf("XPath error in response message #%d:\n - Error: %s",
					i, errorResult)
				return true, description, method.Confidence, nil
			}
		}
	}
	return false, "", 0, nil
}
