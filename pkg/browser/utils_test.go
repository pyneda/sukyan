package browser

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"

	"github.com/go-rod/rod"
)

func setupMockServer() (*rod.Page, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/":
			body, _ := io.ReadAll(r.Body)
			// Specific condition for redirecting
			if string(body) == "trigger bingo" {
				http.Redirect(w, r, "/bingo", http.StatusFound)
				return
			} else {
				w.Write([]byte("received a POST request"))
			}
		case r.Method == "GET" && r.URL.Path == "/bingo":
			w.Write([]byte("bingo"))
			return
		default:
			w.Write([]byte("received a " + r.Method + " request"))
		}
	}))

	page := rod.New().MustConnect().MustPage()
	return page, server
}

func TestReplayRequestInBrowser(t *testing.T) {
	page, server := setupMockServer()
	defer server.Close()

	// Test 1: POST request that triggers redirection
	postReq, _ := http.NewRequest("POST", server.URL, bytes.NewBufferString("trigger bingo"))
	err := ReplayRequestInBrowser(page, postReq)
	assert.Nil(t, err)
	assert.Equal(t, "bingo", page.MustElement("body").MustText())

	// Test 2: Normal POST request
	normalPostReq, _ := http.NewRequest("POST", server.URL, bytes.NewBufferString("normal post"))
	err = ReplayRequestInBrowser(page, normalPostReq)
	assert.Nil(t, err)
	assert.Equal(t, "received a POST request", page.MustElement("body").MustText())

	// Test 3: GET request
	getReq, _ := http.NewRequest("GET", server.URL, nil)
	err = ReplayRequestInBrowser(page, getReq)
	assert.Nil(t, err)
	assert.Equal(t, "received a GET request", page.MustElement("body").MustText())

	// Test 4: PUT request
	putReq, _ := http.NewRequest(http.MethodPut, server.URL, nil)
	err = ReplayRequestInBrowser(page, putReq)
	assert.Nil(t, err)
	assert.Equal(t, "received a PUT request", page.MustElement("body").MustText())
}

func TestReplayRequestInBrowserAndCreateHistory(t *testing.T) {
	page, server := setupMockServer()
	defer server.Close()
	// Test 1: POST request that triggers redirection
	postReq, _ := http.NewRequest("POST", server.URL, bytes.NewBufferString("trigger bingo"))
	history, err := ReplayRequestInBrowserAndCreateHistory(page, postReq, 0, 0, 0, "Testing ReplayRequestInBrowserAndCreateHistory", db.SourceScanner)
	assert.Nil(t, err)
	assert.Equal(t, "bingo", page.MustElement("body").MustText())
	assert.Equal(t, history.Method, "POST")
	assert.Equal(t, true, strings.Contains(string(history.RawResponse), "bingo"))
	// assert.Equal(t, true, strings.Contains(string(history.RawRequest), "bingo"))

	// Test 2: Normal POST request
	normalPostReq, _ := http.NewRequest("POST", server.URL, bytes.NewBufferString("normal post"))
	history, err = ReplayRequestInBrowserAndCreateHistory(page, normalPostReq, 0, 0, 0, "Testing ReplayRequestInBrowserAndCreateHistory", db.SourceScanner)
	assert.Nil(t, err)
	assert.Equal(t, "received a POST request", page.MustElement("body").MustText())
	assert.Equal(t, history.Method, "POST")

	// Test 3: GET request
	getReq, _ := http.NewRequest("GET", server.URL, nil)
	history, err = ReplayRequestInBrowserAndCreateHistory(page, getReq, 0, 0, 0, "Testing ReplayRequestInBrowserAndCreateHistory", db.SourceScanner)
	assert.Nil(t, err)
	assert.Equal(t, "received a GET request", page.MustElement("body").MustText())
	assert.Equal(t, history.Method, "GET")
	assert.Equal(t, history.StatusCode, 200)

	// Test 4: PUT request
	putReq, _ := http.NewRequest(http.MethodPut, server.URL, nil)
	history, err = ReplayRequestInBrowserAndCreateHistory(page, putReq, 0, 0, 0, "Testing ReplayRequestInBrowserAndCreateHistory", db.SourceScanner)
	assert.Nil(t, err)
	assert.Equal(t, "received a PUT request", page.MustElement("body").MustText())
	assert.Equal(t, history.Method, "PUT")
	assert.Equal(t, history.StatusCode, 200)
	assert.Equal(t, true, strings.Contains(string(history.RawResponse), "received a PUT request"))

	// assert.Equal(t, "received a PUT request", string(history.ResponseBody))

}
