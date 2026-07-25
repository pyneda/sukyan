package reflection

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

// canInjectCanary is a hand-maintained mirror of buildTestRequest's capabilities.
// If they drift, a whole class of insertion points silently reports
// "not reflected" and callers skip real work. This asserts they agree by
// checking whether the canary actually made it into the request.
func TestCanInjectCanaryMatchesBuildTestRequest(t *testing.T) {
	const canary = "zqxjcanary123"

	// Each case models a realistic insertion point of its type: a urlpath point's
	// name IS a path segment, a cookie point only exists when the request carries
	// that cookie, and so on. Anything less would test a fixture, not the builder.
	cases := []struct {
		name        string
		pointType   string
		pointName   string
		contentType string
		body        string
	}{
		{name: "query parameter", pointType: "parameter", pointName: "q"},
		{name: "form body", pointType: "body", pointName: "q", contentType: "application/x-www-form-urlencoded", body: "q=orig"},
		{name: "json body", pointType: "body", pointName: "q", contentType: "application/json", body: `{"q":"orig"}`},
		{name: "multipart body", pointType: "body", pointName: "q", contentType: "multipart/form-data; boundary=x", body: "--x--"},
		{name: "header", pointType: "header", pointName: "User-Agent"},
		{name: "cookie", pointType: "cookie", pointName: "sid"},
		{name: "urlpath", pointType: "urlpath", pointName: "b"},
		{name: "fullbody xml", pointType: "fullbody", pointName: "xml", contentType: "application/xml", body: "<a>orig</a>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &db.History{
				URL:                "http://example.com/a/b?q=orig",
				Method:             "POST",
				RequestContentType: tc.contentType,
				RawRequest: []byte("POST /a/b?q=orig HTTP/1.1\r\nHost: example.com\r\n" +
					"User-Agent: sukyan-test\r\nCookie: sid=orig\r\n" +
					"Content-Type: " + tc.contentType + "\r\n\r\n" + tc.body),
			}

			point := InsertionPointInfo{Name: tc.pointName, Type: tc.pointType, OriginalData: "orig"}

			req, err := buildTestRequest(item, point, canary)
			if err != nil {
				// A build error means no canary was sent either way; the predicate
				// must not claim the point was measured.
				if canInjectCanary(point, tc.contentType) {
					t.Fatalf("canInjectCanary said measurable but buildTestRequest failed: %v", err)
				}
				return
			}

			sent := req.URL.String()
			for name, values := range req.Header {
				sent += "\n" + name + ": " + strings.Join(values, ",")
			}
			if req.Body != nil {
				b, _ := io.ReadAll(req.Body)
				sent += "\n" + string(b)
			}

			actuallyInjected := strings.Contains(sent, canary)
			predicted := canInjectCanary(point, tc.contentType)

			if actuallyInjected != predicted {
				t.Errorf("drift for %s: canInjectCanary=%v but canary present in request=%v",
					tc.pointType, predicted, actuallyInjected)
			}
		})
	}
}

// End-to-end proof that the canary reaches header, cookie and URL-path sinks:
// against a server that reflects all of them, IsReflected must be true. Before
// the canary was injected for these types this returned false for every one of
// them, which made "not reflected" indistinguishable from "never probed".
func TestAnalyzeReflectionDetectsHeaderCookieAndPathSinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie := ""
		if c, err := r.Cookie("sid"); err == nil {
			cookie = c.Value
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body>path=%s ua=%s cookie=%s q=%s</body></html>",
			r.URL.Path, r.Header.Get("User-Agent"), cookie, r.URL.Query().Get("q"))
	}))
	defer srv.Close()

	cases := []struct{ name, pointType, pointName string }{
		{"query parameter", "parameter", "q"},
		{"url path", "urlpath", "b"},
		{"header", "header", "User-Agent"},
		{"cookie", "cookie", "sid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &db.History{
				URL:    srv.URL + "/a/b?q=orig",
				Method: "GET",
				RawRequest: []byte("GET /a/b?q=orig HTTP/1.1\r\nHost: " + srv.Listener.Addr().String() +
					"\r\nUser-Agent: sukyan-test\r\nCookie: sid=orig\r\n\r\n"),
			}
			point := InsertionPointInfo{Name: tc.pointName, Type: tc.pointType, OriginalData: "orig"}

			analysis, err := AnalyzeReflection(item, point, AnalysisOptions{})
			if err != nil {
				t.Fatalf("AnalyzeReflection: %v", err)
			}
			if !analysis.Measured {
				t.Fatalf("expected Measured=true for %s", tc.pointType)
			}
			if !analysis.IsReflected {
				t.Fatalf("expected IsReflected=true for a %s sink that echoes the value", tc.pointType)
			}
		})
	}
}
