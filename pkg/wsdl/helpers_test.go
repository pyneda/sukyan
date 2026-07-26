package wsdl

import "testing"

func TestExtractLocalName(t *testing.T) {
	tests := []struct {
		name  string
		qname string
		want  string
	}{
		{"prefixed", "tns:GetUser", "GetUser"},
		{"unprefixed", "GetUser", "GetUser"},
		{"clark notation", "{urn:demo}GetUser", "GetUser"},
		{"empty", "", ""},
		{"whitespace padded", "  tns:GetUser  ", "GetUser"},
		{"clark with colon in namespace", "{http://example.com/ns}Item", "Item"},
		{"leading colon", ":GetUser", "GetUser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractLocalName(tt.qname); got != tt.want {
				t.Errorf("ExtractLocalName(%q) = %q, want %q", tt.qname, got, tt.want)
			}
		})
	}
}

func TestExtractPrefix(t *testing.T) {
	tests := []struct {
		qname string
		want  string
	}{
		{"tns:GetUser", "tns"},
		{"GetUser", ""},
		{"{urn:demo}GetUser", ""},
		{":GetUser", ""},
	}

	for _, tt := range tests {
		t.Run(tt.qname, func(t *testing.T) {
			if got := ExtractPrefix(tt.qname); got != tt.want {
				t.Errorf("ExtractPrefix(%q) = %q, want %q", tt.qname, got, tt.want)
			}
		})
	}
}

func TestParseQName(t *testing.T) {
	ns := map[string]string{
		"tns": "urn:demo",
		"":    "urn:default",
	}

	tests := []struct {
		name          string
		qname         string
		wantNamespace string
		wantLocal     string
		wantPrefix    string
	}{
		{"prefixed resolves namespace", "tns:GetUser", "urn:demo", "GetUser", "tns"},
		{"unprefixed uses default namespace", "GetUser", "urn:default", "GetUser", ""},
		{"clark notation", "{urn:other}GetUser", "urn:other", "GetUser", ""},
		{"unknown prefix yields empty namespace", "zzz:GetUser", "", "GetUser", "zzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQName(tt.qname, ns)
			if got.Namespace != tt.wantNamespace || got.LocalPart != tt.wantLocal || got.Prefix != tt.wantPrefix {
				t.Errorf("ParseQName(%q) = %+v, want ns=%q local=%q prefix=%q",
					tt.qname, got, tt.wantNamespace, tt.wantLocal, tt.wantPrefix)
			}
		})
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		relative string
		want     string
	}{
		{"absolute passthrough", "http://h/a/", "http://other/x.xsd", "http://other/x.xsd"},
		{"sibling file", "http://h/svc", "types.xsd", "http://h/types.xsd"},
		{"parent traversal", "http://h/a/b", "../types.xsd", "http://h/types.xsd"},
		{"root relative", "http://h/a/b", "/types.xsd", "http://h/types.xsd"},
		{"empty relative returns base", "http://h/a/", "", "http://h/a/"},
		{"trailing slash base keeps directory", "https://h/a/", "t.xsd", "https://h/a/t.xsd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveURL(tt.base, tt.relative); got != tt.want {
				t.Errorf("ResolveURL(%q, %q) = %q, want %q", tt.base, tt.relative, got, tt.want)
			}
		})
	}
}

// WSDLs are very commonly served from a "?wsdl" query URL. Relative schema
// locations must resolve against the directory, with the query discarded.
func TestExtractDirectoryURL(t *testing.T) {
	tests := []struct {
		name string
		full string
		want string
	}{
		{"query string dropped", "http://h/soap/svc?wsdl", "http://h/soap/"},
		{"nested path", "http://h/a/b/c.wsdl", "http://h/a/b/"},
		{"root level file", "http://h/c.wsdl", "http://h/"},
		{"no path", "http://h", "http://h/"},
		{"fragment dropped", "http://h/a/b.wsdl#frag", "http://h/a/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractDirectoryURL(tt.full); got != tt.want {
				t.Errorf("ExtractDirectoryURL(%q) = %q, want %q", tt.full, got, tt.want)
			}
		})
	}
}

func TestResolveURLFromQueryStringWSDL(t *testing.T) {
	base := ExtractDirectoryURL("http://h/soap/svc?wsdl")
	got := ResolveURL(base, "types.xsd")
	if want := "http://h/soap/types.xsd"; got != want {
		t.Errorf("relative import from ?wsdl URL = %q, want %q", got, want)
	}
}

func TestXMLEscape(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`a<b`, "a&lt;b"},
		{`a>b`, "a&gt;b"},
		{`a&b`, "a&amp;b"},
		{`a"b`, "a&quot;b"},
		{`a'b`, "a&apos;b"},
		{`<script>alert(1)</script>`, "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"plain", "plain"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := XMLEscape(tt.in); got != tt.want {
				t.Errorf("XMLEscape(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetSOAPVersionAndContentType(t *testing.T) {
	if got := GetSOAPVersion(SOAP12Namespace); got != "1.2" {
		t.Errorf("GetSOAPVersion(soap12) = %q, want 1.2", got)
	}
	if got := GetSOAPVersion(SOAP11Namespace); got != "1.1" {
		t.Errorf("GetSOAPVersion(soap11) = %q, want 1.1", got)
	}
	if got := GetSOAPVersion("urn:unknown"); got != "1.1" {
		t.Errorf("GetSOAPVersion(unknown) = %q, want 1.1 default", got)
	}

	if got := GetSOAPContentType("1.2"); got != "application/soap+xml; charset=utf-8" {
		t.Errorf("GetSOAPContentType(1.2) = %q", got)
	}
	if got := GetSOAPContentType("1.1"); got != "text/xml; charset=utf-8" {
		t.Errorf("GetSOAPContentType(1.1) = %q", got)
	}
}

func TestNamespaceMap(t *testing.T) {
	nm := NewNamespaceMap()
	if !nm.IsEmpty() {
		t.Fatal("new namespace map should be empty")
	}

	nm.Add("tns", "urn:demo")
	nm.Add("alt", "urn:demo")

	if got := nm.GetNamespace("tns"); got != "urn:demo" {
		t.Errorf("GetNamespace(tns) = %q", got)
	}
	if got := nm.GetPrefix("urn:demo"); got != "tns" {
		t.Errorf("GetPrefix should prefer the first prefix registered, got %q", got)
	}

	clone := nm.Clone()
	clone.Add("tns", "urn:changed")
	if nm.GetNamespace("tns") != "urn:demo" {
		t.Error("Clone must not alias the original map")
	}
}

func TestMakeTypeKey(t *testing.T) {
	if got := MakeTypeKey("urn:demo", "User"); got != "{urn:demo}User" {
		t.Errorf("MakeTypeKey = %q", got)
	}
	if got := MakeTypeKey("", "User"); got != "User" {
		t.Errorf("MakeTypeKey with empty namespace = %q", got)
	}
}

func TestStripXMLDeclaration(t *testing.T) {
	in := "<?xml version=\"1.0\"?>\n<root/>"
	if got := StripXMLDeclaration(in); got != "<root/>" {
		t.Errorf("StripXMLDeclaration = %q", got)
	}
	if got := StripXMLDeclaration("<root/>"); got != "<root/>" {
		t.Errorf("StripXMLDeclaration without declaration = %q", got)
	}
}

func TestIsXSDBuiltinType(t *testing.T) {
	for _, name := range []string{"string", "int", "boolean", "dateTime", "base64Binary", "anyType"} {
		if !IsXSDBuiltinType(name) {
			t.Errorf("IsXSDBuiltinType(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"User", "Address", "", "MyString"} {
		if IsXSDBuiltinType(name) {
			t.Errorf("IsXSDBuiltinType(%q) = true, want false", name)
		}
	}
}
