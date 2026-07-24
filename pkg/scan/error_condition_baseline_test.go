package scan

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/passive"
)

func historyWithBody(body string) *db.History {
	return &db.History{
		RawResponse: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n" + body),
	}
}

func TestBaselineContainsDatabaseError(t *testing.T) {
	sqliteErr := &passive.DatabaseErrorMatch{DatabaseName: "SQLite", MatchStr: "sqlite3.OperationalError"}

	tests := []struct {
		name     string
		original *db.History
		match    *passive.DatabaseErrorMatch
		want     bool
	}{
		{
			name:     "same error already in baseline is suppressed",
			original: historyWithBody("sqlite3.OperationalError: near \"x\": syntax error"),
			match:    sqliteErr,
			want:     true,
		},
		{
			name:     "same DBMS family in baseline is suppressed",
			original: historyWithBody("near \"FROM\": syntax error\nsqlite3.OperationalError"),
			match:    sqliteErr,
			want:     true,
		},
		{
			name:     "clean baseline keeps the finding",
			original: historyWithBody("<html><body>Welcome</body></html>"),
			match:    sqliteErr,
			want:     false,
		},
		{
			name:     "nil baseline must not suppress",
			original: nil,
			match:    sqliteErr,
			want:     false,
		},
		{
			name:     "empty body must not suppress",
			original: historyWithBody(""),
			match:    sqliteErr,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baselineContainsDatabaseError(tt.original, tt.match); got != tt.want {
				t.Errorf("baselineContainsDatabaseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssueCodeForDatabaseFamily(t *testing.T) {
	sqlite := &passive.DatabaseErrorMatch{DatabaseName: "SQLite", MatchStr: "sqlite3.OperationalError"}
	mongo := &passive.DatabaseErrorMatch{DatabaseName: "MongoDB", MatchStr: "MongoError"}

	tests := []struct {
		name      string
		declared  db.IssueCode
		override  db.IssueCode
		match     *passive.DatabaseErrorMatch
		want      db.IssueCode
		corrected bool
	}{
		{
			// The observed graphql-api bug: 98 nosql_injection findings whose
			// only evidence was a SQLite error.
			name:      "nosql claim backed by SQL error becomes sql_injection",
			declared:  db.NosqlInjectionCode,
			match:     sqlite,
			want:      db.SqlInjectionCode,
			corrected: true,
		},
		{
			name:      "sql claim backed by NoSQL error becomes nosql_injection",
			declared:  db.SqlInjectionCode,
			match:     mongo,
			want:      db.NosqlInjectionCode,
			corrected: true,
		},
		{
			name:     "nosql claim backed by NoSQL error is left alone",
			declared: db.NosqlInjectionCode,
			match:    mongo,
		},
		{
			name:     "sql claim backed by SQL error is left alone",
			declared: db.SqlInjectionCode,
			match:    sqlite,
		},
		{
			name:     "unrelated issue codes are never rewritten",
			declared: db.XssReflectedCode,
			match:    sqlite,
		},
		{
			name:      "an existing override is corrected too",
			declared:  db.XssReflectedCode,
			override:  db.NosqlInjectionCode,
			match:     sqlite,
			want:      db.SqlInjectionCode,
			corrected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := issueCodeForDatabaseFamily(tt.declared, tt.override, tt.match)
			if ok != tt.corrected {
				t.Fatalf("corrected = %v, want %v", ok, tt.corrected)
			}
			if ok && got != tt.want {
				t.Errorf("code = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaselineContainsXPathError(t *testing.T) {
	const match = "XPathException"

	if !baselineContainsXPathError(historyWithBody("javax.xml.xpath.XPathException: bad expr"), match) {
		t.Error("expected suppression when the baseline already contains the XPath error")
	}
	if baselineContainsXPathError(historyWithBody("<html>ok</html>"), match) {
		t.Error("expected no suppression for a clean baseline")
	}
	if baselineContainsXPathError(nil, match) {
		t.Error("nil baseline must not suppress a finding")
	}
}
