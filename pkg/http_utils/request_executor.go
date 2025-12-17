package http_utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// drainBody reads all of b to memory and then returns two equivalent
// ReadClosers yielding the same bytes.
func drainBody(b io.ReadCloser) (r1, r2 io.ReadCloser, err error) {
	if b == nil || b == http.NoBody {
		return http.NoBody, http.NoBody, nil
	}
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(b); err != nil {
		return nil, b, err
	}
	if err = b.Close(); err != nil {
		return nil, b, err
	}
	return io.NopCloser(&buf), io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

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

	client := options.Client
	if client == nil {
		client = CreateHttpClient()
	}

	timeout := options.Timeout
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	result := RequestExecutionResult{
		Duration: 0,
		TimedOut: false,
	}

	// Handle request body preservation if needed
	var requestBodyCopy io.ReadCloser
	var savedBodyBytes []byte
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			result.Err = err
			result.Duration = time.Since(startTime)
			return result
		}
		savedBodyBytes = bodyBytes
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		requestBodyCopy = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Execute request
	response, err := client.Do(req)
	result.Duration = time.Since(startTime)
	result.Err = err

	if err != nil {
		result.TimedOut = IsTimeoutError(err)

		if options.CreateHistory && result.TimedOut {
			// Restore the body since client.Do consumed it
			if savedBodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewBuffer(savedBodyBytes))
				req.ContentLength = int64(len(savedBodyBytes))
			}
			timeoutHistory, historyErr := CreateTimeoutHistory(req, result.Duration, err, options.HistoryCreationOptions)
			if historyErr != nil {
				log.Error().Err(historyErr).Msg("Error creating timeout history record")
			} else {
				result.History = timeoutHistory
			}
		}
		return result
	}

	// Restore request body in response for dumping
	if requestBodyCopy != nil {
		response.Request.Body = requestBodyCopy
	}

	// Drain response body once to get two identical copies
	responseBody1, responseBody2, err := drainBody(response.Body)
	if err != nil {
		result.Err = err
		return result
	}

	// Use first copy for dumping
	response.Body = responseBody1
	responseDump, err := httputil.DumpResponse(response, true)
	if err != nil {
		result.Err = err
		return result
	}

	// Use second copy for body data
	bodyBytes, err := io.ReadAll(responseBody2)
	if err != nil {
		result.Err = err
		return result
	}
	responseBody2.Close()

	// Create response data
	responseData := FullResponseData{
		Body:      bodyBytes,
		BodySize:  len(bodyBytes),
		Raw:       responseDump,
		RawString: string(responseDump),
		RawSize:   len(responseDump),
	}

	// Set response body for caller if needed
	if options.HistoryCreationOptions.CreateNewBodyStream {
		response.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	} else {
		response.Body = http.NoBody
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

// IsTimeoutError checks if an error is due to timeout
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := err.Error()
	return strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "deadline exceeded") ||
		strings.Contains(errorStr, "context deadline exceeded") ||
		strings.Contains(errorStr, "operation timed out")
}

// ParseStatusCodeFromRawResponse extracts the HTTP status code from a raw HTTP response.
func ParseStatusCodeFromRawResponse(response []byte) int {
	lines := bytes.SplitN(response, []byte("\r\n"), 2)
	if len(lines) == 0 {
		return 0
	}
	statusLine := string(lines[0])
	var statusCode int
	_, err := fmt.Sscanf(statusLine, "HTTP/%s %d", new(string), &statusCode)
	if err != nil {
		return 0
	}
	return statusCode
}
