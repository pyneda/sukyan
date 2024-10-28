package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/sourcegraph/conc/pool"
)

const (
	DefaultConcurrency = 10
	DefaultTimeout     = 10
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
	Method                 string            `json:"method"`
	Body                   string            `json:"body"`
	Concurrency            int               `json:"concurrency"`
	Timeout                int               `json:"timeout"`
	Paths                  []string          `json:"paths"`
	Headers                map[string]string `json:"headers"`
	StopAfterValid         bool              `json:"stop_after_valid"`
	ValidationFunc         ValidationFunc    `json:"-"`
}

// Validate checks and sets default values for DiscoveryInput
func (d *DiscoveryInput) Validate() error {
	if d.URL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if d.Concurrency == 0 {
		d.Concurrency = DefaultConcurrency
	}
	if d.Timeout == 0 {
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

type DiscoverResults struct {
	Responses []*db.History `json:"responses"`
	Errors    []error       `json:"errors,omitempty"`
	Stopped   bool          `json:"stopped,omitempty"`
}

func joinURLPath(baseURL, urlPath string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/" + strings.TrimPrefix(urlPath, "/")
	}
	u.Path = path.Join(u.Path, urlPath)
	return u.String()
}

func setDefaultHeaders(req *http.Request, hasBody bool) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "keep-alive")
	}
	if hasBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

func DiscoverPaths(input DiscoveryInput) (DiscoverResults, error) {
	if err := input.Validate(); err != nil {
		return DiscoverResults{}, fmt.Errorf("invalid input: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	transport := http_utils.CreateHttpTransport()
	transport.ForceAttemptHTTP2 = true
	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(input.Timeout) * time.Second,
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
				return result, ctx.Err()
			default:
			}

			if input.StopAfterValid {
				mu.Lock()
				if validFound {
					mu.Unlock()
					return result, nil
				}
				mu.Unlock()
			}

			targetURL := joinURLPath(input.URL, currentPath)

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

			for key, value := range input.Headers {
				request.Header.Set(key, value)
			}
			setDefaultHeaders(request, input.Body != "")

			response, err := http_utils.SendRequest(client, request)
			if err != nil {
				result.Error = fmt.Errorf("failed to send request: %w", err)
				return result, nil
			}
			defer response.Body.Close()

			responseData, _, err := http_utils.ReadFullResponse(response, false)
			if err != nil {
				result.Error = fmt.Errorf("error reading response: %w", err)
				return result, nil
			}

			history, err := http_utils.CreateHistoryFromHttpResponse(response, responseData, input.HistoryCreationOptions)
			if err != nil {
				result.Error = fmt.Errorf("error creating history: %w", err)
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

			result.History = history
			return result, nil
		})
	}

	responses, err := p.Wait()
	if err != nil && err != context.Canceled && len(responses) == 0 {
		return results, fmt.Errorf("failed to wait for results: %w", err)
	}

	for _, response := range responses {
		if response.Error != nil {
			results.Errors = append(results.Errors, response.Error)
			continue
		}
		if response.History != nil && response.History.ID != 0 {
			results.Responses = append(results.Responses, response.History)
		}
	}

	results.Stopped = validFound
	return results, nil
}

type DiscoverAndCreateIssueInput struct {
	DiscoveryInput
	ValidationFunc ValidationFunc
	IssueCode      db.IssueCode
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

	results, err := DiscoverPaths(input.DiscoveryInput)
	if err != nil {
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
				"",
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
