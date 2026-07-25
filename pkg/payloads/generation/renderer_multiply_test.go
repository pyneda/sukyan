package generation

import (
	"fmt"
	"testing"

	"github.com/pyneda/sukyan/lib/integrations"
)

// Arithmetic-oracle payloads (SSTI, code injection) inject `N*M` and then match
// the target's printed result as a string. A float64 result renders in scientific
// notation above ~1e6, producing a marker like "9.98001e+06" that no target will
// ever print — the detection silently stops firing as operand size grows.
func TestMultiplyRendersIntegersAtAnyMagnitude(t *testing.T) {
	cases := []struct {
		a, b     int
		expected string
	}{
		{7, 7, "49"},
		{999, 999, "998001"},
		{999, 9990, "9980010"},
		{716143, 855793, "612870166399"},
		{999999, 999999, "999998000001"},
	}

	for _, tc := range cases {
		got, err := multiply(tc.a, tc.b)
		if err != nil {
			t.Fatalf("multiply(%d, %d): %v", tc.a, tc.b, err)
		}
		if rendered := fmt.Sprintf("%v", got); rendered != tc.expected {
			t.Errorf("multiply(%d, %d) rendered as %q, want %q", tc.a, tc.b, rendered, tc.expected)
		}
	}
}

// Every SSTI payload's expected marker must be the plain integer the template
// engine will print, at the operand magnitude the templates actually use.
func TestSSTIMarkersArePlainIntegers(t *testing.T) {
	generators, err := LoadGenerators("")
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, g := range generators {
		if g.IssueCode != "ssti" {
			continue
		}
		payloads, err := g.BuildPayloads(integrations.InteractionsManager{})
		if err != nil {
			t.Fatalf("%s: %v", g.ID, err)
		}
		for _, p := range payloads {
			for _, dm := range p.DetectionMethods {
				if dm.ResponseCondition == nil {
					continue
				}
				if containsAny(dm.ResponseCondition.Contains, "e+", "E+") {
					t.Errorf("%s: detection marker %q is in scientific notation; the target prints a plain integer",
						g.ID, dm.ResponseCondition.Contains)
				}
				checked++
			}
		}
	}

	if checked == 0 {
		t.Fatal("no ssti detection conditions were checked")
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
