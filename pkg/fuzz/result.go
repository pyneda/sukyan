package fuzz

import (
	"github.com/pyneda/sukyan/pkg/payloads"
	"net/http"
)

// FuzzResult used to return a result to be futher analyzed
type FuzzResult struct {
	URL      string
	Response http.Response
	Err      error
	Payload  payloads.PayloadInterface
	// StatusCode   int
	// Request      http.Request
	// ResponseSize int
}

func (r FuzzResult) CreateHistoryFromFuzzResult() {
}

type ExpectedResponse struct {
	Response http.Response
	Body     string
	BodySize int
	Err      error
}

type ExpectedResponses struct {
	Base     ExpectedResponse
	NotFound ExpectedResponse
	Errors   []ExpectedResponse
}
