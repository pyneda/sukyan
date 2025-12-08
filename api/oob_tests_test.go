package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestFindOOBTests(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Post("/oob-tests", FindOOBTests)

	// Create test OOB tests
	test1 := db.OOBTest{
		Code:              "sql_injection",
		TestName:          "SQL Injection Test",
		Target:            "https://example.com/api",
		InteractionDomain: "test1.example.com",
		InteractionFullID: "test-id-1",
		Payload:           "' OR 1=1--",
		InsertionPoint:    "query_parameter",
		WorkspaceID:       &workspace.ID,
		Note:              "Test note 1",
	}
	createdTest1, err := db.Connection().CreateOOBTest(test1)
	assert.Nil(t, err)

	test2 := db.OOBTest{
		Code:              "xss",
		TestName:          "XSS Test",
		Target:            "https://example.com/form",
		InteractionDomain: "test2.example.com",
		InteractionFullID: "test-id-2",
		Payload:           "<script>alert('xss')</script>",
		InsertionPoint:    "body_parameter",
		WorkspaceID:       &workspace.ID,
		Note:              "Test note 2",
	}
	createdTest2, err := db.Connection().CreateOOBTest(test2)
	assert.Nil(t, err)

	// Test basic filtering
	t.Run("Basic filtering with workspace", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "count")
	})

	// Test search functionality
	t.Run("Search by query", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			Query:       "SQL",
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test filtering by codes
	t.Run("Filter by codes", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			Codes:       []string{"sql_injection"},
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test filtering by test names
	t.Run("Filter by test names", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			TestNames:   []string{"SQL Injection Test"},
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test sorting
	t.Run("Sort by test name ascending", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			SortBy:      "test_name",
			SortOrder:   "asc",
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test date filtering
	t.Run("Filter by created date", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)

		filter := db.OOBTestsFilter{
			WorkspaceID:  workspace.ID,
			CreatedAfter: &yesterday,
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test pagination
	t.Run("Pagination", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 1,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)

		data := result["data"].([]interface{})
		assert.LessOrEqual(t, len(data), 1)
	})

	// Clean up
	db.Connection().DB().Delete(&createdTest1)
	db.Connection().DB().Delete(&createdTest2)
	db.Connection().DB().Delete(&workspace)
}

func TestFindOOBTestsValidation(t *testing.T) {
	app := fiber.New()
	app.Post("/oob-tests", FindOOBTests)

	// Test invalid JSON
	t.Run("Invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "Invalid request body", result.Error)
	})

	// Test validation errors
	t.Run("Validation errors", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			Query: string(make([]byte, 600)), // Exceeds max length
			Pagination: db.Pagination{
				Page:     0, // Invalid page
				PageSize: 0, // Invalid page size
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "Filter validation failed", result.Error)
	})

	// Test invalid workspace
	t.Run("Invalid workspace", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: 999999, // Non-existent workspace
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "Invalid workspace", result.Error)
	})
}

func TestFindOOBTestsDefaults(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Post("/oob-tests", FindOOBTests)

	// Test default pagination values
	t.Run("Default pagination values", func(t *testing.T) {
		filter := db.OOBTestsFilter{
			WorkspaceID: workspace.ID,
			// No pagination specified
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Clean up
	db.Connection().DB().Delete(&workspace)
}

func TestGetOOBTestDetail(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Get("/oob-tests/:id", GetOOBTestDetail)

	// Create a test OOB test
	test := db.OOBTest{
		Code:              "command_injection",
		TestName:          "Command Injection Test",
		Target:            "https://example.com/exec",
		InteractionDomain: "detail-test.example.com",
		InteractionFullID: "detail-test-id",
		Payload:           "$(curl oob.example.com)",
		InsertionPoint:    "header",
		WorkspaceID:       &workspace.ID,
		Note:              "Detailed test note",
	}
	createdTest, err := db.Connection().CreateOOBTest(test)
	assert.Nil(t, err)

	// Test successful retrieval
	t.Run("Get existing OOB test", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/oob-tests/"+strconv.Itoa(int(createdTest.ID)), nil)
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result db.OOBTest
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, createdTest.ID, result.ID)
		assert.Equal(t, "Command Injection Test", result.TestName)
		assert.Equal(t, "command_injection", string(result.Code))
	})

	// Test non-existent ID
	t.Run("Get non-existent OOB test", func(t *testing.T) {
		// Use a very high ID that's unlikely to exist
		req := httptest.NewRequest("GET", "/oob-tests/999999999", nil)
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "OOB test not found", result.Error)
	})

	// Test invalid ID format
	t.Run("Invalid ID format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/oob-tests/invalid", nil)
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "Invalid OOB test ID", result.Error)
	})

	// Clean up
	db.Connection().DB().Delete(&createdTest)
	db.Connection().DB().Delete(&workspace)
}

func TestFindOOBTestsWithInteractions(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Post("/oob-tests", FindOOBTests)

	// Create OOB test without interactions
	testWithoutInteractions := db.OOBTest{
		Code:              "nosql_injection",
		TestName:          "NoSQL Injection Test",
		Target:            "https://example.com/mongo",
		InteractionDomain: "nosql.example.com",
		InteractionFullID: "nosql-test-id",
		Payload:           "{'$ne': null}",
		InsertionPoint:    "json_parameter",
		WorkspaceID:       &workspace.ID,
	}
	createdTestWithoutInteractions, err := db.Connection().CreateOOBTest(testWithoutInteractions)
	assert.Nil(t, err)

	// Create OOB test with interactions
	testWithInteractions := db.OOBTest{
		Code:              "ldap_injection",
		TestName:          "LDAP Injection Test",
		Target:            "https://example.com/ldap",
		InteractionDomain: "ldap.example.com",
		InteractionFullID: "ldap-test-id",
		Payload:           "*)(uid=*))(|(uid=*",
		InsertionPoint:    "form_parameter",
		WorkspaceID:       &workspace.ID,
	}
	createdTestWithInteractions, err := db.Connection().CreateOOBTest(testWithInteractions)
	assert.Nil(t, err)

	// Create interaction for the second test
	interaction := db.OOBInteraction{
		OOBTestID:     &createdTestWithInteractions.ID,
		Protocol:      "DNS",
		FullID:        "ldap-test-id",
		UniqueID:      "unique-ldap-123",
		QType:         "A",
		RawRequest:    "DNS query for ldap.example.com",
		RawResponse:   "DNS response",
		RemoteAddress: "192.168.1.100",
		Timestamp:     time.Now(),
		WorkspaceID:   &workspace.ID,
	}
	createdInteraction, err := db.Connection().CreateInteraction(&interaction)
	assert.Nil(t, err)

	// Test filtering by has_interactions = true
	t.Run("Filter tests with interactions", func(t *testing.T) {
		hasInteractions := true
		filter := db.OOBTestsFilter{
			WorkspaceID:     workspace.ID,
			HasInteractions: &hasInteractions,
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test filtering by has_interactions = false
	t.Run("Filter tests without interactions", func(t *testing.T) {
		hasInteractions := false
		filter := db.OOBTestsFilter{
			WorkspaceID:     workspace.ID,
			HasInteractions: &hasInteractions,
			Pagination: db.Pagination{
				Page:     1,
				PageSize: 10,
			},
		}

		body, _ := json.Marshal(filter)
		req := httptest.NewRequest("POST", "/oob-tests", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, testErr := app.Test(req, 10000)
		assert.Nil(t, testErr)
		if resp == nil {
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Clean up
	db.Connection().DB().Delete(&createdInteraction)
	db.Connection().DB().Delete(&createdTestWithInteractions)
	db.Connection().DB().Delete(&createdTestWithoutInteractions)
	db.Connection().DB().Delete(&workspace)
}
