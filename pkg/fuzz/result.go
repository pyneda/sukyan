package fuzz

import (
	"net/http"
	"github.com/pyneda/sukyan/pkg/payloads"
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
