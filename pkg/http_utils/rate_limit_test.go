package http_utils

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type FastMockClient struct{}

func (m *FastMockClient) Do(req *http.Request) (*http.Response, error) {
	// Random delay between 5ms and 50ms
	delay := time.Millisecond * time.Duration(rand.Intn(46)+5)
	time.Sleep(delay)
	return &http.Response{}, nil
}

type SlowMockClient struct{}

func (m *SlowMockClient) Do(req *http.Request) (*http.Response, error) {
	// Random delay between 1s and 2s
	delay := time.Second * time.Duration(rand.Intn(2)+1)
	time.Sleep(delay)
	return &http.Response{}, nil
}

func TestFastResponse(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)
	initialRate := limiter.tokenBucket.rate

	// Simulate fast responses
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		limiter.AddRequest(req)
	}

	client := &FastMockClient{}
	go limiter.ProcessQueue(client)

	// Allow some time for requests to be processed
	time.Sleep(1 * time.Second)

	// Check if rate has been increased due to fast responses
	assert.True(t, limiter.tokenBucket.rate > initialRate)
}

func TestSlowResponse(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)
	initialRate := limiter.tokenBucket.rate

	// Simulate slow responses
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		limiter.AddRequest(req)
	}

	client := &SlowMockClient{}
	go limiter.ProcessQueue(client)

	// Allow some time for requests to be processed
	time.Sleep(4 * time.Second)

	// Check if rate has been decreased due to slow responses
	assert.True(t, limiter.tokenBucket.rate < initialRate)
}

type SteadyMockClient struct {
	delay time.Duration
}

func (m *SteadyMockClient) Do(req *http.Request) (*http.Response, error) {
	time.Sleep(m.delay)
	return &http.Response{}, nil
}

func TestQueueDepletionWithSteadyClient(t *testing.T) {
	const requestDelay = 200 * time.Millisecond
	const rate = 1.0
	const maxTokens = 10.0
	const numRequests = 10

	limiter := NewHostRateLimiter("example.com", rate, maxTokens)

	for i := 0; i < numRequests; i++ {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		limiter.AddRequest(req)
	}

	client := &SteadyMockClient{delay: requestDelay}
	go limiter.ProcessQueue(client)

	expectedDepletionTime := time.Duration(numRequests) * requestDelay
	time.Sleep(expectedDepletionTime + (requestDelay / 2)) // Add a buffer time

	assert.Equal(t, 0, len(limiter.requests))
}

func TestResponseReadingFromChannel(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)

	// Number of requests to simulate
	numRequests := 5
	channels := make([]<-chan *ResponseWrapper, numRequests)

	for i := 0; i < numRequests; i++ {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		channels[i] = limiter.AddRequest(req)
	}

	client := &FastMockClient{}
	go limiter.ProcessQueue(client)

	// Collect results from the channels
	results := []ResponseWrapper{}
	for i := 0; i < numRequests; i++ {
		select {
		case res := <-channels[i]:
			results = append(results, *res)
		case <-time.After(2 * time.Second): // timeout if no response after 2 seconds
			t.Fatal("Timed out waiting for response from channel")
		}
	}

	// Assert we received the expected number of results
	assert.Equal(t, numRequests, len(results))

	// Further assertions can be done on the results, e.g., checking response times, sent time, etc.
	for _, res := range results {
		assert.NotNil(t, res.Response)
		assert.True(t, res.SentTime.After(res.QueueTime))
	}
}

func TestNoRequests(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)
	client := &FastMockClient{}

	// No requests added, just processing the queue
	go limiter.ProcessQueue(client)

	time.Sleep(1 * time.Second) // Allow some time for processing
	assert.Equal(t, 10.0, limiter.tokenBucket.rate)
}

type ErrorMockClient struct{}

func (m *ErrorMockClient) Do(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("mocked client error")
}

func TestClientErrorHandling(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)
	client := &ErrorMockClient{}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	respChan := limiter.AddRequest(req)

	go limiter.ProcessQueue(client)

	select {
	case res := <-respChan:
		assert.Nil(t, res.Response)
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for response from channel")
	}
}

func TestConcurrentRequests(t *testing.T) {
	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)
	client := &FastMockClient{}

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			limiter.AddRequest(req)
			limiter.ProcessQueue(client)
		}()
	}

	wg.Wait()
	// Check if rate has been adjusted due to fast responses
	assert.True(t, limiter.tokenBucket.rate > 10.0)
}

// func RateLimiterTestRateAdjustment(t *testing.T) {
// 	limiter := NewHostRateLimiter("example.com", 10.0, 100.0)

// 	// Simulate slow responses to trigger rate decrease
// 	clientSlow := &SlowMockClient{}
// 	for i := 0; i < 10; i++ {
// 		req, _ := http.NewRequest("GET", "http://example.com", nil)
// 		limiter.AddRequest(req)
// 		go limiter.ProcessQueue(clientSlow)
// 	}
// 	time.Sleep(7 * time.Second) // Allow some time for requests to be processed
// 	assert.True(t, limiter.tokenBucket.rate < 10.0)

// 	// Reset and simulate fast responses to trigger rate increase
// 	limiter = NewHostRateLimiter("example.com", 10.0, 100.0)
// 	clientFast := &FastMockClient{}
// 	for i := 0; i < 50; i++ {
// 		req, _ := http.NewRequest("GET", "http://example.com", nil)
// 		limiter.AddRequest(req)
// 		go limiter.ProcessQueue(clientFast)
// 	}
// 	time.Sleep(2 * time.Second) // Allow some time for requests to be processed
// 	assert.True(t, limiter.tokenBucket.rate > 10.0)
// }
