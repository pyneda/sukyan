package http_utils

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/sourcegraph/conc/pool"
)

type SiteBehavior struct {
	NotFoundReturns404 bool                `json:"not_found_returns_404"`
	NotFoundChanges    bool                `json:"not_found_changes"`
	NotFoundSamples    []*db.History       `json:"not_found_samples"`
	BaseURLSample      *db.History         `json:"base_url_sample"`
	CommonBodyParts    []string            `json:"common_body_parts"`
	CommonHeaders      map[string][]string `json:"common_headers"`
	CommonHash         string              `json:"common_hash"`
}

type SiteBehaviourCheckOptions struct {
	Concurrency            int
	BaseURL                string
	HistoryCreationOptions HistoryCreationOptions
	Client                 *http.Client
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

	baseResp, err := SendRequest(client, baseReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get base response: %w", err)
	}
	baseData, _, err := ReadFullResponse(baseResp, false)
	if err != nil {
		return nil, fmt.Errorf("failed to read base response: %w", err)
	}
	behavior.BaseURLSample, err = CreateHistoryFromHttpResponse(baseResp, baseData, options.HistoryCreationOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create base history: %w", err)
	}

	// Create pool for parallel requests
	var mu sync.Mutex
	p := pool.NewWithResults[struct {
		History *db.History
		Error   error
	}]().WithContext(context.Background()).WithMaxGoroutines(concurrency)

	// Test not found paths
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
			req.Header.Set("User-Agent", DefaultUserAgent)
			req.Header.Set("Connection", "keep-alive")

			resp, err := SendRequest(client, req)
			if err != nil {
				result.Error = err
				return result, nil
			}

			respData, _, err := ReadFullResponse(resp, false)
			if err != nil {
				result.Error = err
				return result, nil
			}

			history, err := CreateHistoryFromHttpResponse(resp, respData, options.HistoryCreationOptions)
			if err != nil {
				result.Error = err
				return result, nil
			}

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
	allMatchBase := true

	for _, sample := range b.NotFoundSamples {
		hash := sample.ResponseHash()
		if hash != baseHash {
			allMatchBase = false
		}
		uniqueHashes[hash]++
		if sample.StatusCode == 404 {
			notFoundCount++
		}
	}

	b.NotFoundReturns404 = notFoundCount == len(b.NotFoundSamples)
	b.NotFoundChanges = !allMatchBase && len(uniqueHashes) > 1

	if allMatchBase {
		b.CommonHash = baseHash
	}
}

func (b *SiteBehavior) IsNotFound(history *db.History) bool {
	if history == nil {
		return false
	}

	if b.CommonHash == b.BaseURLSample.ResponseHash() {
		return true
	}

	if b.NotFoundReturns404 {
		return history.StatusCode == 404
	}

	for _, sample := range b.NotFoundSamples {
		if history.ResponseHash() == sample.ResponseHash() {
			return true
		}
	}

	return false
}
