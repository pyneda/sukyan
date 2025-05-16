package scan

import (
	"context"
	"encoding/json"
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
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"gorm.io/datatypes"
)

type WebSocketDetectedIssue struct {
	code           db.IssueCode
	insertionPoint InsertionPoint
	connectionID   uint
	messageIndex   int
}

func (di WebSocketDetectedIssue) String() string {
	return fmt.Sprintf("%s:%s:%d:%d", di.code, di.insertionPoint.String(),
		di.connectionID, di.messageIndex)
}

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
	StartTime         time.Time     // Start time of the test
	PayloadSentAt     time.Time     // Time when the payload was sent
	IssueOverride     db.IssueCode  // Override issue code if needed

	// Results
	Issue *db.Issue
	Err   error
}

// WebSocketScanner is the main scanner for WebSocket connections
type WebSocketScanner struct {
	InteractionsManager *integrations.InteractionsManager
	AvoidRepeatedIssues bool
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
	Concurrency       int
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

// shouldLaunch checks if the generator should be launched according to the launch conditions
func (s *WebSocketScanner) shouldLaunch(conn *db.WebSocketConnection, generator *generation.PayloadGenerator, insertionPoint InsertionPoint, options WebSocketScanOptions) bool {

	if generator.Launch.Conditions == nil || len(generator.Launch.Conditions) == 0 {
		return true
	}

	conditionsMet := 0
	allWebsocketUnsupportedConditions := true
	for _, condition := range generator.Launch.Conditions {
		if condition.Type == generation.AvoidWebSocketMessages {
			skip, _ := lib.ParseBool(condition.Value)
			if skip {
				return false
			}
		}
		if condition.Type != generation.ResponseCondition ||
			(condition.ResponseCondition.Part != generation.Headers &&
				condition.ResponseCondition.StatusCode == 0) {
			// Found a condition that is supported in WebSocket context
			allWebsocketUnsupportedConditions = false
			break
		}
	}

	// If all conditions check headers or status codes, skip for WebSockets
	if allWebsocketUnsupportedConditions {
		return false
	}

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
		case generation.ResponseCondition:
			if condition.ResponseCondition.Part == generation.Headers {
				if generator.Launch.Operator == generation.And || len(generator.Launch.Conditions) == 1 {
					// websocket messages don't have headers, if we're using AND, we shouldn't be able to match
					return false
				}
				conditionsMet--
			} else {
				conditionsMet++
			}

			if condition.ResponseCondition.StatusCode != 0 && (generator.Launch.Operator == generation.And || len(generator.Launch.Conditions) == 1) {
				// websocket messages don't have status codes, if we're using AND, we shouldn't be able to match
				return false
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

	// Validate input parameters
	if targetMessageIndex < 0 || targetMessageIndex >= len(originalMessages) {
		log.Error().
			Int("target_index", targetMessageIndex).
			Int("messages_count", len(originalMessages)).
			Msg("Invalid target message index")
		return nil
	}

	// Set default observation window if not specified
	if options.ObservationWindow <= 0 {
		options.ObservationWindow = 10 * time.Second
	}

	// Set default concurrency
	concurrency := options.Concurrency
	if concurrency <= 0 {
		concurrency = 6
	}

	// Create a task pool using conc
	p := pool.New().WithMaxGoroutines(concurrency)

	// Generate all tasks first
	tasks := s.generateTasks(connection, originalMessages, targetMessageIndex,
		payloadGenerators, insertionPoints, options)

	// Process all tasks with the pool
	for _, task := range tasks {
		// Need to create a copy of the task for the closure to avoid data race
		taskCopy := task
		p.Go(func() {
			s.processTask(taskCopy)
		})
	}

	// Wait for all tasks to complete
	p.Wait()

	// Collect results
	resultsMap := make(map[string][]WebSocketScannerResult)
	s.results.Range(func(key, value interface{}) bool {
		if code, ok := key.(string); ok {
			if result, ok := value.(WebSocketScannerResult); ok {
				if _, exists := resultsMap[code]; !exists {
					resultsMap[code] = make([]WebSocketScannerResult, 0)
				}
				resultsMap[code] = append(resultsMap[code], result)
			}
		}
		return true
	})

	totalIssues := 0
	for _, results := range resultsMap {
		totalIssues += len(results)
	}

	log.Info().
		Int("total_issues", totalIssues).
		Str("connection", connection.URL).
		Msg("Finished running WebSocket scanner")

	return resultsMap
}

// generateTasks creates all the scanning tasks that need to be performed
func (s *WebSocketScanner) generateTasks(
	connection *db.WebSocketConnection,
	originalMessages []db.WebSocketMessage,
	targetMessageIndex int,
	payloadGenerators []*generation.PayloadGenerator,
	insertionPoints []InsertionPoint,
	options WebSocketScanOptions) []WebSocketScannerTask {

	tasks := make([]WebSocketScannerTask, 0)

	// Create tasks for each insertion point and payload combination
	for _, generator := range payloadGenerators {
		for _, insertionPoint := range insertionPoints {
			log.Debug().
				Str("connection", connection.URL).
				Int("targetMsg", targetMessageIndex).
				Str("point", insertionPoint.String()).
				Msg("Scanning WebSocket insertion point")

			if s.shouldLaunch(connection, generator, insertionPoint, options) {
				payloads, err := generator.BuildPayloads(*s.InteractionsManager)
				if err != nil {
					log.Error().Err(err).Interface("generator", generator).Msg("Failed to build payloads")
					continue
				}

				for _, payload := range payloads {
					tasks = append(tasks, WebSocketScannerTask{
						connection:         connection,
						targetMessageIndex: targetMessageIndex,
						payload:            payload,
						insertionPoint:     insertionPoint,
						options:            options,
					})
				}
			} else {
				log.Info().
					Str("url", connection.URL).
					Uint("connection_id", connection.ID).
					Int("target_index", targetMessageIndex).
					Str("generator", generator.ID).
					Str("insertion_point", insertionPoint.String()).
					Msg("Skipping generator as it does not meet launch conditions")
			}
		}
	}

	return tasks
}

// processTask handles a single WebSocket scanning task
func (s *WebSocketScanner) processTask(task WebSocketScannerTask) {
	taskLog := log.With().
		Str("connection", task.connection.URL).
		Int("target_msg", task.targetMessageIndex).
		Str("param", task.insertionPoint.Name).
		Str("payload", task.payload.Value).
		Logger()

	taskLog.Debug().Interface("task", task).Msg("Processing WebSocket scanner task")

	// Skip if we've already found this issue (if avoiding duplicates)
	issueKey := WebSocketDetectedIssue{
		code:           db.IssueCode(task.payload.IssueCode),
		insertionPoint: task.insertionPoint,
		connectionID:   task.connection.ID,
		messageIndex:   task.targetMessageIndex,
	}

	if s.AvoidRepeatedIssues {
		_, ok := s.issuesFound.Load(issueKey.String())
		if ok {
			taskLog.Debug().Msg("Skipping task as issue already found for this insertion point")
			return
		}
	}

	// Initialize the result
	var result WebSocketScannerResult
	result.OriginalConnection = task.connection
	result.TargetMessageIndex = task.targetMessageIndex
	result.Payload = task.payload
	result.InsertionPoint = task.insertionPoint
	result.ObservationWindow = task.options.ObservationWindow

	// Load original messages
	originalMessages, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		ConnectionID: task.connection.ID,
	})
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to load WebSocket messages")
		result.Err = err
		return
	}
	result.OriginalMessages = originalMessages

	if task.targetMessageIndex < 0 || task.targetMessageIndex >= len(originalMessages) {
		taskLog.Error().
			Int("target_index", task.targetMessageIndex).
			Int("messages_count", len(originalMessages)).
			Msg("Invalid target message index")
		result.Err = fmt.Errorf("invalid target message index: %d", task.targetMessageIndex)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), task.options.ObservationWindow*2)
	defer cancel()

	s.executeWebSocketTest(ctx, &result, task, taskLog)
}

// executeWebSocketTest performs the actual WebSocket connection and testing
func (s *WebSocketScanner) executeWebSocketTest(ctx context.Context, result *WebSocketScannerResult, task WebSocketScannerTask, taskLog zerolog.Logger) {
	dialer, err := createWebSocketDialer(task.connection)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to create WebSocket dialer")
		result.Err = err
		return
	}

	u, err := url.Parse(task.connection.URL)
	if err != nil {
		taskLog.Error().Err(err).Str("url", task.connection.URL).Msg("Failed to parse WebSocket URL")
		result.Err = err
		return
	}

	headers, err := task.connection.GetRequestHeadersAsMap()
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to get request headers")
		result.Err = err
		return
	}

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

	startTime := time.Now()
	result.StartTime = startTime

	// Connect to the WebSocket server
	client, upgradeResponse, err := dialer.Dial(u.String(), httpHeaders)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to connect to WebSocket server")
		result.Err = err
		return
	}
	defer client.Close()
	upgradeHistory, err := http_utils.ReadHttpResponseAndCreateHistory(upgradeResponse, http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         uint(task.options.WorkspaceID),
		TaskID:              uint(task.options.TaskID),
		TaskJobID:           uint(task.options.TaskJobID),
		CreateNewBodyStream: true,
		IsWebSocketUpgrade:  true,
	})
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to create history from upgrade response, still trying to continue...")
	}
	newConnection, err := s.setupWebSocketConnection(task, upgradeHistory, upgradeResponse)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to set up WebSocket connection")
		result.Err = err
		return
	}

	// Launch OOB test if needed
	if task.payload.InteractionDomain.URL != "" {
		go s.createOOBTest(task, *upgradeHistory)
	}

	var wg sync.WaitGroup
	// Setup message collection channels
	responseMessages := make([]db.WebSocketMessage, 0)
	responseChan := make(chan db.WebSocketMessage, 100)

	// Start the reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.readWebSocketMessages(ctx, client, newConnection.ID, responseChan, taskLog)
	}()

	// Start the collector goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg, ok := <-responseChan:
				if !ok {
					return
				}
				responseMessages = append(responseMessages, msg)
				err := db.Connection().CreateWebSocketMessage(&msg)
				if err != nil {
					taskLog.Error().Err(err).Msg("Failed to save received WebSocket message")
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	// Replay previous messages if needed to establish context
	if task.options.ReplayMessages && task.targetMessageIndex > 0 {
		sentMessages, err := replayPreviousMessages(client, newConnection.ID, result.OriginalMessages, task.targetMessageIndex)
		if err != nil {
			taskLog.Error().Err(err).Msg("Failed to replay previous websocket messages")
			// Continue anyway, this is not fatal
		} else {
			taskLog.Debug().Int("sent_messages", len(sentMessages)).Msg("Replayed previous WebSocket messages")
		}
	}

	// Prepare the modified message with payload
	originalMessage := result.OriginalMessages[task.targetMessageIndex]
	modifiedMessage, err := CreateModifiedWebSocketMessage(
		&originalMessage,
		task.insertionPoint,
		task.payload.Value,
	)
	if err != nil {
		taskLog.Error().Err(err).Str("original_message", originalMessage.String()).Msg("Failed to inject payload into message")
		result.Err = err
		return
	}

	// Update the connection ID of the modified message to use the new connection
	modifiedMessage.ConnectionID = newConnection.ID
	result.ModifiedMessage = modifiedMessage

	// Send the modified message
	var messageType int
	if modifiedMessage.Opcode == 1 {
		messageType = websocket.TextMessage
	} else {
		messageType = websocket.BinaryMessage
	}

	modifiedMessage.Timestamp = time.Now()
	result.PayloadSentAt = modifiedMessage.Timestamp

	err = client.WriteMessage(messageType, []byte(modifiedMessage.PayloadData))
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to send modified WebSocket message")
		result.Err = err
	} else {
		taskLog.Info().Str("payload", modifiedMessage.PayloadData).Msg("Sent modified WebSocket message")
	}

	// Store the modified message in the database
	err = db.Connection().CreateWebSocketMessage(modifiedMessage)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to save modified WebSocket message to database")
	}

	// Wait for the observation window
	select {
	case <-time.After(task.options.ObservationWindow):
		taskLog.Debug().Msg("Observation window expired, stopping collection of websocket messages")
		// Continue after observation window
	case <-ctx.Done():
		// Context timed out or was cancelled
	}

	// Wait for all goroutines to complete
	wg.Wait()
	// Close response channel to stop collector goroutine
	close(responseChan)

	// Close the WebSocket connection gracefully
	err = client.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		taskLog.Debug().Err(err).Msg("Failed to send close message")
	}

	taskLog.Info().Int("responses", len(responseMessages)).Str("payload", modifiedMessage.PayloadData).
		Msg("Finished collecting WebSocket responses")

	// Record results
	result.ResponseMessages = responseMessages
	result.ElapsedTime = time.Since(startTime)

	// Update the connection's closed timestamp before finishing
	newConnection.ClosedAt = time.Now()
	err = db.Connection().UpdateWebSocketConnection(&newConnection)
	if err != nil {
		taskLog.Error().Err(err).Msg("Failed to update WebSocket connection close time")
	}

	// Evaluate result
	vulnerable, details, confidence, issueOverride, err := s.EvaluateResult(*result)
	if err != nil {
		taskLog.Error().Err(err).Msg("Error evaluating WebSocket scan result")
		return
	}
	if issueOverride != "" {
		result.IssueOverride = issueOverride
	}

	// Create issue if vulnerable
	if vulnerable {
		s.handleVulnerability(result, task, details, confidence, *upgradeHistory, newConnection, taskLog)
	}
}

// readWebSocketMessages reads messages from the WebSocket connection
func (s *WebSocketScanner) readWebSocketMessages(ctx context.Context, client *websocket.Conn, connectionID uint, responseChan chan<- db.WebSocketMessage, taskLog zerolog.Logger) {

	go func() {
		<-ctx.Done()
		client.Close() // Force close the connection if the context is done
	}()

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

		wsMessage := db.WebSocketMessage{
			ConnectionID: connectionID,
			Opcode:       float64(messageType),
			Mask:         false,
			PayloadData:  string(message),
			Timestamp:    time.Now(),
			Direction:    db.MessageReceived,
		}

		select {
		case responseChan <- wsMessage:
			// Message sent to channel
		case <-ctx.Done():
			// Context cancelled or timed out
			return
		}
	}
}

// setupWebSocketConnection creates and sets up a new WebSocket connection in the database
func (s *WebSocketScanner) setupWebSocketConnection(task WebSocketScannerTask, upgradeHistory *db.History, upgradeResponse *http.Response) (db.WebSocketConnection, error) {

	// Prepare response headers
	respHeadersMap := make(map[string][]string)
	for k, v := range upgradeResponse.Header {
		respHeadersMap[k] = v
	}

	// Convert headers to JSON
	headers, err := upgradeHistory.RequestHeaders()
	if err != nil {
		headers = make(map[string][]string)
	}
	reqHeadersJSON, _ := json.Marshal(headers)
	respHeadersJSON, _ := json.Marshal(respHeadersMap)

	// Create a new connection record
	newConnection := db.WebSocketConnection{
		URL:              task.connection.URL,
		RequestHeaders:   datatypes.JSON(reqHeadersJSON),
		ResponseHeaders:  datatypes.JSON(respHeadersJSON),
		StatusCode:       upgradeResponse.StatusCode,
		StatusText:       upgradeResponse.Status,
		WorkspaceID:      &task.options.WorkspaceID,
		TaskID:           &task.options.TaskID,
		Source:           db.SourceScanner,
		UpgradeRequestID: &upgradeHistory.ID,
	}

	// Save to database
	err = db.Connection().CreateWebSocketConnection(&newConnection)
	if err != nil {
		return newConnection, err
	}

	return newConnection, nil
}

// createOOBTest creates an out-of-band test record
func (s *WebSocketScanner) createOOBTest(task WebSocketScannerTask, upgradeHistory db.History) {
	oobTest := db.OOBTest{
		Code:              db.IssueCode(task.payload.IssueCode),
		TestName:          "WebSocket OOB Test",
		InteractionDomain: task.payload.InteractionDomain.URL,
		InteractionFullID: task.payload.InteractionDomain.ID,
		Target:            task.connection.URL,
		Payload:           task.payload.Value,
		InsertionPoint:    task.insertionPoint.String(),
		WorkspaceID:       &task.options.WorkspaceID,
		TaskID:            &task.options.TaskID,
		TaskJobID:         &task.options.TaskJobID,
		HistoryID:         &upgradeHistory.ID,
	}
	db.Connection().CreateOOBTest(oobTest)
}

// handleVulnerability creates an issue record for a detected vulnerability
func (s *WebSocketScanner) handleVulnerability(result *WebSocketScannerResult, task WebSocketScannerTask,
	details string, confidence int, upgradeHistory db.History, newConnection db.WebSocketConnection, taskLog zerolog.Logger) {

	issueCode := db.IssueCode(task.payload.IssueCode)
	if result.IssueOverride != "" {
		issueCode = result.IssueOverride
		details = fmt.Sprintf("%s\n\n This issue has been detected looking for %s, but matched a response condition of %s and has been overriden", details, task.payload.IssueCode, result.IssueOverride)
	}

	fullDetails := fmt.Sprintf(
		"The following payload was inserted in the `%s` %s of WebSocket message #%d: %s\n\n%s",
		task.insertionPoint.Name,
		task.insertionPoint.Type,
		task.targetMessageIndex,
		task.payload.Value,
		details)

	createdIssue, err := db.CreateIssueFromWebSocketMessage(
		result.ModifiedMessage,
		issueCode,
		fullDetails,
		confidence,
		"",
		&task.options.WorkspaceID,
		&task.options.TaskID,
		&task.options.TaskJobID,
		&newConnection.ID,
		&upgradeHistory.ID,
	)

	if err != nil {
		taskLog.Error().Str("code", string(issueCode)).Err(err).Msg("Error creating issue")
	} else if createdIssue.ID != 0 {
		result.Issue = &createdIssue
		s.results.Store(string(createdIssue.Code), *result)
	}

	// Store issue key to avoid duplicates
	if s.AvoidRepeatedIssues {
		issueKey := WebSocketDetectedIssue{
			code:           issueCode,
			insertionPoint: task.insertionPoint,
			connectionID:   task.connection.ID,
			messageIndex:   task.targetMessageIndex,
		}
		s.issuesFound.Store(issueKey.String(), true)

		// Store broader issue key
		broadIssueKey := WebSocketDetectedIssue{
			code:           issueCode,
			insertionPoint: task.insertionPoint,
			connectionID:   task.connection.ID,
			messageIndex:   task.targetMessageIndex,
		}
		s.issuesFound.Store(broadIssueKey.String(), true)
	}
}

// EvaluateResult evaluates all detection methods for a WebSocket scan result
func (s *WebSocketScanner) EvaluateResult(result WebSocketScannerResult) (bool, string, int, db.IssueCode, error) {
	// Iterate through payload detection methods
	vulnerable := false
	condition := result.Payload.DetectionCondition
	confidence := 0
	var sb strings.Builder
	var issueOverride db.IssueCode

	for _, detectionMethod := range result.Payload.DetectionMethods {
		// Evaluate the detection method
		detectionMethodResult, description, conf, override, err := s.EvaluateDetectionMethod(result, detectionMethod)

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
			return false, "", confidence, issueOverride, nil
		}
	}

	return vulnerable, sb.String(), confidence, issueOverride, nil
}

// EvaluateDetectionMethod evaluates a single detection method against WebSocket responses
func (s *WebSocketScanner) EvaluateDetectionMethod(result WebSocketScannerResult, method generation.DetectionMethod) (bool, string, int, db.IssueCode, error) {
	switch m := method.GetMethod().(type) {
	case *generation.OOBInteractionDetectionMethod:
		// OOB detection is handled externally by the interaction manager
		log.Debug().Msg("OOB Interaction detection method validation handled by interaction manager")
		return false, "OOB Interaction detection will be validated by interaction callbacks", m.Confidence, "", nil

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
		return false, "", 0, "", nil

	default:
		return false, "", 0, "", fmt.Errorf("unsupported detection method type for WebSocket scanning")
	}
}

// evaluateResponseCondition checks for specific content in WebSocket response messages
func (s *WebSocketScanner) evaluateResponseCondition(result WebSocketScannerResult, method *generation.ResponseConditionDetectionMethod) (bool, string, int, db.IssueCode, error) {
	var sb strings.Builder

	if len(result.ResponseMessages) == 0 {
		return false, "No response messages received", 0, "", nil
	}

	// Check for content match in any response message
	for i, msg := range result.ResponseMessages {
		if method.Contains != "" {
			if strings.Contains(msg.PayloadData, method.Contains) {
				sb.WriteString(fmt.Sprintf("Response message #%d contains the value: %s\n",
					i, method.Contains))
				return true, sb.String(), method.Confidence, method.IssueOverride, nil
			}
		}
	}

	return false, "", 0, "", nil
}

// evaluateReflection checks if a payload is reflected in any response message
func (s *WebSocketScanner) evaluateReflection(result WebSocketScannerResult, method *generation.ReflectionDetectionMethod) (bool, string, int, db.IssueCode, error) {
	for i, msg := range result.ResponseMessages {
		if strings.Contains(msg.PayloadData, method.Value) {
			description := fmt.Sprintf("WebSocket response message #%d contains the reflected value: %s",
				i, method.Value)
			return true, description, method.Confidence, "", nil
		}
	}
	return false, "", 0, "", nil
}

// evaluateTimeBased checks if the scan execution took longer than expected
func (s *WebSocketScanner) evaluateTimeBased(result WebSocketScannerResult, method *generation.TimeBasedDetectionMethod) (bool, string, int, db.IssueCode, error) {
	matched := false
	var sb strings.Builder
	confidence := 0

	for i, msg := range result.ResponseMessages {
		duration := msg.Timestamp.Sub(result.PayloadSentAt)
		if method.CheckIfResultDurationIsHigher(duration) {
			sb.WriteString(fmt.Sprintf("WebSocket response message #%d took %s, which is greater than the expected payload sleep time of %s\n", i, duration, method.Sleep))
			sb.WriteString(fmt.Sprintf(" - Payload: %s\n", result.Payload.Value))
			sb.WriteString(fmt.Sprintf(" - Payload sent at: %s\n", result.PayloadSentAt))
			sb.WriteString(fmt.Sprintf(" - Response received at: %s\n\n", msg.Timestamp))
			// NOTE: Additional validation could be done here, similar to the http template scanner
			if confidence == 0 {
				confidence = method.Confidence
			} else {
				confidence += 5
			}
			matched = true
		}
	}

	if confidence > 100 {
		confidence = 100
	}
	return matched, sb.String(), confidence, "", nil
}

// evaluateResponseCheck checks for error patterns in WebSocket response messages
func (s *WebSocketScanner) evaluateResponseCheck(result WebSocketScannerResult, method *generation.ResponseCheckDetectionMethod) (bool, string, int, db.IssueCode, error) {
	for i, msg := range result.ResponseMessages {
		if method.Check == generation.DatabaseErrorCondition {
			errorResult := passive.SearchDatabaseErrors(msg.PayloadData)
			if errorResult != nil {
				description := fmt.Sprintf("Database error in response message #%d:\n - Database: %s\n - Error: %s",
					i, errorResult.DatabaseName, errorResult.MatchStr)
				return true, description, method.Confidence, method.IssueOverride, nil
			}
		} else if method.Check == generation.XPathErrorCondition {
			errorResult := passive.SearchXPathErrors(msg.PayloadData)
			if errorResult != "" {
				description := fmt.Sprintf("XPath error in response message #%d:\n - Error: %s",
					i, errorResult)
				return true, description, method.Confidence, method.IssueOverride, nil
			}
		}
	}
	return false, "", 0, "", nil
}
