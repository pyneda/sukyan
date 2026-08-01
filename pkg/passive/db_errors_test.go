package passive

import (
	"testing"
)

func TestSearchDatabaseErrors(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantDatabaseName string
		wantMatched      string
	}{
		{"MySQL", "You have an error in your SQL syntax; MySQL server version", "MySQL", "SQL syntax; MySQL"},
		{"PostgreSQL", "PostgreSQL ERROR: column does not exist", "PostgreSQL", "PostgreSQL ERROR"},
		{"MS SQL Server", "Driver for SQL Server failed to connect", "Microsoft SQL Server", "Driver for SQL Server"},
		{"MS Access", "Microsoft Access Driver error", "Microsoft Access", "Microsoft Access Driver"},
		{"Oracle", "error received ORA-00090 when querying the database", "Oracle", "ORA-00090"},
		{"IBM DB2", "CLI Driver for DB2 SQL error", "IBM DB2", "CLI Driver for DB2"},
		{"SQLite", "[SQLITE_ERROR] SQL error", "SQLite", "[SQLITE_ERROR]"},
		// Raw engine error text emitted by Go SQLite drivers (modernc.org/sqlite,
		// mattn/go-sqlite3) that the framework-wrapper patterns above do not catch.
		{"SQLite logic error", `{"error":"SQL logic error: unrecognized token: \"'\" (1)","found":false}`, "SQLite", "SQL logic error"},
		{"SQLite near syntax", `SQL logic error: near "x": syntax error`, "SQLite", "SQL logic error"},
		{"SQLite near syntax no prefix", `near "x": syntax error`, "SQLite", `near "x": syntax error`},
		{"SQLite unrecognized token", `unrecognized token: "@"`, "SQLite", "unrecognized token:"},
		{"SQLite no such column", "no such column: password", "SQLite", "no such column:"},
		{"SQLite no such table", "no such table: agents", "SQLite", "no such table:"},
		{"Sybase", "Sybase message: Server is not responding", "Sybase", "Sybase message"},
		// Native Postgres driver output. Every wrapper pattern above expects a
		// vendor prefix these drivers never emit, so the raw server message is
		// the only thing on the wire.
		{"node-postgres unterminated string", `{"errors":[{"message":"unterminated quoted string at or near \"' or pg_sleep(7)--\""}]}`, "PostgreSQL", "unterminated quoted string at or near"},
		{"pgx syntax error", `ERROR: syntax error at or near "'" (SQLSTATE 42601)`, "PostgreSQL", `syntax error at or near "`},
		{"lib/pq syntax error", `pq: syntax error at or near "SELECT"`, "PostgreSQL", `syntax error at or near "`},
		{"psycopg module", "psycopg2.errors.UndefinedColumn: column \"x\" does not exist", "PostgreSQL", `column "x" does not exist`},
		{"asyncpg module", "asyncpg.exceptions.PostgresSyntaxError: bad", "PostgreSQL", "asyncpg.exceptions."},
		{"invalid input syntax", `invalid input syntax for type integer: "abc"`, "PostgreSQL", "invalid input syntax for type "},
		{"operator does not exist", "operator does not exist: text = integer", "PostgreSQL", "operator does not exist: "},
		{"relation does not exist", `relation "users" does not exist`, "PostgreSQL", `relation "users" does not exist`},
		{"syntax error at end of input", "syntax error at end of input", "PostgreSQL", "syntax error at end of input"},
		{"Non-matching", "This is a non-matching error message", "", ""},
		// The Postgres phrases carry their own punctuation so ordinary prose and
		// unrelated validation copy must not false-fire.
		{"Prose syntax error", "There is a syntax error in your search query", "", ""},
		{"Prose column missing", "That column does not exist in this view", "", ""},
		{"Prose invalid input", "invalid input for the type of card selected", "", ""},
		// The generic SQLite phrases are anchored to the trailing colon SQLite
		// always emits, so bare English prose in a non-SQLite response must NOT
		// false-fire (regression guard for the FIX1 precision fix).
		{"Auth token error (no colon)", `{"error":"unrecognized token"}`, "", ""},
		{"Prose no such table", "Sorry, there is no such table available tonight", "", ""},
		{"Prose no such column in UI", "The report has no such column configured", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SearchDatabaseErrors(tt.input)
			if got == nil {
				if tt.wantDatabaseName != "" {
					t.Errorf("Expected database name %s, but got nil", tt.wantDatabaseName)
				}
				return
			}
			if got.DatabaseName != tt.wantDatabaseName {
				t.Errorf("Expected database name %s, but got %s", tt.wantDatabaseName, got.DatabaseName)
			}
			if got.MatchStr != tt.wantMatched {
				t.Errorf("Expected matched string %s, but got %s", tt.wantMatched, got.MatchStr)
			}
		})
	}
}
