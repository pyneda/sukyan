package workspace

import (
	"fmt"
	"sort"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type foreignKey struct {
	Child  string
	Parent string
	Column string
}

func liveForeignKeys(t *testing.T) []foreignKey {
	t.Helper()
	var keys []foreignKey
	require.NoError(t, db.Connection().DB().Raw(`
		SELECT src.relname AS child, tgt.relname AS parent,
		       (SELECT string_agg(att.attname, ',' ORDER BY att.attnum)
		          FROM unnest(con.conkey) k
		          JOIN pg_attribute att ON att.attrelid = src.oid AND att.attnum = k) AS column
		FROM pg_constraint con
		JOIN pg_class src ON src.oid = con.conrelid
		JOIN pg_class tgt ON tgt.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = src.relnamespace AND n.nspname = 'public'
		WHERE con.contype = 'f'`).Scan(&keys).Error)
	require.NotEmpty(t, keys, "no foreign keys found; is the test database migrated?")
	return keys
}

func specNames() map[string]tableSpec {
	byName := make(map[string]tableSpec, len(orderedTables))
	for _, spec := range orderedTables {
		byName[spec.Name] = spec
	}
	return byName
}

// The insert order is only valid if every foreign key a table carries -- other
// than the ones it explicitly defers -- points at a table already inserted.
func TestTableOrderSatisfiesLiveForeignKeyGraph(t *testing.T) {
	specs := specNames()
	deferredBy := make(map[string]map[string]bool, len(orderedTables))
	for _, spec := range orderedTables {
		deferredBy[spec.Name] = make(map[string]bool, len(spec.DeferredColumns))
		for _, column := range spec.DeferredColumns {
			deferredBy[spec.Name][column] = true
		}
	}

	inserted := make(map[string]bool, len(orderedTables))
	for _, spec := range orderedTables {
		for _, fk := range liveForeignKeys(t) {
			if fk.Child != spec.Name || fk.Parent == spec.Name {
				continue
			}
			if _, tracked := specs[fk.Parent]; !tracked {
				continue
			}
			if deferredBy[spec.Name][fk.Column] {
				continue
			}
			assert.True(t, inserted[fk.Parent],
				"%s is inserted before %s, but %s.%s references it and is not deferred",
				spec.Name, fk.Parent, spec.Name, fk.Column)
		}
		inserted[spec.Name] = true
	}
}

// Every column deferred must actually be nullable, otherwise withholding it on
// the first insert would violate a NOT NULL constraint.
func TestDeferredColumnsAreNullable(t *testing.T) {
	for _, spec := range orderedTables {
		for _, column := range spec.DeferredColumns {
			var notNull bool
			require.NoError(t, db.Connection().DB().Raw(`
				SELECT a.attnotnull FROM pg_attribute a
				JOIN pg_class c ON c.oid = a.attrelid
				JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
				WHERE c.relname = ? AND a.attname = ? AND NOT a.attisdropped`,
				spec.Name, column).Scan(&notNull).Error)
			assert.False(t, notNull, "%s.%s is deferred but declared NOT NULL", spec.Name, column)
		}
	}
}

// A bigint identifier that the registry does not list would survive import
// unshifted and point at an unrelated row, so the registry must match the
// schema exactly.
func TestIDColumnsMatchLiveSchema(t *testing.T) {
	for _, spec := range orderedTables {
		var columns []struct {
			Column string
			Type   string
		}
		require.NoError(t, db.Connection().DB().Raw(`
			SELECT a.attname AS column, format_type(a.atttypid, -1) AS type
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
			JOIN pg_constraint con ON con.conrelid = c.oid AND con.contype IN ('p','f')
			JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
			WHERE c.relname = ?
			GROUP BY 1, 2
			UNION
			SELECT a.attname, format_type(a.atttypid, -1)
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
			JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'workspace_id' AND NOT a.attisdropped
			WHERE c.relname = ?`, spec.Name, spec.Name).Scan(&columns).Error)

		var bigints, uuids []string
		for _, column := range columns {
			switch column.Type {
			case "bigint":
				bigints = append(bigints, column.Column)
			case "uuid":
				uuids = append(uuids, column.Column)
			}
		}

		assert.ElementsMatch(t, bigints, spec.IDColumns,
			"%s: IDColumns does not match the bigint key columns in the database", spec.Name)

		declaredUUIDs := append([]string{}, spec.UUIDReferences...)
		if spec.UUIDPrimaryKey != "" {
			declaredUUIDs = append(declaredUUIDs, spec.UUIDPrimaryKey)
		}
		assert.ElementsMatch(t, uuids, declaredUUIDs,
			"%s: uuid columns do not match the uuid key columns in the database", spec.Name)
	}
}

// If a new workspace-scoped table appears and nobody adds it here, export would
// silently drop it and delete would silently orphan it.
func TestEveryWorkspaceReachableTableIsRegistered(t *testing.T) {
	children := make(map[string][]string)
	for _, fk := range liveForeignKeys(t) {
		if fk.Child != fk.Parent {
			children[fk.Parent] = append(children[fk.Parent], fk.Child)
		}
	}

	reachable := map[string]bool{"workspaces": true}
	queue := []string{"workspaces"}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if !reachable[child] {
				reachable[child] = true
				queue = append(queue, child)
			}
		}
	}

	// Carried by workspace_id without a declared constraint.
	reachable["scan_jobs"] = true
	reachable["playground_fuzz_runs"] = true

	var missing []string
	specs := specNames()
	for table := range reachable {
		if _, ok := specs[table]; !ok {
			missing = append(missing, table)
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing, "these workspace-reachable tables are not registered in orderedTables")

	var unknown []string
	for _, spec := range orderedTables {
		if !reachable[spec.Name] {
			unknown = append(unknown, spec.Name)
		}
	}
	assert.Empty(t, unknown, "these registered tables are not reachable from a workspace")
}

// The scoping queries must be valid SQL against the real schema and must return
// whole rows of their own table.
func TestEveryScopeQueryRunsAndReturnsOwnColumns(t *testing.T) {
	for _, spec := range orderedTables {
		query := fmt.Sprintf("SELECT row_to_json(sukyan_exported_row) FROM (%s) AS sukyan_exported_row LIMIT 1", spec.Query)
		rows, err := db.Connection().DB().Raw(query, 0).Rows()
		require.NoError(t, err, "scope query for %s failed", spec.Name)
		rows.Close()
	}
}
