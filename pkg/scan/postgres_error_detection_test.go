package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
)

// The response graphql-apollo (Node + node-postgres) returns for a broken quote.
// Before the native-driver patterns were added, no DBMS_ERRORS entry matched it,
// so seven string-concatenated SQLi sinks produced zero findings.
const apolloPostgresError = `{"errors":[{"message":"unterminated quoted string at or near \"' or pg_sleep(7)-- ORDER BY id\"",` +
	`"path":["issuesByQuery"],"extensions":{"code":"INTERNAL_SERVER_ERROR","stacktrace":[` +
	`"error: unterminated quoted string at or near \"' or pg_sleep(7)-- ORDER BY id\"",` +
	`"    at /app/node_modules/pg-pool/index.js:45:11"]}}],"data":null}`

func resultWithBodies(baseline, payloadResponse string) TemplateScannerResult {
	raw := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n" + payloadResponse
	return TemplateScannerResult{
		Original: historyWithBody(baseline),
		Result:   &db.History{RawResponse: []byte(raw)},
		ResponseData: http_utils.FullResponseData{
			Body:      []byte(payloadResponse),
			RawString: raw,
		},
	}
}

func TestDatabaseErrorConditionDetectsNativePostgresDriver(t *testing.T) {
	scanner := &TemplateScanner{}
	method := generation.DetectionMethod{
		ResponseCheck: &generation.ResponseCheckDetectionMethod{
			Check:      generation.DatabaseErrorCondition,
			Confidence: 80,
		},
	}

	t.Run("fires on the driver's bare message", func(t *testing.T) {
		res := resultWithBodies(`{"data":{"issuesByQuery":[]}}`, apolloPostgresError)
		matched, desc, confidence, _, err := scanner.EvaluateDetectionMethod(context.Background(), res, method)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !matched {
			t.Fatal("expected a match on the node-postgres error message")
		}
		if confidence != 80 {
			t.Errorf("confidence = %d, want 80", confidence)
		}
		if want := "PostgreSQL"; !strings.Contains(desc, want) {
			t.Errorf("description %q should name %s", desc, want)
		}
	})

	t.Run("still suppressed when the baseline already errors", func(t *testing.T) {
		res := resultWithBodies(apolloPostgresError, apolloPostgresError)
		matched, _, _, _, err := scanner.EvaluateDetectionMethod(context.Background(), res, method)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matched {
			t.Error("an error present in the baseline is an endpoint property, not evidence")
		}
	})
}

