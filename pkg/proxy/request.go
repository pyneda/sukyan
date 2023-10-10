package proxy

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type InterceptedRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
	Body   string              `json:"body"`
}

func ConvertToInterceptedRequest(r *http.Request) (*InterceptedRequest, error) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return &InterceptedRequest{
		Method: r.Method,
		URL:    r.URL.String(),
		Header: r.Header,
		Body:   string(bodyBytes),
	}, nil
}

func ConvertFromInterceptedRequest(ir *InterceptedRequest) (*http.Request, error) {
	reqURL, err := url.Parse(ir.URL)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: ir.Method,
		URL:    reqURL,
		Header: ir.Header,
		Body:   ioutil.NopCloser(strings.NewReader(ir.Body)),
	}

	return req, nil
}
