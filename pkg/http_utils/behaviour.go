package http_utils

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type SiteBehavior struct {
	NotFoundReturns404 bool          `json:"not_found_returns_404"`
	NotFoundChanges    bool          `json:"not_found_changes"`
	NotFoundSamples    []*db.History `json:"not_found_samples"`
	BaseURLSample      *db.History   `json:"base_url_sample"`
	CommonHash         string        `json:"common_hash"`
	NotFoundStatusCode int           `json:"not_found_status_code"`
}

type SiteBehaviourCheckOptions struct {
	Concurrency            int
	BaseURL                string
	HistoryCreationOptions HistoryCreationOptions
	Client                 *http.Client
	Headers                map[string][]string `json:"headers" validate:"omitempty"`
}

func (o *SiteBehaviourCheckOptions) Validate() error {
	if o.BaseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}
	if o.Concurrency < 0 {
		return fmt.Errorf("concurrency cannot be negative")
	}
	return nil
}

func getNotFoundCheckPaths() []string {
	return []string{
		"da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"nonexistent-resource-" + lib.GenerateRandomString(16),
		fmt.Sprintf("random-path-%d", lib.GenerateRandInt(100000, 999999)),
		"this-path-should-not-exist-" + lib.GenerateRandomString(8),
	}
}

func CheckSiteBehavior(options SiteBehaviourCheckOptions) (*SiteBehavior, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}

	behavior := &SiteBehavior{}
	client := options.Client
	if client == nil {
		transport := CreateHttpTransport()
		transport.ForceAttemptHTTP2 = true
		client = &http.Client{
			Transport: transport,
		}
	}

	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = 5
	}

	// Get base URL response first
	baseReq, err := http.NewRequest(http.MethodGet, options.BaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create base request: %w", err)
	}

	for key, values := range options.Headers {
		for _, value := range values {
			baseReq.Header.Add(key, value)
		}
	}
	if baseReq.Header.Get("User-Agent") == "" {
		baseReq.Header.Set("User-Agent", DefaultUserAgent)
	}
	baseReq.Header.Set("Connection", "keep-alive")

	executionResult := ExecuteRequest(baseReq, RequestExecutionOptions{
		Client:                 client,
		CreateHistory:          true,
		HistoryCreationOptions: options.HistoryCreationOptions,
	})
	if executionResult.Err != nil {
		return nil, fmt.Errorf("failed to get base response: %w", executionResult.Err)
	}
	behavior.BaseURLSample = executionResult.History

	var mu sync.Mutex
	p := pool.NewWithResults[struct {
		History *db.History
		Error   error
	}]().WithContext(context.Background()).WithMaxGoroutines(concurrency)

	for _, path := range getNotFoundCheckPaths() {
		currentPath := path
		p.Go(func(ctx context.Context) (struct {
			History *db.History
			Error   error
		}, error) {
			result := struct {
				History *db.History
				Error   error
			}{}

			targetURL := lib.JoinURLPath(options.BaseURL, currentPath)
			req, err := http.NewRequest(http.MethodGet, targetURL, nil)
			if err != nil {
				result.Error = err
				return result, nil
			}

			req = req.WithContext(ctx)
			for key, values := range options.Headers {
				for _, value := range values {
					baseReq.Header.Add(key, value)
				}
			}
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", DefaultUserAgent)
			}
			req.Header.Set("Connection", "keep-alive")

			executionResult := ExecuteRequest(req, RequestExecutionOptions{
				Client:                 client,
				CreateHistory:          true,
				HistoryCreationOptions: options.HistoryCreationOptions,
			})
			if executionResult.Err != nil {
				result.Error = executionResult.Err
				return result, nil
			}

			history := executionResult.History

			result.History = history
			mu.Lock()
			behavior.NotFoundSamples = append(behavior.NotFoundSamples, history)
			mu.Unlock()

			return result, nil
		})
	}

	samples, err := p.Wait()
	if err != nil {
		return behavior, fmt.Errorf("failed waiting for samples: %w", err)
	}

	behavior.analyzeResponses()

	for _, sample := range samples {
		if sample.Error != nil {
			return behavior, fmt.Errorf("error getting sample: %w", sample.Error)
		}
	}

	return behavior, nil
}

func (b *SiteBehavior) analyzeResponses() {
	if len(b.NotFoundSamples) == 0 || b.BaseURLSample == nil {
		return
	}

	baseHash := b.BaseURLSample.ResponseHash()
	notFoundCount := 0
	uniqueHashes := make(map[string]int)
	statusCodes := make(map[int]int)
	allMatchBase := true

	for _, sample := range b.NotFoundSamples {
		hash := sample.ResponseHash()
		if hash != baseHash {
			allMatchBase = false
		}
		uniqueHashes[hash]++
		statusCodes[sample.StatusCode]++
		if sample.StatusCode == 404 {
			notFoundCount++
		}
	}

	b.NotFoundReturns404 = notFoundCount == len(b.NotFoundSamples)
	b.NotFoundChanges = !allMatchBase && len(uniqueHashes) > 1

	if allMatchBase {
		b.CommonHash = baseHash
	}

	maxCount := 0
	mostCommonStatus := 0
	for status, count := range statusCodes {
		if count > maxCount {
			maxCount = count
			mostCommonStatus = status
		}
	}
	b.NotFoundStatusCode = mostCommonStatus
}

func (b *SiteBehavior) IsNotFound(history *db.History) bool {
	if history == nil {
		log.Debug().Msg("history is nil, returning false")
		return false
	}
	if b.BaseURLSample == nil {
		log.Debug().Msg("BaseURLSample is nil, cannot determine not found status")
		return false
	}
	logger := history.Logger()

	if b.NotFoundReturns404 {
		logger.Debug().Int("status_code", history.StatusCode).Msg("NotFoundReturns404 is true, checking if status code is 404")
		return history.StatusCode == 404
	}

	body, _ := history.ResponseBody()
	bodyStr := string(body)

	if strings.Contains(strings.ToLower(history.ResponseContentType), "text/html") {
		titleRegex := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
		if matches := titleRegex.FindStringSubmatch(bodyStr); len(matches) > 1 {
			title := strings.ToLower(matches[1])
			if strings.Contains(title, "404") || strings.Contains(title, "not found") {
				logger.Debug().Str("title", matches[1]).Msg("Found 404/not found in HTML title, returning true")
				return true
			}
		}
		h1Regex := regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
		if matches := h1Regex.FindStringSubmatch(bodyStr); len(matches) > 1 {
			title := strings.ToLower(matches[1])
			if strings.Contains(title, "404") || strings.Contains(title, "not found") {
				logger.Debug().Str("title", matches[1]).Msg("Found 404/not found in H1 HTML title, returning true")
				return true
			}
		}
	}

	if history.URL != b.BaseURLSample.URL {
		logger.Debug().Str("history_url", history.URL).Str("base_url_sample", b.BaseURLSample.URL).Msg("history.URL != b.BaseURLSample.URL")
		if b.BaseURLSample.ResponseHash() == history.ResponseHash() {
			logger.Debug().Msg("history response hash matches base URL sample response hash, returning true")
			return true
		}

		sampleBody, _ := b.BaseURLSample.ResponseBody()
		if len(bodyStr) == len(string(sampleBody)) {
			logger.Debug().Msg("history response body length matches base URL sample response body length, returning true")
			return true
		}
	}

	if b.CommonHash == history.ResponseHash() {
		logger.Debug().Str("history_hash", history.ResponseHash()).Str("common_hash", b.CommonHash).Msg("history response hash matches CommonHash, returning true")
		return true
	}

	isAuthStatus := history.StatusCode == 401 || history.StatusCode == 403
	matchesBaselineStatus := b.NotFoundStatusCode == history.StatusCode
	similarityThreshold := 0.9
	if isAuthStatus && matchesBaselineStatus {
		similarityThreshold = 0.7
		logger.Debug().Int("status_code", history.StatusCode).Int("baseline_status", b.NotFoundStatusCode).Float64("threshold", similarityThreshold).Msg("Using lenient similarity threshold for auth status matching baseline")
	}

	for _, sample := range b.NotFoundSamples {
		if history.ResponseHash() == sample.ResponseHash() {
			logger.Debug().Interface("sample", sample).Str("history_hash", history.ResponseHash()).Str("sample_hash", sample.ResponseHash()).Msg("history response hash matches not found sample response hash, returning true")
			return true
		}
		sampleBody, _ := sample.ResponseBody()

		if len(bodyStr) == len(string(sampleBody)) {
			logger.Debug().Int("history_length", len(bodyStr)).Int("sample_length", len(sampleBody)).Msg("history response body length matches not found sample response body length, returning true")
			return true
		}

		similarity := lib.ComputeSimilarity(body, sampleBody)
		logger.Debug().Float64("similarity", similarity).Float64("threshold", similarityThreshold).Msg("Response similarity with not found sample")
		if similarity > similarityThreshold {
			logger.Debug().Float64("similarity", similarity).Msg("history response is similar to not found sample, returning true")
			return true
		}
	}

	if b.NotFoundChanges && history.URL != b.BaseURLSample.URL {
		sampleBody, _ := b.BaseURLSample.ResponseBody()
		similarity := lib.ComputeSimilarity(body, sampleBody)
		log.Debug().Float64("similarity", similarity).Msg("Response similarity with base URL sample")
		if similarity > 0.9 {
			log.Debug().Float64("similarity", similarity).Msg("history response is similar to base URL sample, returning true")
			return true
		}
	}

	log.Debug().Str("url", history.URL).Msg("no match found, returning false")
	return false
}
