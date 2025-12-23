package manual

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/projectdiscovery/rawhttp"
	"github.com/spf13/viper"
)

type Request struct {
	URL         string              `json:"url" validate:"required"`
	URI         string              `json:"uri" validate:"omitempty"`
	Method      string              `json:"method" validate:"required"`
	Headers     map[string][]string `json:"headers" validate:"required"`
	Body        string              `json:"body" validate:"omitempty"`
	HTTPVersion string              `json:"http_version" validate:"omitempty"`
}

func (r *Request) toHTTPRequest() (*http.Request, error) {
	url := r.URL
	if r.URI != "" {
		url += r.URI
	}
	req, err := http.NewRequest(r.Method, url, strings.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	req.Header = r.Headers
	return req, nil
}

type RequestOptions struct {
	FollowRedirects     bool `json:"follow_redirects"`
	MaxRedirects        int  `json:"max_redirects" validate:"min=0"`
	UpdateHostHeader    bool `json:"update_host_header"`
	UpdateContentLength bool `json:"update_content_length"`
	Timeout             int  `json:"timeout" validate:"min=0"`
}

func (o *RequestOptions) toRawHTTPOptions() *rawhttp.Options {
	requestOptions := rawhttp.DefaultOptions
	requestOptions.FollowRedirects = o.FollowRedirects
	if o.MaxRedirects == 0 && o.FollowRedirects {
		requestOptions.MaxRedirects = viper.GetInt("navigation.max_redirects")
	} else {
		requestOptions.MaxRedirects = o.MaxRedirects
	}
	requestOptions.AutomaticHostHeader = o.UpdateHostHeader
	requestOptions.AutomaticContentLength = o.UpdateContentLength
	requestOptions.ForceReadAllBody = true
	return requestOptions
}

func (o *RequestOptions) toRawHTTPPipelineOptions(host string) rawhttp.PipelineOptions {
	pipeOptions := rawhttp.DefaultPipelineOptions
	pipeOptions.AutomaticHostHeader = o.UpdateHostHeader
	pipeOptions.Host = host
	if o.Timeout > 0 {
		pipeOptions.Timeout = time.Duration(o.Timeout) * time.Second
	}

	return pipeOptions
}

// ParseRawRequest parses a raw HTTP request and returns a Request struct
func ParseRawRequest(raw string, targetURL string) (*Request, error) {
	lines := strings.Split(raw, "\n")
	if len(lines) < 1 {
		return nil, errors.New("invalid request format")
	}

	// Extract method, URI, and HTTP version
	requestLine := strings.Fields(lines[0])
	if len(requestLine) < 3 {
		return nil, errors.New("invalid request line")
	}
	method := requestLine[0]
	if method == "" {
		return nil, errors.New("missing HTTP method")
	}

	// Use the provided targetURL as the URL, and extract the URI (path) from the raw request
	rawURI := requestLine[1]
	parsedURI, err := url.Parse(rawURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI from raw request: %s", err.Error())
	}
	uri := parsedURI.Path
	if parsedURI.RawQuery != "" {
		uri += "?" + parsedURI.RawQuery
	}
	if parsedURI.Fragment != "" {
		uri += "#" + parsedURI.Fragment
	}
	httpVersion := requestLine[2]

	// Parse headers
	headers := make(map[string][]string)
	i := 1
	for ; i < len(lines) && lines[i] != ""; i++ {
		headerParts := strings.SplitN(lines[i], ":", 2)
		if len(headerParts) != 2 {
			continue
		}
		key := strings.TrimSpace(headerParts[0])
		value := strings.TrimSpace(headerParts[1])
		headers[key] = append(headers[key], value)
	}

	// Body
	body := ""
	if i+1 < len(lines) {
		body = strings.Join(lines[i+1:], "\n")
	}

	return &Request{
		URL:         targetURL,
		URI:         uri,
		Method:      method,
		Headers:     headers,
		Body:        body,
		HTTPVersion: httpVersion,
	}, nil
}

func InsertPayloadIntoRawRequest(raw string, point FuzzerInsertionPoint, payload string) string {
	before := raw[:point.Start]
	after := raw[point.End:]
	return before + payload + after
}
