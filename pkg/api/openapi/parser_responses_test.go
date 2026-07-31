package openapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const responsesSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "t", "version": "1"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "responses": {
          "200": {
            "description": "A list of pets",
            "content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}
          },
          "404": {"description": "Not found"}
        }
      }
    }
  }
}`

func TestParseFromRawDefinitionPopulatesResponses(t *testing.T) {
	ops, err := ParseFromRawDefinition([]byte(responsesSpec))
	require.NoError(t, err)
	require.Len(t, ops, 1)

	responses := ops[0].Responses
	require.Len(t, responses, 2)

	// Sorted by status code so the order is stable for the UI.
	require.Equal(t, "200", responses[0].StatusCode)
	require.Equal(t, "A list of pets", responses[0].Description)
	require.Equal(t, "application/json", responses[0].ContentType)
	require.NotNil(t, responses[0].Schema)

	require.Equal(t, "404", responses[1].StatusCode)
	require.Equal(t, "Not found", responses[1].Description)
	require.Empty(t, responses[1].ContentType)
}

const requestBodySpec = `{
  "openapi": "3.0.0",
  "info": {"title": "t", "version": "1"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/pets": {
      "post": {
        "operationId": "addPet",
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}
        },
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

func TestParseFromRawDefinitionPopulatesRequestBodySchema(t *testing.T) {
	ops, err := ParseFromRawDefinition([]byte(requestBodySpec))
	require.NoError(t, err)
	require.Len(t, ops, 1)

	body := ops[0].OpenAPI.RequestBody
	require.NotNil(t, body)
	require.True(t, body.Required)
	require.Equal(t, "application/json", body.ContentType)
	require.NotNil(t, body.Schema, "the body's own schema must survive, not just its flattened parameters")
}
