package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
)

func usersHandlerApp() *fiber.App {
	app := fiber.New()
	app.Get("/users", ListUsersHandler)
	return app
}

func usersRequestStatus(t *testing.T, target string) int {
	t.Helper()
	resp, err := usersHandlerApp().Test(httptest.NewRequest("GET", target, nil), fiber.TestConfig{})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp.StatusCode
}

func TestListUsersHandlerRejectsInvalidParams(t *testing.T) {
	cases := map[string]string{
		"unknown sort field": "/users?sort_by=password_hash",
		"bad sort order":     "/users?sort_by=email&sort_order=sideways",
		"page below one":     "/users?page=0",
		"negative page":      "/users?page=-3",
		"page size zero":     "/users?page_size=0",
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			if status := usersRequestStatus(t, target); status != fiber.StatusBadRequest {
				t.Errorf("status = %d, want %d", status, fiber.StatusBadRequest)
			}
		})
	}
}

func TestListUsersHandlerAcceptsEverySortField(t *testing.T) {
	for _, field := range db.UserSortFields {
		t.Run(field, func(t *testing.T) {
			if status := usersRequestStatus(t, "/users?sort_by="+field); status == fiber.StatusBadRequest {
				t.Errorf("sort_by=%s was rejected, want it accepted", field)
			}
		})
	}
}

// The response serialises db.User directly, so the password hash must be
// unreachable by construction rather than by the handler remembering to strip it.
func TestUsersResponseNeverCarriesAPasswordHash(t *testing.T) {
	body, err := json.Marshal(UsersResponse{
		Data:  []db.User{{Email: "someone@example.com", PasswordHash: "$2a$10$averysecretbcrypthash"}},
		Count: 1,
	})
	if err != nil {
		t.Fatalf("marshalling response: %v", err)
	}

	encoded := string(body)
	if strings.Contains(encoded, "password_hash") {
		t.Error("response contains a password_hash key")
	}
	if strings.Contains(encoded, "averysecretbcrypthash") {
		t.Errorf("response leaked the hash value: %s", encoded)
	}
}
