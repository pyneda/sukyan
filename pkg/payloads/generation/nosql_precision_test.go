package generation

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

// A NoSQL-injection template must never substantiate a finding from a
// class-agnostic signal. A bare status-code condition (status_code set, no
// contains) matches any payload that makes the backend return that status - a
// SQL syntax error, a null-byte rejection, or a framework validation crash all
// yield a 500 - so it mislabels SQL/file/validation sinks as nosql_injection.
// Every nosql detection method must carry a NoSQL-specific signal: a contains
// fragment, a database_error check (family-corrected to the matched engine at
// report time), or a time-based probe.
func TestNosqlTemplatesRequireNosqlSpecificSignal(t *testing.T) {
	generators, err := LoadLocalGenerators()
	require.NoError(t, err)

	checked := 0
	for _, g := range generators {
		if g.IssueCode != string(db.NosqlInjectionCode) {
			continue
		}
		checked++
		for _, dm := range g.DetectionMethods {
			rc := dm.ResponseCondition
			if rc == nil {
				continue
			}
			if rc.StatusCode != 0 && rc.Contains == "" {
				t.Errorf("nosql template %q has a bare status-code detection method (status_code=%d, no contains): a status change is not NoSQL-specific and false-fires on SQL/validation 500s", g.ID, rc.StatusCode)
			}
		}
	}
	require.NotZero(t, checked, "expected at least one nosql_injection template to be present")
}
