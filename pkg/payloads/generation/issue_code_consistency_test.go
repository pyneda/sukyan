package generation

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

// Every embedded template's issue_code must resolve to a KB entry. A mismatch
// (a hyphen/underscore typo, a renamed code) makes GetIssueTemplateByCode return
// nil at report time — and for OOB-only templates that nil is dereferenced
// inside the poller transaction, panicking on the callback. This guards the
// whole template tree against that class of typo.
func TestEmbeddedTemplateIssueCodesExistInKB(t *testing.T) {
	generators, err := LoadLocalGenerators()
	require.NoError(t, err)
	require.NotEmpty(t, generators)

	for _, g := range generators {
		if g.IssueCode == "" {
			continue
		}
		if db.GetIssueTemplateByCode(db.IssueCode(g.IssueCode)) == nil {
			t.Errorf("template %q declares issue_code %q which has no KB entry (db/kb/*.yaml)", g.ID, g.IssueCode)
		}

		for _, dm := range g.DetectionMethods {
			var override db.IssueCode
			switch {
			case dm.ResponseCondition != nil:
				override = dm.ResponseCondition.IssueOverride
			case dm.ResponseCheck != nil:
				override = dm.ResponseCheck.IssueOverride
			}
			if override != "" && db.GetIssueTemplateByCode(override) == nil {
				t.Errorf("template %q has a detection-method issue_override %q with no KB entry", g.ID, override)
			}
		}
	}
}
