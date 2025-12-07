package wsdl

import (
	"net/url"
	"path"
	"strings"
)

// QName represents a qualified name with namespace
type QName struct {
	Namespace string
	LocalPart string
	Prefix    string
}

// ParseQName parses a qualified name string like "tns:localName" or "{namespace}localName"
func ParseQName(qname string, namespaces map[string]string) QName {
	qname = strings.TrimSpace(qname)

	// Handle Clark notation: {namespace}localName
	if strings.HasPrefix(qname, "{") {
		idx := strings.Index(qname, "}")
		if idx > 0 {
			return QName{
				Namespace: qname[1:idx],
				LocalPart: qname[idx+1:],
			}
		}
	}

	// Handle prefix:localName
	if idx := strings.Index(qname, ":"); idx > 0 {
		prefix := qname[:idx]
		localPart := qname[idx+1:]
		ns := ""
		if namespaces != nil {
			ns = namespaces[prefix]
		}
		return QName{
			Prefix:    prefix,
			LocalPart: localPart,
			Namespace: ns,
		}
	}

	// No prefix - use default namespace if available
	ns := ""
	if namespaces != nil {
		ns = namespaces[""]
	}
	return QName{
		LocalPart: qname,
		Namespace: ns,
	}
}

// ExtractLocalName extracts the local part from a QName string
func ExtractLocalName(qname string) string {
	qname = strings.TrimSpace(qname)

	// Handle Clark notation: {namespace}localName
	if strings.HasPrefix(qname, "{") {
		idx := strings.Index(qname, "}")
		if idx > 0 {
			return qname[idx+1:]
		}
	}

	// Handle prefix:localName
	if idx := strings.LastIndex(qname, ":"); idx >= 0 {
		return qname[idx+1:]
	}

	return qname
}

// ExtractPrefix extracts the prefix from a QName string (returns empty if no prefix)
func ExtractPrefix(qname string) string {
	qname = strings.TrimSpace(qname)

	// Clark notation has no prefix
	if strings.HasPrefix(qname, "{") {
		return ""
	}

	if idx := strings.Index(qname, ":"); idx > 0 {
		return qname[:idx]
	}
	return ""
}

// BuildQName creates a qualified name string from namespace and local part
func BuildQName(namespace, localPart, preferredPrefix string) string {
	if namespace == "" {
		return localPart
	}
	if preferredPrefix != "" {
		return preferredPrefix + ":" + localPart
	}
	return "{" + namespace + "}" + localPart
}

// MakeTypeKey creates a unique key for type lookup combining namespace and local name
func MakeTypeKey(namespace, localName string) string {
	if namespace == "" {
		return localName
	}
	return "{" + namespace + "}" + localName
}

// ResolveURL resolves a relative URL against a base URL
func ResolveURL(baseURL, relativeURL string) string {
	if relativeURL == "" {
		return baseURL
	}

	// Check if relativeURL is already absolute
	if strings.HasPrefix(relativeURL, "http://") || strings.HasPrefix(relativeURL, "https://") {
		return relativeURL
	}

	// Parse base URL
	base, err := url.Parse(baseURL)
	if err != nil {
		return relativeURL
	}

	// Parse relative URL
	rel, err := url.Parse(relativeURL)
	if err != nil {
		return relativeURL
	}

	// Resolve
	resolved := base.ResolveReference(rel)
	return resolved.String()
}

// ExtractBaseURL extracts the base URL from a full URL (removes path, query, fragment)
func ExtractBaseURL(fullURL string) string {
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return fullURL
	}

	// Keep scheme and host, clear path/query/fragment
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// ExtractDirectoryURL extracts the directory portion of a URL (for relative imports)
func ExtractDirectoryURL(fullURL string) string {
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return fullURL
	}

	// Get directory of path
	parsed.Path = path.Dir(parsed.Path)
	if parsed.Path == "." {
		parsed.Path = ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// NamespaceMap manages XML namespace prefixes
type NamespaceMap struct {
	prefixToNS map[string]string
	nsToPrefix map[string]string
}

// NewNamespaceMap creates a new namespace map
func NewNamespaceMap() *NamespaceMap {
	return &NamespaceMap{
		prefixToNS: make(map[string]string),
		nsToPrefix: make(map[string]string),
	}
}

// Add adds a prefix-namespace mapping
func (nm *NamespaceMap) Add(prefix, namespace string) {
	nm.prefixToNS[prefix] = namespace
	// Only set reverse mapping if not already present (prefer first prefix)
	if _, exists := nm.nsToPrefix[namespace]; !exists {
		nm.nsToPrefix[namespace] = prefix
	}
}

// GetNamespace returns the namespace for a prefix
func (nm *NamespaceMap) GetNamespace(prefix string) string {
	return nm.prefixToNS[prefix]
}

// GetPrefix returns a prefix for a namespace
func (nm *NamespaceMap) GetPrefix(namespace string) string {
	return nm.nsToPrefix[namespace]
}

// ResolveQName resolves a QName to namespace and local part
func (nm *NamespaceMap) ResolveQName(qname string) (namespace, localPart string) {
	q := ParseQName(qname, nm.prefixToNS)
	return q.Namespace, q.LocalPart
}

// Clone creates a copy of the namespace map
func (nm *NamespaceMap) Clone() *NamespaceMap {
	clone := NewNamespaceMap()
	for k, v := range nm.prefixToNS {
		clone.prefixToNS[k] = v
	}
	for k, v := range nm.nsToPrefix {
		clone.nsToPrefix[k] = v
	}
	return clone
}

// IsEmpty returns true if the map is empty
func (nm *NamespaceMap) IsEmpty() bool {
	return len(nm.prefixToNS) == 0
}

// All returns all prefix-namespace pairs
func (nm *NamespaceMap) All() map[string]string {
	result := make(map[string]string)
	for k, v := range nm.prefixToNS {
		result[k] = v
	}
	return result
}

// XMLEscape escapes special XML characters in a string
func XMLEscape(s string) string {
	var builder strings.Builder
	builder.Grow(len(s))

	for _, r := range s {
		switch r {
		case '&':
			builder.WriteString("&amp;")
		case '<':
			builder.WriteString("&lt;")
		case '>':
			builder.WriteString("&gt;")
		case '"':
			builder.WriteString("&quot;")
		case '\'':
			builder.WriteString("&apos;")
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

// StripXMLDeclaration removes XML declaration if present
func StripXMLDeclaration(xml string) string {
	if strings.HasPrefix(xml, "<?xml") {
		idx := strings.Index(xml, "?>")
		if idx > 0 {
			return strings.TrimSpace(xml[idx+2:])
		}
	}
	return xml
}

// GetSOAPVersion determines SOAP version from namespace
func GetSOAPVersion(namespace string) string {
	switch namespace {
	case SOAP11Namespace, SOAP11EnvelopeNS:
		return "1.1"
	case SOAP12Namespace, SOAP12EnvelopeNS:
		return "1.2"
	default:
		return "1.1" // Default to 1.1
	}
}

// GetSOAPEnvelopeNamespace returns the appropriate SOAP envelope namespace
func GetSOAPEnvelopeNamespace(version string) string {
	if version == "1.2" {
		return SOAP12EnvelopeNS
	}
	return SOAP11EnvelopeNS
}

// GetSOAPContentType returns the appropriate Content-Type header for SOAP version
func GetSOAPContentType(version string) string {
	if version == "1.2" {
		return "application/soap+xml; charset=utf-8"
	}
	return "text/xml; charset=utf-8"
}

// NormalizeWhitespace collapses whitespace in a string (like xs:token)
func NormalizeWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
