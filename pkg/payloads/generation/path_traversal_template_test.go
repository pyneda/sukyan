package generation

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

// The path_traversal builder in pkg/payloads was never wired to any scanner and
// there was no KB entry, so a live arbitrary-file-read sink went undetected (or
// was mislabeled by unrelated detectors). This template must exist, resolve to
// the path_traversal KB entry, detect only on known-file signatures (never a
// bare status code, which is the FP-prone pattern), and target file-path-shaped
// parameters so it actually reaches sinks like ?path= without a full fuzz run.
func TestPathTraversalTemplateIsSignatureBased(t *testing.T) {
	generators, err := LoadLocalGenerators()
	require.NoError(t, err)

	var gen *PayloadGenerator
	for _, g := range generators {
		if g.ID == "path-traversal" {
			gen = g
			break
		}
	}
	require.NotNil(t, gen, "path-traversal template must exist")
	require.Equal(t, string(db.PathTraversalCode), gen.IssueCode)
	require.NotNil(t, db.GetIssueTemplateByCode(db.PathTraversalCode), "path_traversal KB entry must exist")

	require.NotEmpty(t, gen.DetectionMethods)
	for _, dm := range gen.DetectionMethods {
		require.Nil(t, dm.OOBInteraction)
		require.Nil(t, dm.TimeBased)
		rc := dm.ResponseCondition
		require.NotNil(t, rc, "every path-traversal detection method must be a signature response_condition")
		require.NotEmpty(t, rc.Contains, "detection must match a known-file signature, not a bare status")
	}

	// Must reach file-path-shaped params (e.g. ?path=) in smart mode, not only fuzz.
	var names []string
	for _, c := range gen.Launch.Conditions {
		if c.Type == ParameterName {
			names = append(names, c.ParameterNames...)
		}
	}
	require.Contains(t, names, "path", "template must launch on the common file-path parameter name")

	// Payloads must actually attempt traversal to a known file.
	foundLadder := false
	for _, tmpl := range gen.Templates {
		if tmpl == "../../../../../../../../../../etc/passwd" {
			foundLadder = true
		}
	}
	require.True(t, foundLadder, "template must include a plain ../ ladder to /etc/passwd")
}
