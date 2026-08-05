package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindWebSocketConnections(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsconnections", FindWebSocketConnections)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestFindWebSocketConnections",
		Code:  "TestFindWebSocketConnections",
	})
	assert.Nil(t, err)

	// Test with valid parameters
	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=2&page=1&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid page parameter
	req = httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=2&page=abc&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Test with invalid page_size parameter
	req = httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/wsconnections?page_size=xyz&page=1&workspace=%d", workspace.ID),
		nil,
	)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestFindWebSocketConnectionsFilterParams(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsconnections", FindWebSocketConnections)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestFindWebSocketConnectionsFilterParams",
		Code:  "TestFindWebSocketConnectionsFilterParams",
	})
	assert.Nil(t, err)

	get := func(query string) int {
		req := httptest.NewRequest(
			"GET",
			fmt.Sprintf("/api/v1/wsconnections?workspace=%d&%s", workspace.ID, query),
			nil,
		)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
		assert.NoError(t, err)
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, get("query=lichess&min_messages=10"))
	assert.Equal(t, http.StatusOK, get("sources=Proxy,playground,ws_fuzz"))

	// The lowercase HTTP-taxonomy spellings the v2 UI used to send were silently
	// dropped, which read as "filter applied, everything matched".
	assert.Equal(t, http.StatusBadRequest, get("sources=proxy"))
	assert.Equal(t, http.StatusBadRequest, get("sources=Proxy,bogus"))

	assert.Equal(t, http.StatusBadRequest, get("min_messages=-1"))
	assert.Equal(t, http.StatusBadRequest, get("min_messages=lots"))
}

// The same max=200 tag HistoryFilter gets for free via validate.Struct has to be
// enforced by hand here. The prefixes are kept as short as possible because 201 of
// them plus the request line must fit in fasthttp's 4096 byte ReadBufferSize, which
// neither this app nor the real server (api/server.go) overrides — realistic length
// prefixes are refused while the headers are read and never reach the handler.
func TestFindWebSocketConnectionsRejectsTooManyURLPrefixes(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsconnections", FindWebSocketConnections)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "TestFindWebSocketConnectionsRejectsTooManyURLPrefixes",
		Code:  "TestFindWebSocketConnectionsRejectsTooManyURLPrefixes",
	})
	assert.Nil(t, err)

	tooMany := fmt.Sprintf("workspace=%d", workspace.ID)
	for i := 0; i < 201; i++ {
		tooMany += fmt.Sprintf("&url_prefixes=/%d", i)
	}
	req := httptest.NewRequest("GET", "/api/v1/wsconnections?"+tooMany, nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	atLimit := fmt.Sprintf("workspace=%d&url_prefixes=https://app.test/only-one", workspace.ID)
	req = httptest.NewRequest("GET", "/api/v1/wsconnections?"+atLimit, nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFindWebSocketMessages(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/wsmessages", FindWebSocketMessages)

	// Test with valid parameters
	req := httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=2&page=1", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid page parameter
	req = httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=2&page=abc", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Test with invalid page_size parameter
	req = httptest.NewRequest("GET", "/api/v1/wsmessages?page_size=xyz&page=1", nil)
	resp, _ = app.Test(req)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
