package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureAPIDefinitionFilter runs buildAPIDefinitionFilter against a real Fiber
// request so the query string is parsed exactly as it is in production, without
// needing a database. It returns the parsed filter, the HTTP status the handler
// would answer with, and the validation error, if any.
func captureAPIDefinitionFilter(t *testing.T, queryString string) (db.APIDefinitionFilter, int, error) {
	t.Helper()

	var (
		filter    db.APIDefinitionFilter
		filterErr error
	)

	app := fiber.New()
	app.Get("/api-definitions", func(c *fiber.Ctx) error {
		filter, filterErr = buildAPIDefinitionFilter(c)
		if filterErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(NewErrorResponse("Invalid filter", filterErr.Error()))
		}
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/api-definitions?"+queryString, nil))
	require.NoError(t, err)

	return filter, resp.StatusCode, filterErr
}

func TestBuildAPIDefinitionFilterDefaults(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t, "")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "", filter.Query)
	assert.Equal(t, uint(0), filter.WorkspaceID)
	assert.Nil(t, filter.ScanID)
	assert.Nil(t, filter.AutoDiscovered)
	assert.Empty(t, filter.Types)
	assert.Empty(t, filter.Statuses)
	assert.Equal(t, "", filter.SortBy)
	assert.Equal(t, "", filter.SortOrder)
	assert.Equal(t, 1, filter.Pagination.Page)
	assert.Equal(t, 20, filter.Pagination.PageSize)
}

// The handler used to read only workspace_id/scan_id/type/status/page/page_size,
// so query, sort_by, sort_order and auto_discovered were accepted by the URL and
// then dropped. Every one of them must reach the filter.
func TestBuildAPIDefinitionFilterReadsEveryParameter(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t,
		"workspace_id=7&scan_id=42&query=%20petstore%20&type=openapi&status=parsed&auto_discovered=true&sort_by=name&sort_order=asc&page=3&page_size=50")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "petstore", filter.Query, "query must be trimmed and forwarded")
	assert.Equal(t, uint(7), filter.WorkspaceID)
	require.NotNil(t, filter.ScanID)
	assert.Equal(t, uint(42), *filter.ScanID)
	assert.Equal(t, []db.APIDefinitionType{db.APIDefinitionTypeOpenAPI}, filter.Types)
	assert.Equal(t, []db.APIDefinitionStatus{db.APIDefinitionStatusParsed}, filter.Statuses)
	require.NotNil(t, filter.AutoDiscovered)
	assert.True(t, *filter.AutoDiscovered)
	assert.Equal(t, "name", filter.SortBy)
	assert.Equal(t, "asc", filter.SortOrder)
	assert.Equal(t, 3, filter.Pagination.Page)
	assert.Equal(t, 50, filter.Pagination.PageSize)
}

func TestBuildAPIDefinitionFilterAcceptsMultipleTypesAndStatuses(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t, "type=openapi,GraphQL&status=parsed,%20failed")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, []db.APIDefinitionType{db.APIDefinitionTypeOpenAPI, db.APIDefinitionTypeGraphQL}, filter.Types)
	assert.Equal(t, []db.APIDefinitionStatus{db.APIDefinitionStatusParsed, db.APIDefinitionStatusFailed}, filter.Statuses)
}

func TestBuildAPIDefinitionFilterAutoDiscoveredFalseIsNotDropped(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t, "auto_discovered=false")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	require.NotNil(t, filter.AutoDiscovered, "auto_discovered=false must filter, not be treated as unset")
	assert.False(t, *filter.AutoDiscovered)
}

// The db layer ignores SortOrder unless SortBy names a whitelisted column, so a
// bare sort_order has to be anchored to the default column or it silently does
// nothing.
func TestBuildAPIDefinitionFilterAnchorsBareSortOrder(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t, "sort_order=ASC")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "created_at", filter.SortBy)
	assert.Equal(t, "asc", filter.SortOrder)
}

func TestBuildAPIDefinitionFilterRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name        string
		queryString string
		wantMessage string
	}{
		{
			name:        "unknown sort column",
			queryString: "sort_by=raw_definition",
			wantMessage: "sort_by must be one of: id, created_at, updated_at, name, type, status, endpoint_count",
		},
		{
			name:        "sql injection attempt in sort column",
			queryString: "sort_by=name%3B+drop+table+api_definitions",
			wantMessage: "sort_by must be one of: id, created_at, updated_at, name, type, status, endpoint_count",
		},
		{
			name:        "unknown sort order",
			queryString: "sort_by=name&sort_order=sideways",
			wantMessage: "sort_order must be either asc or desc",
		},
		{
			name:        "unknown type",
			queryString: "type=rest",
			wantMessage: "type must be one of: openapi, graphql, wsdl",
		},
		{
			name:        "unknown status in a list",
			queryString: "status=parsed,pending",
			wantMessage: "status must be one of: parsed, scanning, completed, failed",
		},
		{
			name:        "non boolean auto_discovered",
			queryString: "auto_discovered=maybe",
			wantMessage: "auto_discovered must be a boolean",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, status, err := captureAPIDefinitionFilter(t, tc.queryString)

			require.Error(t, err)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, tc.wantMessage, err.Error())
		})
	}
}

// A negative page would reach db.Pagination.GetData as a negative offset; clamp
// it at the boundary so paging can never run backwards past the first row.
func TestBuildAPIDefinitionFilterClampsNegativePage(t *testing.T) {
	filter, status, err := captureAPIDefinitionFilter(t, "page=-3")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, filter.Pagination.Page)
}

// listAPIDefinitionsTestWorkspace creates a throwaway workspace seeded with one
// definition of each type so list filters can be asserted in isolation.
func listAPIDefinitionsTestWorkspace(t *testing.T) uint {
	t.Helper()

	conn := db.Connection()
	suffix := fmt.Sprintf("_%d", apiDefinitionListTestCounter.Add(1))
	workspace, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Title: "api-definitions-list-" + t.Name() + suffix,
		Code:  "apidefs_" + sanitizeForCode(t.Name()) + suffix,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(workspace.ID) })

	seed := []*db.APIDefinition{
		{
			WorkspaceID: workspace.ID,
			Name:        "Petstore OpenAPI",
			Type:        db.APIDefinitionTypeOpenAPI,
			Status:      db.APIDefinitionStatusParsed,
			SourceURL:   "https://petstore.example.com/openapi.json",
			BaseURL:     "https://petstore.example.com",
		},
		{
			WorkspaceID: workspace.ID,
			Name:        "Billing GraphQL",
			Type:        db.APIDefinitionTypeGraphQL,
			Status:      db.APIDefinitionStatusCompleted,
			SourceURL:   "https://billing.example.com/graphql",
			BaseURL:     "https://billing.example.com",
		},
		{
			WorkspaceID:    workspace.ID,
			Name:           "Legacy WSDL",
			Type:           db.APIDefinitionTypeWSDL,
			Status:         db.APIDefinitionStatusParsed,
			SourceURL:      "https://legacy.example.com/service?wsdl",
			BaseURL:        "https://legacy.example.com",
			AutoDiscovered: true,
		},
	}
	for _, definition := range seed {
		_, err := conn.CreateAPIDefinition(definition)
		require.NoError(t, err)
	}

	return workspace.ID
}

var apiDefinitionListTestCounter atomic.Uint64

func listAPIDefinitions(t *testing.T, queryString string) (*http.Response, APIDefinitionListResponse) {
	t.Helper()

	app := fiber.New()
	app.Get("/api-definitions", ListAPIDefinitions)

	resp, err := app.Test(httptest.NewRequest("GET", "/api-definitions?"+queryString, nil), 10000)
	require.NoError(t, err)

	var body APIDefinitionListResponse
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	}
	return resp, body
}

func definitionNames(items []*db.APIDefinition) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}

// End-to-end proof that the parameters the handler now reads actually reach the
// SQL the db layer builds — the parsing tests alone would still pass if the
// filter were dropped between the handler and db.ListAPIDefinitions.
func TestListAPIDefinitionsAppliesEveryFilter(t *testing.T) {
	workspaceID := listAPIDefinitionsTestWorkspace(t)
	base := fmt.Sprintf("workspace_id=%d&page_size=50", workspaceID)

	t.Run("query searches name and urls", func(t *testing.T) {
		resp, body := listAPIDefinitions(t, base+"&query=petstore")

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int64(1), body.Count)
		assert.Equal(t, []string{"Petstore OpenAPI"}, definitionNames(body.Items))
	})

	t.Run("query matches the source url too", func(t *testing.T) {
		_, body := listAPIDefinitions(t, base+"&query=billing.example.com")

		assert.Equal(t, []string{"Billing GraphQL"}, definitionNames(body.Items))
	})

	t.Run("sort_by and sort_order order the page", func(t *testing.T) {
		_, ascending := listAPIDefinitions(t, base+"&sort_by=name&sort_order=asc")
		assert.Equal(t, []string{"Billing GraphQL", "Legacy WSDL", "Petstore OpenAPI"}, definitionNames(ascending.Items))

		_, descending := listAPIDefinitions(t, base+"&sort_by=name&sort_order=desc")
		assert.Equal(t, []string{"Petstore OpenAPI", "Legacy WSDL", "Billing GraphQL"}, definitionNames(descending.Items))
	})

	t.Run("auto_discovered splits the workspace", func(t *testing.T) {
		_, discovered := listAPIDefinitions(t, base+"&auto_discovered=true")
		assert.Equal(t, []string{"Legacy WSDL"}, definitionNames(discovered.Items))

		_, imported := listAPIDefinitions(t, base+"&auto_discovered=false&sort_by=name&sort_order=asc")
		assert.Equal(t, []string{"Billing GraphQL", "Petstore OpenAPI"}, definitionNames(imported.Items))
	})

	t.Run("type accepts a list", func(t *testing.T) {
		_, body := listAPIDefinitions(t, base+"&type=openapi,graphql&sort_by=name&sort_order=asc")

		assert.Equal(t, int64(2), body.Count)
		assert.Equal(t, []string{"Billing GraphQL", "Petstore OpenAPI"}, definitionNames(body.Items))
	})

	t.Run("status filters", func(t *testing.T) {
		_, body := listAPIDefinitions(t, base+"&status=completed")

		assert.Equal(t, []string{"Billing GraphQL"}, definitionNames(body.Items))
	})

	t.Run("invalid sort column is rejected", func(t *testing.T) {
		resp, _ := listAPIDefinitions(t, base+"&sort_by=raw_definition")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// The sort whitelist is a copy of the one in db/api_definition.go; if the db
// layer grows or loses a column the two drift and the API either rejects a
// valid sort or forwards one the db layer will silently discard. The oneof tag
// on db.APIDefinitionFilter.SortBy is the db layer's own declaration of that
// set, so validating against it catches the drift.
func TestAPIDefinitionSortFieldsMatchDBLayer(t *testing.T) {
	validate := validator.New()

	for _, field := range apiDefinitionSortFields {
		filter := db.APIDefinitionFilter{
			Pagination: db.Pagination{Page: 1, PageSize: 1},
			SortBy:     field,
			SortOrder:  "asc",
		}
		require.NoError(t, validate.Struct(filter), "sort field %q rejected by the db filter contract", field)
	}
}
