package payloads

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/scan/reflection"
)

// A *measured* analysis that found no canary in the response proves the insertion
// point cannot produce reflected XSS. Every payload returned here is paid for with
// a full browser navigation, so the correct answer is "none".
func TestGetPayloadsForContextSkipsNonReflectedInsertionPoints(t *testing.T) {
	analysis := &reflection.ReflectionAnalysis{
		Measured:        true,
		IsReflected:     false,
		ReflectionCount: 0,
	}

	got := GetPayloadsForContext(analysis)

	if len(got) != 0 {
		t.Fatalf("expected no payloads for a measured, non-reflected insertion point, got %d", len(got))
	}
}

// The canary can only be injected into query parameters and form/JSON bodies. For
// headers, cookies, URL paths and XML bodies the probe sends the original request
// unchanged, so IsReflected=false is an artefact of the probe. Skipping those would
// silently delete browser XSS coverage for every one of those insertion point types.
func TestGetPayloadsForContextDoesNotSkipUnmeasuredInsertionPoints(t *testing.T) {
	analysis := &reflection.ReflectionAnalysis{
		Measured:        false,
		IsReflected:     false,
		ReflectionCount: 0,
	}

	got := GetPayloadsForContext(analysis)

	if len(got) == 0 {
		t.Fatal("expected the full payload set when reflection could not be measured for this insertion point type")
	}
}

// A nil analysis means reflection was never measured (analysis disabled, or the
// canary request errored). We cannot narrow, so the full set is still correct.
func TestGetPayloadsForContextFallsBackWhenAnalysisMissing(t *testing.T) {
	got := GetPayloadsForContext(nil)

	if len(got) == 0 {
		t.Fatal("expected the full payload set when no reflection analysis is available")
	}
}

func TestGetPayloadsForContextStillReturnsPayloadsWhenReflected(t *testing.T) {
	analysis := &reflection.ReflectionAnalysis{
		Measured:        true,
		IsReflected:     true,
		ReflectionCount: 1,
		HasHTMLContext:  true,
		Contexts: []reflection.ReflectionContext{
			{Mode: reflection.ModeHTML, Position: 0},
		},
	}

	got := GetPayloadsForContext(analysis)

	if len(got) == 0 {
		t.Fatal("expected payloads for a reflected HTML-context insertion point")
	}
}

func TestGetCSPAwarePayloadsWithDetailsReportsZeroForNonReflected(t *testing.T) {
	analysis := &reflection.ReflectionAnalysis{Measured: true, IsReflected: false}

	result := GetCSPAwarePayloadsWithDetails(analysis, nil)

	if result.FilteredCount != 0 || len(result.Payloads) != 0 {
		t.Fatalf("expected zero payloads for a non-reflected insertion point, got filtered=%d payloads=%d",
			result.FilteredCount, len(result.Payloads))
	}
}
