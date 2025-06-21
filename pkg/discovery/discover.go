package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

const (
	DefaultConcurrency = 10
	DefaultTimeout     = 45
	DefaultMethod      = "GET"
	DefaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"
)

type ValidationFunc func(*db.History) (bool, string, int)

// DefaultValidationFunc validates based on HTTP status code 200
func DefaultValidationFunc(history *db.History) (bool, string, int) {
	return history.StatusCode == 200, "Validated that status code is 200", 90
}

type DiscoveryInput struct {
	URL                    string `json:"url"`
	HistoryCreationOptions http_utils.HistoryCreationOptions
	Method                 string                   `json:"method"`
	Body                   string                   `json:"body"`
	Concurrency            int                      `json:"concurrency"`
	Timeout                int                      `json:"timeout"`
	Paths                  []string                 `json:"paths"`
	Headers                map[string]string        `json:"headers"`
	StopAfterValid         bool                     `json:"stop_after_valid"`
	ValidationFunc         ValidationFunc           `json:"-"`
	HttpClient             *http.Client             `json:"-"`
	SiteBehavior           *http_utils.SiteBehavior `json:"-"`
	ScanMode               scan_options.ScanMode    `json:"-"`
}

type DiscoverResults struct {
	Responses []*db.History `json:"responses"`
	Errors    []error       `json:"errors,omitempty"`
	Stopped   bool          `json:"stopped,omitempty"`
}

// Validate checks and sets default values for DiscoveryInput
func (d *DiscoveryInput) Validate() error {
	if d.URL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if d.Concurrency == 0 {
		d.Concurrency = DefaultConcurrency
	}
	if d.Timeout == 0 || d.Timeout < 0 {
		d.Timeout = DefaultTimeout
	}
	if d.Method == "" {
		d.Method = DefaultMethod
	}
	if d.ValidationFunc == nil {
		d.ValidationFunc = DefaultValidationFunc
	}

	_, err := url.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	return nil
}

func DiscoverPaths(input DiscoveryInput) (DiscoverResults, error) {
	if err := input.Validate(); err != nil {
		return DiscoverResults{}, fmt.Errorf("invalid input: %w", err)
	}

	baseCtx := context.Background()
	if input.Timeout > 0 {
		var cancel context.CancelFunc
		baseCtx, cancel = context.WithTimeout(baseCtx, time.Duration(input.Timeout)*time.Second)
		defer cancel()
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if input.StopAfterValid {
		ctx, cancel = context.WithCancel(baseCtx)
	} else {
		ctx = baseCtx
		cancel = func() {}
	}
	defer cancel()

	p := pool.NewWithResults[struct {
		History *db.History
		Error   error
	}]().WithContext(ctx).WithMaxGoroutines(input.Concurrency)

	var (
		mu         sync.Mutex
		validFound bool
		results    = DiscoverResults{
			Responses: make([]*db.History, 0),
			Errors:    make([]error, 0),
		}
	)

	client := input.HttpClient
	if client == nil {
		transport := http_utils.CreateHttpTransport()
		transport.ForceAttemptHTTP2 = true
		client = &http.Client{
			Transport: transport,
		}
	}

	for _, path := range input.Paths {
		currentPath := path
		p.Go(func(ctx context.Context) (struct {
			History *db.History
			Error   error
		}, error) {
			result := struct {
				History *db.History
				Error   error
			}{}

			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result, nil
			default:
			}

			if input.StopAfterValid {
				mu.Lock()
				if validFound {
					mu.Unlock()
					log.Info().Msg("stopping discovery after valid response")
					//result.Error = context.Canceled
					return result, nil
				}
				mu.Unlock()
			}

			targetURL := lib.JoinURLPath(input.URL, currentPath)

			var bodyReader io.Reader
			if input.Body != "" {
				bodyReader = strings.NewReader(input.Body)
			}

			request, err := http.NewRequest(input.Method, targetURL, bodyReader)
			if err != nil {
				result.Error = fmt.Errorf("failed to create request: %w", err)
				return result, nil
			}

			request = request.WithContext(ctx)
			setDefaultHeaders(request, input.Body != "")

			for key, value := range input.Headers {
				request.Header.Set(key, value)
			}

			executionResult := http_utils.ExecuteRequestWithTimeout(request, 30*time.Second, input.HistoryCreationOptions)

			if executionResult.Err != nil {
				log.Warn().Err(executionResult.Err).Msg("failed to send request")
				result.Error = fmt.Errorf("failed to send request: %w", executionResult.Err)
				return result, nil
			}

			history := executionResult.History
			result.History = history

			if input.SiteBehavior != nil && input.SiteBehavior.IsNotFound(history) {
				log.Debug().Int("history", int(history.ID)).Str("url", history.URL).Int("status_code", history.StatusCode).Msg("skipping not found response based on site behavior")
				return result, nil
			}

			if valid, _, _ := input.ValidationFunc(history); valid && input.StopAfterValid {
				mu.Lock()
				if !validFound {
					validFound = true
					cancel()
				}
				mu.Unlock()
			}

			return result, nil
		})
	}

	responses, err := p.Wait()
	if err != nil && err != context.Canceled && len(responses) == 0 {
		log.Error().Err(err).Msg("failed to wait for results")
		return results, fmt.Errorf("failed to wait for results: %w", err)
	}

	for _, response := range responses {
		if response.Error != nil {
			if !input.StopAfterValid || !errors.Is(response.Error, context.Canceled) {
				results.Errors = append(results.Errors, response.Error)
			}
			continue
		}
		if response.History != nil && response.History.ID != 0 {
			results.Responses = append(results.Responses, response.History)
		}
	}

	results.Stopped = validFound && input.StopAfterValid
	return results, nil
}

type DiscoverAndCreateIssueInput struct {
	DiscoveryInput
	ValidationFunc   ValidationFunc
	IssueCode        db.IssueCode
	SeverityOverride string
}

type DiscoverAndCreateIssueResults struct {
	DiscoverResults
	Issues []db.Issue `json:"issues"`
	Errors []error    `json:"errors,omitempty"`
}

func DiscoverAndCreateIssue(input DiscoverAndCreateIssueInput) (DiscoverAndCreateIssueResults, error) {
	if input.ValidationFunc == nil {
		input.ValidationFunc = DefaultValidationFunc
	}

	maxPaths := input.ScanMode.MaxDiscoveryPathsPerModule()
	if maxPaths > 0 && len(input.Paths) > maxPaths {
		log.Debug().Str("scan_mode", input.ScanMode.String()).Int("max_paths", maxPaths).Int("paths", len(input.Paths)).Msg("Too many discovery module paths for this scan mode, truncating")
		input.Paths = input.Paths[:maxPaths]
	}

	results, err := DiscoverPaths(input.DiscoveryInput)
	if err != nil {
		log.Warn().Err(err).Interface("input", input).Msg("discovery failed")
		return DiscoverAndCreateIssueResults{DiscoverResults: results}, fmt.Errorf("discovery failed: %w", err)
	}

	output := DiscoverAndCreateIssueResults{
		DiscoverResults: results,
		Issues:          make([]db.Issue, 0, len(results.Responses)),
		Errors:          make([]error, 0),
	}

	for _, history := range results.Responses {
		passed, message, confidence := input.ValidationFunc(history)
		if passed {
			issue, err := db.CreateIssueFromHistoryAndTemplate(
				history,
				input.IssueCode,
				message,
				confidence,
				input.SeverityOverride,
				&input.HistoryCreationOptions.WorkspaceID,
				&input.HistoryCreationOptions.TaskID,
				&input.HistoryCreationOptions.TaskJobID,
			)
			if err != nil {
				output.Errors = append(output.Errors, fmt.Errorf("failed to create issue: %w", err))
				continue
			}
			output.Issues = append(output.Issues, issue)
		}
	}

	return output, nil
}
