package http_utils

import (
	"github.com/rs/zerolog/log"
	"net/http"
	"sync"
	"time"
)

const (
	UPPER_THRESHOLD = 2   // 2 seconds
	LOWER_THRESHOLD = 0.3 // 300 milliseconds
	MIN_RATE        = 3   // 3 requests per second
)

type RequestExecutor interface {
	Do(req *http.Request) (*http.Response, error)
}

type ResponseWrapper struct {
	Response  *http.Response
	Request   *http.Request
	QueueTime time.Time
	SentTime  time.Time
}

type QueuedRequest struct {
	Request   *http.Request
	Response  chan *ResponseWrapper
	QueueTime time.Time
}

type HostRateLimiter struct {
	hostName               string
	tokenBucket            *TokenBucket
	rollingAvgResponseTime float64
	numResponses           int64
	requests               []*QueuedRequest
	requestMu              sync.Mutex
}

func NewHostRateLimiter(hostName string, rate float64, maxTokens float64) *HostRateLimiter {
	return &HostRateLimiter{
		hostName:    hostName,
		tokenBucket: NewTokenBucket(rate, maxTokens, MIN_RATE),
		requests:    make([]*QueuedRequest, 0),
	}
}

func (h *HostRateLimiter) AddRequest(request *http.Request) <-chan *ResponseWrapper {
	respChan := make(chan *ResponseWrapper, 1)
	h.requestMu.Lock()
	h.requests = append(h.requests, &QueuedRequest{
		Request:   request,
		Response:  respChan,
		QueueTime: time.Now(),
	})
	h.requestMu.Unlock()
	return respChan
}

func (h *HostRateLimiter) GetNextQueuedRequest() *QueuedRequest {
	h.requestMu.Lock()
	defer h.requestMu.Unlock()
	if len(h.requests) == 0 {
		return nil
	}
	req := h.requests[0]
	h.requests = h.requests[1:]
	return req
}

func (h *HostRateLimiter) RecordResponseTime(responseTime float64) {
	h.requestMu.Lock()
	defer h.requestMu.Unlock()

	// Update rolling average response time
	h.rollingAvgResponseTime = (h.rollingAvgResponseTime*float64(h.numResponses) + responseTime) / float64(h.numResponses+1)
	h.numResponses++

	// Adjust rate based on response time
	if h.rollingAvgResponseTime > UPPER_THRESHOLD {
		h.tokenBucket.AdjustRate(h.tokenBucket.rate * 0.9) // reduce rate by 10%
		log.Info().Float64("avg_response_time", h.rollingAvgResponseTime).Msgf("Reducing request concurrency rate for host %s to %f", h.hostName, h.tokenBucket.rate)
	} else if h.rollingAvgResponseTime < LOWER_THRESHOLD {
		h.tokenBucket.AdjustRate(h.tokenBucket.rate * 1.1) // increase rate by 10%
		log.Info().Float64("avg_response_time", h.rollingAvgResponseTime).Msgf("Increasing request concurrency rate for host %s to %f", h.hostName, h.tokenBucket.rate)
	}
}

func (h *HostRateLimiter) ProcessQueue(client RequestExecutor) {
	for {
		if h.tokenBucket.HasToken() {
			queuedReq := h.GetNextQueuedRequest()
			if queuedReq != nil {
				sentTime := time.Now() // Record the time of sending
				resp, err := client.Do(queuedReq.Request)
				if err != nil {
					log.Error().Err(err).Msg("Error sending request")
					continue
				}

				responseTime := time.Now().Sub(sentTime).Seconds()
				h.RecordResponseTime(responseTime)

				// Send back the response along with metadata
				queuedReq.Response <- &ResponseWrapper{
					Response:  resp,
					Request:   queuedReq.Request,
					QueueTime: queuedReq.QueueTime,
					SentTime:  sentTime,
				}
				close(queuedReq.Response)
			}
		} else {
			time.Sleep(50 * time.Millisecond) // Sleep before checking again
		}
	}
}
