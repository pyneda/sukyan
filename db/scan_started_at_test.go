package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// newSQLCapturingConnection builds a DatabaseConnection that never talks to a
// server: the pgx pool is opened lazily and every statement is compiled but not
// executed, so the generated SQL can be asserted on without a live database.
func newSQLCapturingConnection(t *testing.T) (*DatabaseConnection, *string) {
	t.Helper()

	gormDB, err := gorm.Open(
		postgres.New(postgres.Config{DSN: "postgres://sukyan:sukyan@127.0.0.1:1/sukyan?sslmode=disable"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true},
	)
	require.NoError(t, err)

	var captured string
	require.NoError(t, gormDB.Callback().Update().After("gorm:update").Register("test:capture_sql", func(tx *gorm.DB) {
		captured = tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...)
	}))

	return &DatabaseConnection{db: gormDB}, &captured
}

// started_at must be stamped by the first transition into a running status only.
// The orchestrator calls SetScanStatus(ScanStatusScanning) when the active scan
// phase begins, long after the scan started; overwriting started_at there made
// every reported scan duration cover just the active scan phase.
func TestSetScanStatusStartedAtWrites(t *testing.T) {
	tests := []struct {
		name              string
		status            ScanStatus
		wantStartedAt     bool
		wantCompletedAt   bool
		wantStartedAtOnly bool
	}{
		{name: "crawling preserves an existing start time", status: ScanStatusCrawling, wantStartedAt: true},
		{name: "scanning preserves an existing start time", status: ScanStatusScanning, wantStartedAt: true},
		{name: "completed stamps completion only", status: ScanStatusCompleted, wantCompletedAt: true},
		{name: "failed stamps completion only", status: ScanStatusFailed, wantCompletedAt: true},
		{name: "cancelled stamps completion only", status: ScanStatusCancelled, wantCompletedAt: true},
		{name: "paused touches neither timestamp", status: ScanStatusPaused},
		{name: "pending touches neither timestamp", status: ScanStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, captured := newSQLCapturingConnection(t)

			require.NoError(t, conn.SetScanStatus(42, tt.status))

			sql := *captured
			require.NotEmpty(t, sql)
			assert.Contains(t, sql, string(tt.status))

			if tt.wantStartedAt {
				assert.Contains(t, sql, `"started_at"=COALESCE(started_at,`,
					"started_at must only be written when it is still NULL")
			} else {
				assert.NotContains(t, sql, "started_at")
			}

			if tt.wantCompletedAt {
				assert.Contains(t, sql, `"completed_at"=`)
				assert.NotContains(t, sql, "COALESCE(completed_at,")
			} else {
				assert.NotContains(t, sql, "completed_at")
			}
		})
	}
}

func TestSetScanStatusUpdatesTargetedScanOnly(t *testing.T) {
	conn, captured := newSQLCapturingConnection(t)

	require.NoError(t, conn.SetScanStatus(7, ScanStatusScanning))

	sql := *captured
	assert.True(t, strings.HasPrefix(sql, `UPDATE "scans" SET `), "unexpected statement: %s", sql)
	assert.Contains(t, sql, "WHERE id = 7")
}
