package manual

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRawRequest(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		targetURL string
		result    *Request
		err       bool
	}{
		{
			name:      "basic GET request",
			raw:       "GET /path/to/resource?query=value HTTP/1.1\nHost: localhost\nUser-Agent: browser\n\nbody content",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/path/to/resource?query=value",
				Method:      "GET",
				Headers:     map[string][]string{"Host": {"localhost"}, "User-Agent": {"browser"}},
				Body:        "body content",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "POST request with JSON data",
			raw:       "POST /api/data HTTP/1.1\nHost: localhost\nContent-Type: application/json\n\n{\"key\":\"value\"}",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/api/data",
				Method:      "POST",
				Headers:     map[string][]string{"Host": {"localhost"}, "Content-Type": {"application/json"}},
				Body:        "{\"key\":\"value\"}",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "PUT request with XML data",
			raw:       "PUT /api/resource HTTP/1.1\nHost: localhost\nContent-Type: application/xml\n\n<resource><id>123</id></resource>",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/api/resource",
				Method:      "PUT",
				Headers:     map[string][]string{"Host": {"localhost"}, "Content-Type": {"application/xml"}},
				Body:        "<resource><id>123</id></resource>",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "DELETE request",
			raw:       "DELETE /api/resource/123 HTTP/1.1\nHost: localhost\nUser-Agent: curl\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/api/resource/123",
				Method:      "DELETE",
				Headers:     map[string][]string{"Host": {"localhost"}, "User-Agent": {"curl"}},
				Body:        "",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "request with full URL in request line",
			raw:       "GET http://example.com/resource HTTP/1.1\nHost: example.com\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource",
				Method:      "GET",
				Headers:     map[string][]string{"Host": {"example.com"}},
				Body:        "",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "request with HTTP/2 version",
			raw:       "GET /resource HTTP/2\nHost: localhost\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource",
				Method:      "GET",
				Headers:     map[string][]string{"Host": {"localhost"}},
				Body:        "",
				HTTPVersion: "HTTP/2",
			},
			err: false,
		},
		{
			name:      "request with multiple values for single header",
			raw:       "GET /resource HTTP/1.1\nHost: localhost\nAccept: text/plain\nAccept: text/html\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource",
				Method:      "GET",
				Headers:     map[string][]string{"Host": {"localhost"}, "Accept": {"text/plain", "text/html"}},
				Body:        "",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "PATCH request",
			raw:       "PATCH /resource/456 HTTP/1.1\nHost: localhost\nUser-Agent: curl\n\n{ \"name\": \"new name\" }",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource/456",
				Method:      "PATCH",
				Headers:     map[string][]string{"Host": {"localhost"}, "User-Agent": {"curl"}},
				Body:        "{ \"name\": \"new name\" }",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "HEAD request",
			raw:       "HEAD /resource HTTP/1.1\nHost: localhost\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource",
				Method:      "HEAD",
				Headers:     map[string][]string{"Host": {"localhost"}},
				Body:        "",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
		{
			name:      "request with fragment in URL",
			raw:       "GET /resource#section HTTP/1.1\nHost: localhost\n\n",
			targetURL: "http://localhost",
			result: &Request{
				URL:         "http://localhost",
				URI:         "/resource#section",
				Method:      "GET",
				Headers:     map[string][]string{"Host": {"localhost"}},
				Body:        "",
				HTTPVersion: "HTTP/1.1",
			},
			err: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRawRequest(tt.raw, tt.targetURL)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.result, result)
			}
		})
	}
}

func TestInvalidParseRawRequest(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty request",
			raw:  "",
		},
		{
			name: "missing method and URI",
			raw:  "HTTP/1.1",
		},
		{
			name: "malformed request line",
			raw:  "GETHTTP/1.1",
		},
		{
			name: "just the method",
			raw:  "GET",
		},
		{
			name: "invalid URI",
			raw:  "GET ::invalid-uri:: HTTP/1.1",
		},
		{
			name: "missing HTTP version",
			raw:  "GET /path",
		},
		{
			name: "no space after method",
			raw:  "GET/path HTTP/1.1",
		},
		{
			name: "no space before HTTP version",
			raw:  "GET /pathHTTP/1.1",
		},
		{
			name: "just newline",
			raw:  "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRawRequest(tt.raw, "http://localhost")
			assert.Error(err, "Failed on test:", tt.name)
		})
	}

}

func TestInsertPayloadIntoRawRequest(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		point    FuzzerInsertionPoint
		payload  string
		expected string
	}{
		{
			name: "insert at beginning",
			raw:  "original content",
			point: FuzzerInsertionPoint{
				Start: 0,
				End:   0,
			},
			payload:  "payload ",
			expected: "payload original content",
		},
		{
			name: "insert at end",
			raw:  "original content",
			point: FuzzerInsertionPoint{
				Start: 16,
				End:   16,
			},
			payload:  " appended",
			expected: "original content appended",
		},
		{
			name: "replace in the middle",
			raw:  "original content here",
			point: FuzzerInsertionPoint{
				Start: 9,
				End:   16,
			},
			payload:  "replacement",
			expected: "original replacement here",
		},
		{
			name: "replace entire content",
			raw:  "replace me",
			point: FuzzerInsertionPoint{
				Start: 0,
				End:   10,
			},
			payload:  "I'm new",
			expected: "I'm new",
		},
		{
			name: "insert between characters",
			raw:  "123456",
			point: FuzzerInsertionPoint{
				Start: 3,
				End:   3,
			},
			payload:  "ABC",
			expected: "123ABC456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InsertPayloadIntoRawRequest(tt.raw, tt.point, tt.payload)
			assert.Equal(t, tt.expected, result)
		})
	}
}
