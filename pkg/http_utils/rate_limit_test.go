package http_utils

import (
	"math/rand"
	"net/http"
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
