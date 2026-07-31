package operationlink

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/stretchr/testify/require"
)

func TestAttachOperationJSONMatchesOpenAPIByPathAndMethod(t *testing.T) {
	endpoints := []*db.APIEndpoint{
		{Path: "/pets", Method: "GET"},
		{Path: "/pets", Method: "POST"},
		{Path: "/pets/{id}", Method: "GET"},
	}
	ops := []core.Operation{
		{Path: "/pets", Method: "post", Summary: "create"},
		{Path: "/pets", Method: "GET", Summary: "list"},
	}

	matched := AttachOperationJSON(endpoints, ops, db.APIDefinitionTypeOpenAPI)
	require.Equal(t, 2, matched)

	var listed core.Operation
	require.NoError(t, json.Unmarshal(endpoints[0].OperationJSON, &listed))
	require.Equal(t, "list", listed.Summary)

	var created core.Operation
	require.NoError(t, json.Unmarshal(endpoints[1].OperationJSON, &created))
	require.Equal(t, "create", created.Summary)

	require.Nil(t, endpoints[2].OperationJSON)
}

func TestAttachOperationJSONMatchesGraphQLByNameAndType(t *testing.T) {
	endpoints := []*db.APIEndpoint{
		{OperationID: "pets", OperationType: "query"},
		{OperationID: "pets", OperationType: "mutation"},
	}
	ops := []core.Operation{
		{Name: "pets", OperationID: "pets", GraphQL: &core.GraphQLMetadata{OperationType: "mutation"}, Summary: "m"},
		{Name: "pets", OperationID: "pets", GraphQL: &core.GraphQLMetadata{OperationType: "query"}, Summary: "q"},
	}

	require.Equal(t, 2, AttachOperationJSON(endpoints, ops, db.APIDefinitionTypeGraphQL))

	var q core.Operation
	require.NoError(t, json.Unmarshal(endpoints[0].OperationJSON, &q))
	require.Equal(t, "q", q.Summary)

	var m core.Operation
	require.NoError(t, json.Unmarshal(endpoints[1].OperationJSON, &m))
	require.Equal(t, "m", m.Summary)
}

func TestAttachOperationJSONMatchesSOAPByNameAndAction(t *testing.T) {
	endpoints := []*db.APIEndpoint{
		{OperationID: "GetWeather", SOAPAction: "urn:GetWeather"},
		{OperationID: "GetWeather", SOAPAction: "urn:GetWeatherV2"},
	}
	ops := []core.Operation{
		{Name: "GetWeather", OperationID: "GetWeather", SOAP: &core.SOAPMetadata{SOAPAction: "urn:GetWeatherV2"}, Summary: "v2"},
	}

	require.Equal(t, 1, AttachOperationJSON(endpoints, ops, db.APIDefinitionTypeWSDL))
	require.Nil(t, endpoints[0].OperationJSON)

	var v2 core.Operation
	require.NoError(t, json.Unmarshal(endpoints[1].OperationJSON, &v2))
	require.Equal(t, "v2", v2.Summary)
}

func TestAttachOperationJSONIsIdempotent(t *testing.T) {
	endpoints := []*db.APIEndpoint{{Path: "/pets", Method: "GET"}}
	ops := []core.Operation{{Path: "/pets", Method: "GET", Summary: "list"}}

	require.Equal(t, 1, AttachOperationJSON(endpoints, ops, db.APIDefinitionTypeOpenAPI))
	first := string(endpoints[0].OperationJSON)
	require.Equal(t, 1, AttachOperationJSON(endpoints, ops, db.APIDefinitionTypeOpenAPI))
	require.Equal(t, first, string(endpoints[0].OperationJSON))
}

func TestAttachOperationJSONHandlesEmptyInput(t *testing.T) {
	require.Equal(t, 0, AttachOperationJSON(nil, nil, db.APIDefinitionTypeOpenAPI))
	require.Equal(t, 0, AttachOperationJSON([]*db.APIEndpoint{{Path: "/x", Method: "GET"}}, nil, db.APIDefinitionTypeOpenAPI))
}
