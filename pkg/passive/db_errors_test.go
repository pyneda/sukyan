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
		{"Sybase", "Sybase message: Server is not responding", "Sybase", "Sybase message"},
		{"Non-matching", "This is a non-matching error message", "", ""},
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
