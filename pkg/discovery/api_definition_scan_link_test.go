package discovery

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScan(t *testing.T, workspaceID uint, title string) *db.Scan {
	t.Helper()

	scan, err := db.Connection().CreateScan(&db.Scan{WorkspaceID: workspaceID, Title: title})
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Connection().DB().Where("scan_id = ?", scan.ID).Delete(&db.ScanAPIDefinition{})
	})

	return scan
}

// The validation gate is not what these tests exercise, and the fixtures are SDL
// rather than introspection responses, so it always passes here. Its real
// behaviour is covered by IsGraphQLValidationFunc's own tests.
func acceptEveryResponse(*db.History, *ValidationContext) (bool, string, int) {
	return true, "valid", 100
}

func runDiscoveryPersist(t *testing.T, workspaceID, scanID uint, histories ...*db.History) {
	t.Helper()

	persistDiscoveredAPIDefinitions(
		DiscoverAndCreateIssueResults{
			DiscoverResults: DiscoverResults{Responses: histories},
			Issues:          []db.Issue{{}},
		},
		DiscoveryOptions{
			HistoryCreationOptions: http_utils.HistoryCreationOptions{
				WorkspaceID: workspaceID,
				ScanID:      scanID,
			},
		},
		acceptEveryResponse,
		PersistGraphQLDefinition,
		"GraphQL",
	)
}

func linkCount(t *testing.T, scanID uint, definitionID uuid.UUID) int {
	t.Helper()

	linked, err := db.Connection().GetLinkedAPIDefinitionIDs(scanID)
	require.NoError(t, err)

	count := 0
	for _, id := range linked {
		if id == definitionID {
			count++
		}
	}
	return count
}

func definitionIDsForScan(t *testing.T, scanID uint) []uuid.UUID {
	t.Helper()

	definitions, err := db.Connection().GetAllAPIDefinitionsForScan(scanID)
	require.NoError(t, err)

	ids := make([]uuid.UUID, 0, len(definitions))
	for _, definition := range definitions {
		ids = append(ids, definition.ID)
	}
	return ids
}

// A scan reaches a definition through api_definitions.scan_id or through the join
// table, and rediscovery used to write neither. Every repeat scan of a target
// therefore found no definitions, its API phase was gated off, and a GraphQL
// target got no API scan at all after the very first run.
func TestRediscoveredDefinitionIsClaimedByTheNewScan(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	first := testScan(t, workspace.ID, "rediscovery-scan-one")
	second := testScan(t, workspace.ID, "rediscovery-scan-two")

	runDiscoveryPersist(t, workspace.ID, first.ID, historyForSpec(t, workspace.ID, origin+"/graphql", dedupSchemaSDL))

	discovered := graphQLDefinitionsForOrigin(t, workspace.ID, origin)
	require.Len(t, discovered, 1)
	definition := discovered[0]
	require.NotNil(t, definition.ScanID)
	require.Equal(t, first.ID, *definition.ScanID)

	hasDefinitions, err := db.Connection().HasLinkedAPIDefinitions(second.ID)
	require.NoError(t, err)
	require.False(t, hasDefinitions, "the second scan starts with no claim on the definition")

	runDiscoveryPersist(t, workspace.ID, second.ID,
		historyForSpec(t, workspace.ID, origin+"/graphql", dedupSchemaSDL),
		historyForSpec(t, workspace.ID, origin+"/graphql/playground", dedupSchemaSDL),
	)

	hasDefinitions, err = db.Connection().HasLinkedAPIDefinitions(second.ID)
	require.NoError(t, err)
	assert.True(t, hasDefinitions, "the API scan phase is gated on this predicate")

	assert.Equal(t, []uuid.UUID{definition.ID}, definitionIDsForScan(t, second.ID))
	assert.Equal(t, 1, linkCount(t, second.ID, definition.ID), "one link row per scan, whatever the alias count")

	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, origin), 1, "rediscovery must not store a second definition")
	assertCountMatchesEndpoints(t, definition.ID, 3)

	endpoints, err := db.Connection().GetEnabledAPIEndpointsByDefinitionID(definition.ID)
	require.NoError(t, err)
	assert.Len(t, endpoints, 3, "ScheduleAPIScan enumerates these for the new scan")

	// The discovering scan keeps its own view: scan_id is left where it was.
	assert.Equal(t, []uuid.UUID{definition.ID}, definitionIDsForScan(t, first.ID))
	assert.Equal(t, first.ID, *storedDefinition(t, definition.ID).ScanID)
}

// The scan that discovered the definition owns it through scan_id and needs no
// join row.
func TestDiscoveringScanIsNotAlsoLinked(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	scan := testScan(t, workspace.ID, "discovering-scan")

	runDiscoveryPersist(t, workspace.ID, scan.ID,
		historyForSpec(t, workspace.ID, origin+"/graphql", dedupSchemaSDL),
		historyForSpec(t, workspace.ID, origin+"/graphql/v2", dedupSchemaSDL),
	)

	definitions := graphQLDefinitionsForOrigin(t, workspace.ID, origin)
	require.Len(t, definitions, 1)

	assert.Equal(t, 0, linkCount(t, scan.ID, definitions[0].ID))
	assert.Equal(t, []uuid.UUID{definitions[0].ID}, definitionIDsForScan(t, scan.ID))
}

// Workers rediscover the aliases of one endpoint concurrently, so the link write
// has to be idempotent rather than a check-then-act on a composite primary key.
func TestConcurrentRediscoveryWritesOneLink(t *testing.T) {
	workspace := setupTestWorkspace(t)
	origin := graphQLTestOrigin()

	first := testScan(t, workspace.ID, "concurrent-rediscovery-one")
	second := testScan(t, workspace.ID, "concurrent-rediscovery-two")

	runDiscoveryPersist(t, workspace.ID, first.ID, historyForSpec(t, workspace.ID, origin+"/graphql", dedupSchemaSDL))

	definitions := graphQLDefinitionsForOrigin(t, workspace.ID, origin)
	require.Len(t, definitions, 1)
	definition := definitions[0]

	aliases := []string{"/graphql", "/graphql/api", "/graphql/v1", "/graphql/v2", "/graphql/console", "/graphql/playground"}
	histories := make([]*db.History, 0, len(aliases))
	for _, alias := range aliases {
		histories = append(histories, historyForSpec(t, workspace.ID, origin+alias, dedupSchemaSDL))
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, history := range histories {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			runDiscoveryPersist(t, workspace.ID, second.ID, history)
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, linkCount(t, second.ID, definition.ID))
	assert.Len(t, graphQLDefinitionsForOrigin(t, workspace.ID, origin), 1)
	assert.Equal(t, []uuid.UUID{definition.ID}, definitionIDsForScan(t, second.ID))
}

func TestLinkAPIDefinitionToScanIsIdempotent(t *testing.T) {
	workspace := setupTestWorkspace(t)
	scan := testScan(t, workspace.ID, "idempotent-link")

	definition, err := db.Connection().CreateAPIDefinition(&db.APIDefinition{
		WorkspaceID: workspace.ID,
		Name:        "Idempotent Link",
		Type:        db.APIDefinitionTypeGraphQL,
		Status:      db.APIDefinitionStatusParsed,
		SourceURL:   graphQLTestOrigin() + "/graphql",
	})
	require.NoError(t, err)

	require.NoError(t, db.Connection().LinkAPIDefinitionToScan(scan.ID, definition.ID))
	require.NoError(t, db.Connection().LinkAPIDefinitionToScan(scan.ID, definition.ID),
		"re-linking is the desired state, not a duplicate-key failure")

	assert.Equal(t, 1, linkCount(t, scan.ID, definition.ID))
}
