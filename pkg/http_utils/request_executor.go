package http_utils

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// RequestExecutionResult contains the complete result of an HTTP request execution
type RequestExecutionResult struct {
	Response     *http.Response
	ResponseData FullResponseData
	History      *db.History
	Duration     time.Duration
	Err          error
	TimedOut     bool
}

// RequestExecutionOptions contains options for executing HTTP requests
type RequestExecutionOptions struct {
	Client                 *http.Client
	Timeout                time.Duration
	HistoryCreationOptions HistoryCreationOptions
	CreateHistory          bool
}

// ExecuteRequest executes an HTTP request and returns a complete result including history
func ExecuteRequest(req *http.Request, options RequestExecutionOptions) RequestExecutionResult {
	startTime := time.Now()

	// Use defaults if not provided
	client := options.Client
	if client == nil {
		client = CreateHttpClient()
	}

	timeout := options.Timeout
	// if timeout == 0 {
	// 	timeout = 2 * time.Minute
	// }

	// Set up context with timeout
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	result := RequestExecutionResult{
		Duration: 0,
		TimedOut: false,
	}

	// Send the request
	response, err := SendRequest(client, req)
	result.Duration = time.Since(startTime)
	result.Err = err

	if err != nil {
		result.TimedOut = isTimeoutError(err)

		// For timeout errors, create timeout history if requested
		if options.CreateHistory && result.TimedOut {
			timeoutHistory, historyErr := CreateTimeoutHistory(req, result.Duration, err, options.HistoryCreationOptions)
			if historyErr != nil {
				log.Error().Err(historyErr).Msg("Error creating timeout history record")
			} else {
				result.History = timeoutHistory
			}
		}
		return result
	}

	// Read response data completely before doing anything else
	responseData, newBody, err := ReadFullResponse(response, options.HistoryCreationOptions.CreateNewBodyStream)
	if err != nil {
		result.Err = err
		log.Error().Err(err).Msg("Error reading response body")
		return result
	}

	// Replace response body if requested
	if options.HistoryCreationOptions.CreateNewBodyStream {
		response.Body = newBody
	}

	result.Response = response
	result.ResponseData = responseData

	// Create history if requested
	if options.CreateHistory {
		history, err := CreateHistoryFromHttpResponse(response, responseData, options.HistoryCreationOptions)
		if err != nil {
			log.Error().Err(err).Msg("Error creating history from response")
		} else {
			result.History = history
		}
	}

	return result
}

// ExecuteRequestSimple is a convenience method for simple request execution with default options
func ExecuteRequestSimple(req *http.Request, historyOptions HistoryCreationOptions) RequestExecutionResult {
	return ExecuteRequest(req, RequestExecutionOptions{
		CreateHistory:          true,
		HistoryCreationOptions: historyOptions,
	})
}

// ExecuteRequestWithTimeout executes a request with a specific timeout
func ExecuteRequestWithTimeout(req *http.Request, timeout time.Duration, historyOptions HistoryCreationOptions) RequestExecutionResult {
	return ExecuteRequest(req, RequestExecutionOptions{
		Timeout:                timeout,
		CreateHistory:          true,
		HistoryCreationOptions: historyOptions,
	})
}

// IsTimeoutError checks if an error is due to timeout (exported version)
func IsTimeoutError(err error) bool {
	return isTimeoutError(err)
}

// isTimeoutError checks if an error is due to timeout
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := err.Error()
	return strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "deadline exceeded") ||
		strings.Contains(errorStr, "context deadline exceeded") ||
		strings.Contains(errorStr, "operation timed out")
}

// CalculateTimeoutForPayload calculates an appropriate timeout for a time-based payload
func CalculateTimeoutForPayload(expectedSleepDuration time.Duration) time.Duration {
	if expectedSleepDuration > 0 {
		// For time-based payloads, add buffer to expected sleep duration
		timeout := time.Duration(float64(expectedSleepDuration) * 2.0)

		// Set reasonable bounds: minimum 30s, maximum 5 minutes
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}
		if timeout > 5*time.Minute {
			timeout = 5 * time.Minute
		}
		return timeout
	}

	return 2 * time.Minute
}
