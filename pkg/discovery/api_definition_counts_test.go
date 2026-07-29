package discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedDefinition re-reads the definition from the database. Asserting on the struct
// the persist call returned proves nothing about endpoint_count: the bug these tests
// pin down is precisely a row that disagrees with the struct, and every consumer other
// than the caller itself — the definitions list, the API studio rail, sorting by
// endpoint_count — reads the row.
func storedDefinition(t *testing.T, id uuid.UUID) *db.APIDefinition {
	t.Helper()
	stored, err := db.Connection().GetAPIDefinitionByID(id)
	require.NoError(t, err)
	return stored
}

func storedEndpointCount(t *testing.T, id uuid.UUID) int {
	t.Helper()
	endpoints, err := db.Connection().GetAPIEndpointsByDefinitionID(id)
	require.NoError(t, err)
	return len(endpoints)
}

// assertCountMatchesEndpoints is the invariant the whole feature rests on: whatever a
// definition's endpoint_count says, the api_endpoints table has to agree.
func assertCountMatchesEndpoints(t *testing.T, id uuid.UUID, expected int) {
	t.Helper()
	stored := storedDefinition(t, id)
	assert.Equal(t, expected, storedEndpointCount(t, id), "endpoint rows")
	assert.Equal(t, expected, stored.EndpointCount, "stored endpoint_count")
}

const countsSpec = `{
	"openapi": "3.0.3",
	"info": {"title": "Counted API", "version": "1.0.0"},
	"servers": [{"url": "http://counted.test"}],
	"paths": {
		"/users": {
			"get": {"operationId": "listUsers", "responses": {"200": {"description": "ok"}}},
			"post": {"operationId": "createUser", "responses": {"201": {"description": "ok"}}}
		},
		"/users/{id}": {"get": {"operationId": "getUser", "responses": {"200": {"description": "ok"}}}}
	}
}`

// A spec whose global security requirement forces an extra write of the definition row
// mid-transaction — the write that must not roll the counter back.
const countsSpecWithGlobalSecurity = `{
	"openapi": "3.0.3",
	"info": {"title": "Secured API", "version": "1.0.0"},
	"servers": [{"url": "http://secured.test"}],
	"security": [{"bearerAuth": []}],
	"components": {"securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer"}}},
	"paths": {
		"/a": {"get": {"operationId": "a", "responses": {"200": {"description": "ok"}}}},
		"/b": {"get": {"operationId": "b", "responses": {"200": {"description": "ok"}}}}
	}
}`

const countsIntrospection = `{
	"data": {
		"__schema": {
			"queryType": {"name": "Query"},
			"mutationType": {"name": "Mutation"},
			"subscriptionType": {"name": "Subscription"},
			"types": [
				{
					"kind": "OBJECT", "name": "Query", "fields": [
						{"name": "me", "args": [], "type": {"kind": "OBJECT", "name": "User", "ofType": null}, "isDeprecated": false},
						{"name": "users", "args": [], "type": {"kind": "OBJECT", "name": "User", "ofType": null}, "isDeprecated": false},
						{"name": "search", "args": [], "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "isDeprecated": false}
					]
				},
				{
					"kind": "OBJECT", "name": "Mutation", "fields": [
						{"name": "createUser", "args": [], "type": {"kind": "OBJECT", "name": "User", "ofType": null}, "isDeprecated": false},
						{"name": "deleteUser", "args": [], "type": {"kind": "SCALAR", "name": "Boolean", "ofType": null}, "isDeprecated": false}
					]
				},
				{
					"kind": "OBJECT", "name": "Subscription", "fields": [
						{"name": "userAdded", "args": [], "type": {"kind": "OBJECT", "name": "User", "ofType": null}, "isDeprecated": false}
					]
				},
				{
					"kind": "OBJECT", "name": "User", "fields": [
						{"name": "id", "args": [], "type": {"kind": "SCALAR", "name": "ID", "ofType": null}, "isDeprecated": false}
					]
				}
			]
		}
	}
}`

const countsWSDL = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/"
             xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
             xmlns:tns="urn:counted"
             targetNamespace="urn:counted"
             name="CountedService">
  <message name="PingIn"/>
  <message name="PingOut"/>
  <portType name="CountedPortType">
    <operation name="Ping"><input message="tns:PingIn"/><output message="tns:PingOut"/></operation>
    <operation name="Echo"><input message="tns:PingIn"/><output message="tns:PingOut"/></operation>
  </portType>
  <binding name="CountedBinding" type="tns:CountedPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Ping">
      <soap:operation soapAction="urn:counted/Ping"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
    <operation name="Echo">
      <soap:operation soapAction="urn:counted/Echo"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>
  <service name="CountedService">
    <port name="CountedPort" binding="tns:CountedBinding">
      <soap:address location="http://counted.test/soap"/>
    </port>
  </service>
</definitions>`

func persistForCounts(t *testing.T, workspaceID uint, content string, apiType db.APIDefinitionType, opts APIPersistenceFromContentOptions) *db.APIDefinition {
	t.Helper()

	opts.WorkspaceID = workspaceID
	if opts.SourceURL == "" {
		opts.SourceURL = "http://counted.test/" + uuid.NewString()
	}

	definition, err := PersistAPIDefinitionFromContent([]byte(content), apiType, opts)
	require.NoError(t, err)
	require.NotNil(t, definition)
	return definition
}

func TestPersistedOpenAPIDefinitionStoresItsEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsSpec, db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{})

	assert.Equal(t, 3, definition.EndpointCount)
	assertCountMatchesEndpoints(t, definition.ID, 3)
}

// A global security requirement makes the OpenAPI path save the definition row again
// inside the same transaction. Ordering that write after the counter update would put
// the struct's zero back on the row.
func TestPersistedOpenAPIDefinitionWithGlobalSecurityStoresItsEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsSpecWithGlobalSecurity, db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{})

	assertCountMatchesEndpoints(t, definition.ID, 2)
}

// GraphQL definitions carry their shape in the graphql_* counters, but endpoint_count
// still has to hold the total: it is what the definition list sorts and filters on, and
// what every non-GraphQL-aware consumer reads.
func TestPersistedGraphQLDefinitionStoresItsEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsIntrospection, db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{})

	stored := storedDefinition(t, definition.ID)
	assert.Equal(t, 3, stored.GraphQLQueryCount)
	assert.Equal(t, 2, stored.GraphQLMutationCount)
	assert.Equal(t, 1, stored.GraphQLSubscriptionCount)

	total := stored.GraphQLQueryCount + stored.GraphQLMutationCount + stored.GraphQLSubscriptionCount
	assert.Equal(t, total, stored.EndpointCount, "endpoint_count must total the GraphQL operation counters")
	assertCountMatchesEndpoints(t, definition.ID, 6)
}

func TestPersistedWSDLDefinitionStoresItsEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsWSDL, db.APIDefinitionTypeWSDL, APIPersistenceFromContentOptions{
		SourceURL: "http://counted.test/" + uuid.NewString() + ".wsdl",
	})

	assertCountMatchesEndpoints(t, definition.ID, 2)
}

// The import surfaces (URL import, content import, import-and-scan) all pass a name
// and/or base URL override. Applying those used to re-save the whole definition struct,
// rewriting endpoint_count from memory — which is how definitions ended up in the
// library reporting 0 endpoints while their endpoint rows were sitting right there.
func TestNamedImportKeepsTheStoredEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsSpec, db.APIDefinitionTypeOpenAPI, APIPersistenceFromContentOptions{
		Name:    "Renamed On Import",
		BaseURL: "http://override.test",
	})

	stored := storedDefinition(t, definition.ID)
	assert.Equal(t, "Renamed On Import", stored.Name)
	assert.Equal(t, "http://override.test", stored.BaseURL)
	assertCountMatchesEndpoints(t, definition.ID, 3)
}

func TestNamedGraphQLImportKeepsTheStoredEndpointCount(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsIntrospection, db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		Name: "GraphQL - Testbed",
	})

	stored := storedDefinition(t, definition.ID)
	assert.Equal(t, "GraphQL - Testbed", stored.Name)
	assertCountMatchesEndpoints(t, definition.ID, 6)
}

// An override must touch only the columns it names.
func TestNamedImportDoesNotDisturbTheProtocolCounters(t *testing.T) {
	workspace := setupTestWorkspace(t)

	definition := persistForCounts(t, workspace.ID, countsIntrospection, db.APIDefinitionTypeGraphQL, APIPersistenceFromContentOptions{
		Name:    "Renamed GraphQL",
		BaseURL: "http://override.test",
	})

	stored := storedDefinition(t, definition.ID)
	assert.Equal(t, 3, stored.GraphQLQueryCount)
	assert.Equal(t, 2, stored.GraphQLMutationCount)
	assert.Equal(t, 1, stored.GraphQLSubscriptionCount)
	assert.Equal(t, 4, stored.GraphQLTypeCount)
}
