package graphql

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

// A GraphQL base history whose body carries a query + variables must yield the
// resolver-argument insertion points regardless of scan mode, so per-operation
// injection actually reaches the resolver args (Break C).
func TestGraphQLResolverInsertionPointsFromHistory(t *testing.T) {
	body := `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1"},"operationName":"GetUser"}`
	history := &db.History{
		Method:             "POST",
		URL:                "http://example.com/graphql",
		RequestContentType: "application/json",
		RawRequest:         []byte("POST /graphql HTTP/1.1\r\nContent-Type: application/json\r\n\r\n" + body),
	}

	points, err := graphqlResolverInsertionPoints(history)
	if err != nil {
		t.Fatalf("graphqlResolverInsertionPoints returned error: %v", err)
	}

	var foundIDVariable bool
	for _, p := range points {
		if p.Type != scan.InsertionPointTypeGraphQLVariable && p.Type != scan.InsertionPointTypeGraphQLInlineArg {
			t.Errorf("expected only graphql insertion points, got type %q for %q", p.Type, p.Name)
		}
		if p.Type == scan.InsertionPointTypeGraphQLVariable && p.Name == "id" {
			foundIDVariable = true
		}
	}

	if !foundIDVariable {
		t.Fatalf("expected a graphql_variable insertion point for the resolver arg 'id', got %d points: %+v", len(points), points)
	}
}

// In Smart/Fast mode the standard base-history scan drops the GraphQL insertion
// points (they are not dynamic/reflected/body/parameter), so per-operation injection
// must run to reach the resolver args. In Fuzz mode the standard scan already keeps
// and injects those points, so re-running here would only duplicate work.
func TestShouldRunResolverArgInjectionByMode(t *testing.T) {
	cases := []struct {
		mode scan_options.ScanMode
		want bool
	}{
		{scan_options.ScanModeSmart, true},
		{scan_options.ScanModeFast, true},
		{scan_options.ScanModeFuzz, false},
	}

	for _, tc := range cases {
		if got := shouldRunResolverArgInjection(tc.mode); got != tc.want {
			t.Errorf("shouldRunResolverArgInjection(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}
