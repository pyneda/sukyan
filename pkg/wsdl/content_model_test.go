package wsdl

import (
	"strings"
	"testing"
	"time"
)

func elementNames(elems []XSDElement) []string {
	names := make([]string, 0, len(elems))
	for _, e := range elems {
		names = append(names, e.Name)
	}
	return names
}

func TestCollectElementsNestedCompositors(t *testing.T) {
	doc := parseFixture(t, "axis_rpc.wsdl")

	ct := doc.TypeRegistry.ComplexTypes["TransferRequest"]
	if ct == nil {
		t.Fatal("TransferRequest not registered")
	}

	got := strings.Join(elementNames(CollectElements(ct, doc.TypeRegistry)), ",")
	for _, want := range []string{"fromAccount", "toAccount", "amount", "currency", "minorUnits", "memo"} {
		if !strings.Contains(got, want) {
			t.Errorf("CollectElements dropped %q; got %s", want, got)
		}
	}
}

func TestCollectElementsFollowsExtensionBase(t *testing.T) {
	registry := NewTypeRegistry()
	registry.ComplexTypes["Base"] = &XSDComplexType{
		Name:     "Base",
		Sequence: &XSDSequence{Elements: []XSDElement{{Name: "id"}, {Name: "createdOn"}}},
	}
	derived := &XSDComplexType{
		Name: "Derived",
		ComplexContent: &XSDComplexContent{
			Extension: &XSDExtension{
				Base:     "tns:Base",
				Sequence: &XSDSequence{Elements: []XSDElement{{Name: "extra"}}},
			},
		},
	}

	got := strings.Join(elementNames(CollectElements(derived, registry)), ",")
	if got != "id,createdOn,extra" {
		t.Errorf("CollectElements = %q, want id,createdOn,extra", got)
	}
}

func TestCollectElementsComplexContentRestriction(t *testing.T) {
	registry := NewTypeRegistry()
	ct := &XSDComplexType{
		ComplexContent: &XSDComplexContent{
			Restriction: &XSDRestriction{
				Base:     "tns:Base",
				Sequence: &XSDSequence{Elements: []XSDElement{{Name: "narrowed"}}},
			},
		},
	}

	if got := elementNames(CollectElements(ct, registry)); len(got) != 1 || got[0] != "narrowed" {
		t.Errorf("CollectElements = %v, want [narrowed]", got)
	}
}

// Cyclic inheritance appears in malformed and hostile schemas; the walker must
// terminate rather than hang a scan worker.
func TestCollectElementsCyclicInheritanceTerminates(t *testing.T) {
	registry := NewTypeRegistry()
	registry.ComplexTypes["A"] = &XSDComplexType{
		Name:           "A",
		ComplexContent: &XSDComplexContent{Extension: &XSDExtension{Base: "tns:B"}},
	}
	registry.ComplexTypes["B"] = &XSDComplexType{
		Name:           "B",
		ComplexContent: &XSDComplexContent{Extension: &XSDExtension{Base: "tns:A"}},
	}

	done := make(chan int, 1)
	go func() { done <- len(CollectElements(registry.ComplexTypes["A"], registry)) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cyclic complexContent extension did not terminate")
	}
}

func TestCollectElementsNilSafe(t *testing.T) {
	if got := CollectElements(nil, NewTypeRegistry()); got != nil {
		t.Errorf("CollectElements(nil) = %v, want nil", got)
	}
	if got := CollectElements(&XSDComplexType{Name: "Empty"}, nil); len(got) != 0 {
		t.Errorf("empty complex type = %v, want no elements", got)
	}
}

// The generated request must actually carry the nested-compositor arguments,
// not merely have them present in the parsed model.
func TestGeneratedBodyIncludesNestedCompositorArguments(t *testing.T) {
	doc := parseFixture(t, "axis_rpc.wsdl")
	endpoints, err := NewGenerator(doc, DefaultGenerationConfig()).GenerateRequests()
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if len(endpoints) == 0 {
		t.Fatal("no endpoints generated")
	}

	body := endpoints[0].Operations[0].Requests[0].Body
	for _, want := range []string{"fromAccount", "toAccount", "currency", "minorUnits"} {
		if !strings.Contains(body, want) {
			t.Errorf("generated body is missing %q:\n%s", want, body)
		}
	}
}
