package db

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) (*Workspace, func()) {
	workspace, err := Connection.GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceCookies",
		Code:        "test-workspace-cookies-" + uuid.New().String(),
		Description: "Test workspace for cookie tests",
	})
	require.NoError(t, err)
	require.NotNil(t, workspace)

	return workspace, func() {
		Connection.db.Where("workspace_id = ?", workspace.ID).Delete(&WorkspaceCookie{})
		Connection.DeleteWorkspace(workspace.ID)
	}
}

func TestWorkspaceCookie_CRUD(t *testing.T) {
	workspace, cleanup := setupTest(t)
	defer cleanup()

	t.Run("Create", func(t *testing.T) {
		cookie := &WorkspaceCookie{
			WorkspaceID: &workspace.ID,
			Name:        "test_cookie",
			Value:       "test_value",
			Domain:      "example.com",
			Path:        "/",
			Expires:     time.Now().Add(24 * time.Hour),
			MaxAge:      86400,
			Secure:      true,
			HttpOnly:    true,
			SameSite:    "Strict",
		}
		err := Connection.CreateWorkspaceCookie(cookie)
		require.NoError(t, err)
		assert.NotEmpty(t, cookie.ID)

		retrieved, err := Connection.GetWorkspaceCookie(cookie.ID)
		require.NoError(t, err)
		assert.Equal(t, cookie.Name, retrieved.Name)
		assert.Equal(t, cookie.Value, retrieved.Value)
	})

	t.Run("Update", func(t *testing.T) {
		cookie := &WorkspaceCookie{
			WorkspaceID: &workspace.ID,
			Name:        "update_test",
			Value:       "original_value",
			Domain:      "example.com",
			Path:        "/",
		}
		err := Connection.CreateWorkspaceCookie(cookie)
		require.NoError(t, err)

		cookie.Value = "updated_value"
		err = Connection.UpdateWorkspaceCookie(cookie)
		require.NoError(t, err)

		retrieved, err := Connection.GetWorkspaceCookie(cookie.ID)
		require.NoError(t, err)
		assert.Equal(t, "updated_value", retrieved.Value)
	})

	t.Run("Delete", func(t *testing.T) {
		cookie := &WorkspaceCookie{
			WorkspaceID: &workspace.ID,
			Name:        "delete_test",
			Value:       "value",
			Domain:      "example.com",
		}
		err := Connection.CreateWorkspaceCookie(cookie)
		require.NoError(t, err)

		err = Connection.DeleteWorkspaceCookie(cookie.ID)
		require.NoError(t, err)

		_, err = Connection.GetWorkspaceCookie(cookie.ID)
		assert.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		Connection.db.Where("workspace_id = ?", workspace.ID).Delete(&WorkspaceCookie{})

		for i := 0; i < 3; i++ {
			cookie := &WorkspaceCookie{
				WorkspaceID: &workspace.ID,
				Name:        "list_test",
				Value:       "value",
				Domain:      "example.com",
			}
			err := Connection.CreateWorkspaceCookie(cookie)
			require.NoError(t, err)
		}

		filter := WorkspaceCookieFilter{
			WorkspaceID: workspace.ID,
			Domain:      "example.com",
		}
		cookies, count, err := Connection.ListWorkspaceCookies(filter)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
		assert.Len(t, cookies, 3)
	})
}

func TestWorkspaceCookieJar(t *testing.T) {
	workspace, cleanup := setupTest(t)
	defer cleanup()

	t.Run("Cookie Jar Operations", func(t *testing.T) {
		jar := NewWorkspaceCookieJar(workspace.ID)
		testURL, err := url.Parse("https://example.com")
		require.NoError(t, err)

		cookies := jar.Cookies(testURL)
		assert.Empty(t, cookies)

		newCookie := &http.Cookie{
			Name:     "test_cookie",
			Value:    "test_value",
			Domain:   "example.com",
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
		}
		jar.SetCookies(testURL, []*http.Cookie{newCookie})

		cookies = jar.Cookies(testURL)
		require.Len(t, cookies, 1)
		assert.Equal(t, "test_cookie", cookies[0].Name)
		assert.Equal(t, "test_value", cookies[0].Value)
	})

	t.Run("Domain Handling", func(t *testing.T) {
		jar := NewWorkspaceCookieJar(workspace.ID)
		Connection.db.Where("workspace_id = ?", workspace.ID).Delete(&WorkspaceCookie{})

		domain1URL, _ := url.Parse("https://domain1.com")
		domain2URL, _ := url.Parse("https://domain2.com")

		jar.SetCookies(domain1URL, []*http.Cookie{{
			Name:   "domain1_cookie",
			Value:  "value1",
			Domain: "domain1.com",
		}})
		jar.SetCookies(domain2URL, []*http.Cookie{{
			Name:   "domain2_cookie",
			Value:  "value2",
			Domain: "domain2.com",
		}})

		domain1Cookies := jar.Cookies(domain1URL)
		domain2Cookies := jar.Cookies(domain2URL)

		assert.Len(t, domain1Cookies, 1)
		assert.Len(t, domain2Cookies, 1)
		assert.Equal(t, "domain1_cookie", domain1Cookies[0].Name)
		assert.Equal(t, "domain2_cookie", domain2Cookies[0].Name)
	})
}

func TestWorkspaceCookie_Conversion(t *testing.T) {
	workspace, cleanup := setupTest(t)
	defer cleanup()

	t.Run("ToHTTPCookie", func(t *testing.T) {
		expires := time.Now().Add(24 * time.Hour)
		wsCookie := &WorkspaceCookie{
			WorkspaceID: &workspace.ID,
			Name:        "test_cookie",
			Value:       "test_value",
			Domain:      "example.com",
			Path:        "/",
			Expires:     expires,
			MaxAge:      86400,
			Secure:      true,
			HttpOnly:    true,
			SameSite:    "Strict",
		}

		httpCookie := wsCookie.ToHTTPCookie()
		assert.Equal(t, wsCookie.Name, httpCookie.Name)
		assert.Equal(t, wsCookie.Value, httpCookie.Value)
		assert.Equal(t, wsCookie.Domain, httpCookie.Domain)
		assert.Equal(t, wsCookie.Path, httpCookie.Path)
		assert.Equal(t, wsCookie.Expires.Unix(), httpCookie.Expires.Unix())
		assert.Equal(t, wsCookie.MaxAge, httpCookie.MaxAge)
		assert.Equal(t, wsCookie.Secure, httpCookie.Secure)
		assert.Equal(t, wsCookie.HttpOnly, httpCookie.HttpOnly)
		assert.Equal(t, http.SameSiteStrictMode, httpCookie.SameSite)
	})

	t.Run("SameSite Values", func(t *testing.T) {
		testCases := []struct {
			sameSite string
			expected http.SameSite
		}{
			{"Strict", http.SameSiteStrictMode},
			{"Lax", http.SameSiteLaxMode},
			{"None", http.SameSiteNoneMode},
			{"Invalid", http.SameSite(0)},
		}

		for _, tc := range testCases {
			cookie := &WorkspaceCookie{
				WorkspaceID: &workspace.ID,
				Name:        "test",
				SameSite:    tc.sameSite,
			}
			httpCookie := cookie.ToHTTPCookie()
			assert.Equal(t, tc.expected, httpCookie.SameSite, "SameSite value mismatch for %s", tc.sameSite)
		}
	})
}
