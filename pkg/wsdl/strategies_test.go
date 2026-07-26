package wsdl

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateForTypeByCategory(t *testing.T) {
	s := NewDefaultValueStrategy()

	tests := []struct {
		xsdType string
		check   func(string) bool
		desc    string
	}{
		{"xsd:string", func(v string) bool { return v != "" }, "non-empty string"},
		{"xs:int", func(v string) bool { return v == "1" }, "integer 1"},
		{"xsd:boolean", func(v string) bool { return v == "true" || v == "false" }, "boolean literal"},
		{"xsd:decimal", func(v string) bool { return strings.Contains(v, ".") }, "decimal with fraction"},
		{"xsd:negativeInteger", func(v string) bool { return strings.HasPrefix(v, "-") }, "negative"},
		{"xsd:base64Binary", func(v string) bool { return v == "dGVzdA==" }, "valid base64"},
		{"xsd:anyURI", func(v string) bool { return strings.HasPrefix(v, "http") }, "URI"},
	}

	for _, tt := range tests {
		t.Run(tt.xsdType, func(t *testing.T) {
			got := s.GenerateForType(tt.xsdType)
			if !tt.check(got) {
				t.Errorf("GenerateForType(%q) = %q, want %s", tt.xsdType, got, tt.desc)
			}
		})
	}
}

// Date/time values are rejected outright by strongly-typed stacks if the
// lexical form is wrong, so the generated value must round-trip.
func TestGenerateForTypeProducesParseableDates(t *testing.T) {
	s := NewDefaultValueStrategy()

	tests := []struct {
		xsdType string
		layout  string
	}{
		{"xsd:date", "2006-01-02"},
		{"xsd:dateTime", time.RFC3339},
		{"xsd:time", "15:04:05"},
		{"xsd:gYear", "2006"},
		{"xsd:gYearMonth", "2006-01"},
	}

	for _, tt := range tests {
		t.Run(tt.xsdType, func(t *testing.T) {
			got := s.GenerateForType(tt.xsdType)
			if _, err := time.Parse(tt.layout, got); err != nil {
				t.Errorf("GenerateForType(%q) = %q, not parseable as %q: %v", tt.xsdType, got, tt.layout, err)
			}
		})
	}
}

func TestGenerateForTypeIgnoresPrefix(t *testing.T) {
	s := NewDefaultValueStrategy()
	for _, prefixed := range []string{"xsd:int", "xs:int", "int", "{http://www.w3.org/2001/XMLSchema}int"} {
		if got := s.GenerateForType(prefixed); got != "1" {
			t.Errorf("GenerateForType(%q) = %q, want 1", prefixed, got)
		}
	}
}

// An enumerated type has a closed value space. Anything else is rejected before
// the payload can reach the code under test.
func TestSimpleTypeEnumerationIsHonoured(t *testing.T) {
	s := NewDefaultValueStrategy()
	st := &XSDSimpleType{
		Name: "Status",
		Restriction: &XSDRestriction{
			Base:        "xsd:string",
			Enumeration: []string{"Active", "Suspended"},
		},
	}
	if got := s.generateSimpleTypeValue(st); got != "Active" {
		t.Errorf("enumerated value = %q, want Active", got)
	}
}

func TestSimpleTypeFallsBackToBaseType(t *testing.T) {
	s := NewDefaultValueStrategy()
	st := &XSDSimpleType{Restriction: &XSDRestriction{Base: "xsd:int"}}
	if got := s.generateSimpleTypeValue(st); got != "1" {
		t.Errorf("restricted int = %q, want 1", got)
	}
}

func TestSimpleTypeUnionUsesFirstMember(t *testing.T) {
	s := NewDefaultValueStrategy()
	st := &XSDSimpleType{Union: &XSDUnion{MemberTypes: "xsd:int xsd:string"}}
	if got := s.generateSimpleTypeValue(st); got != "1" {
		t.Errorf("union value = %q, want the first member type's value (1)", got)
	}
}

func TestGenerateXMLForElementEscapesValues(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()
	registry.SimpleTypes["Evil"] = &XSDSimpleType{
		Name:        "Evil",
		Restriction: &XSDRestriction{Base: "xsd:string", Enumeration: []string{`<script>&"'`}},
	}

	elem := &XSDElement{Name: "field", Type: "tns:Evil"}
	got := s.GenerateXMLForElement(elem, registry, "", "urn:t", 0)

	for _, raw := range []string{"<script>", `&"`} {
		if strings.Contains(got, raw) {
			t.Errorf("generated XML contains unescaped %q: %s", raw, got)
		}
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped output, got %s", got)
	}
}

// Recursive schemas are common (tree/graph models). Generation must terminate.
func TestRecursiveComplexTypeTerminates(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()
	registry.ComplexTypes["Node"] = &XSDComplexType{
		Name: "Node",
		Sequence: &XSDSequence{
			Elements: []XSDElement{
				{Name: "value", Type: "xsd:string"},
				{Name: "child", Type: "tns:Node"},
			},
		},
	}

	done := make(chan string, 1)
	go func() {
		done <- s.GenerateXMLForElement(&XSDElement{Name: "root", Type: "tns:Node"}, registry, "", "urn:t", 0)
	}()

	select {
	case out := <-done:
		if !strings.Contains(out, "<root>") {
			t.Errorf("expected a root element, got %s", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("recursive type generation did not terminate")
	}
}

func TestComplexTypeValueIncludesAllSequenceFields(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()
	ct := &XSDComplexType{
		Name: "User",
		Sequence: &XSDSequence{Elements: []XSDElement{
			{Name: "id", Type: "xsd:int"},
			{Name: "name", Type: "xsd:string"},
		}},
	}

	value := s.generateComplexTypeValue(ct, registry, 0)
	for _, field := range []string{"id", "name"} {
		if _, ok := value[field]; !ok {
			t.Errorf("generated value missing field %q: %+v", field, value)
		}
	}
}

// complexContent extension must contribute the base type's fields too,
// otherwise inherited parameters are never fuzzed.
func TestComplexContentExtensionIncludesBaseFields(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()
	registry.ComplexTypes["Base"] = &XSDComplexType{
		Name:     "Base",
		Sequence: &XSDSequence{Elements: []XSDElement{{Name: "baseField", Type: "xsd:string"}}},
	}
	derived := &XSDComplexType{
		Name: "Derived",
		ComplexContent: &XSDComplexContent{
			Extension: &XSDExtension{
				Base:     "tns:Base",
				Sequence: &XSDSequence{Elements: []XSDElement{{Name: "derivedField", Type: "xsd:string"}}},
			},
		},
	}

	value := s.generateComplexTypeValue(derived, registry, 0)
	for _, field := range []string{"baseField", "derivedField"} {
		if _, ok := value[field]; !ok {
			t.Errorf("extension value missing %q: %+v", field, value)
		}
	}
}

func TestGenerateXMLForMessagePartByType(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()

	part := &MessagePart{Name: "amount", Type: "xsd:decimal"}
	got := s.GenerateXMLForMessagePart(part, registry, "", "urn:t")
	if !strings.Contains(got, "<amount>") || !strings.Contains(got, "</amount>") {
		t.Errorf("RPC part XML = %q, want an <amount> element", got)
	}
}

func TestGenerateXMLForMessagePartByElement(t *testing.T) {
	s := NewDefaultValueStrategy()
	registry := NewTypeRegistry()
	registry.Elements["GetUser"] = &XSDElement{
		Name: "GetUser",
		ComplexType: &XSDComplexType{
			Sequence: &XSDSequence{Elements: []XSDElement{{Name: "userId", Type: "xsd:string"}}},
		},
	}

	part := &MessagePart{Name: "parameters", Element: "tns:GetUser"}
	got := s.GenerateXMLForMessagePart(part, registry, "", "urn:t")

	if !strings.Contains(got, "<GetUser>") {
		t.Errorf("doc/literal part must emit the element name, got %q", got)
	}
	if !strings.Contains(got, "<userId>") {
		t.Errorf("doc/literal part must emit the wrapper's children, got %q", got)
	}
	if strings.Contains(got, "<parameters>") {
		t.Errorf("the message part name must not appear as an element, got %q", got)
	}
}
