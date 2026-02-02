package lib

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Helper function to base64 decode a string
func Base64Decode(text string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(text)
}

// Base64Encode just returns a text encoded to base 64
func Base64Encode(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}

// DecodeBase36 decodes a Base36 string to an integer
func DecodeBase36(s string) (int64, error) {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const maxInt64 = int64(math.MaxInt64)
	result := int64(0)
	for _, char := range s {
		position := strings.IndexRune(charset, char)
		if position == -1 {
			return 0, fmt.Errorf("invalid Base36 character: %c", char)
		}
		if result > (maxInt64-int64(position))/36 {
			return 0, fmt.Errorf("overflow for Base36 string: %s", s)
		}
		result = result*36 + int64(position)
	}
	return result, nil
}

// SanitizeUTF8 removes or replaces invalid UTF-8 byte sequences and null bytes in a string
func SanitizeUTF8(s string) string {
	hasNullBytes := strings.Contains(s, "\x00")
	isValidUTF8 := utf8.ValidString(s)

	if isValidUTF8 && !hasNullBytes {
		return s
	}

	sanitized := string([]rune(s))
	if hasNullBytes {
		sanitized = strings.ReplaceAll(sanitized, "\x00", "")
	}
	return sanitized
}

// JSONToXML converts a JSON object to a simple XML representation.
// If the input is an empty byte slice, returns an empty root element.
// If the input is not valid JSON, wraps the raw content in a root element.
func JSONToXML(jsonBody []byte) ([]byte, error) {
	if len(jsonBody) == 0 {
		return []byte("<?xml version=\"1.0\"?><root></root>"), nil
	}

	var data map[string]any
	if err := json.Unmarshal(jsonBody, &data); err != nil {
		// If not a JSON object, wrap the value
		return []byte(fmt.Sprintf("<?xml version=\"1.0\"?><root>%s</root>", string(jsonBody))), nil
	}

	var xmlBuilder strings.Builder
	xmlBuilder.WriteString("<?xml version=\"1.0\"?>\n<root>\n")
	for key, value := range data {
		// Escape XML special characters in value
		valStr := fmt.Sprintf("%v", value)
		valStr = strings.ReplaceAll(valStr, "&", "&amp;")
		valStr = strings.ReplaceAll(valStr, "<", "&lt;")
		valStr = strings.ReplaceAll(valStr, ">", "&gt;")
		xmlBuilder.WriteString(fmt.Sprintf("  <%s>%s</%s>\n", key, valStr, key))
	}
	xmlBuilder.WriteString("</root>")
	return []byte(xmlBuilder.String()), nil
}

// JSONToFormURLEncoded converts a JSON object to application/x-www-form-urlencoded format.
// Returns an error if the input is not a valid JSON object.
func JSONToFormURLEncoded(jsonBody []byte) ([]byte, error) {
	if len(jsonBody) == 0 {
		return []byte(""), nil
	}

	var data map[string]any
	if err := json.Unmarshal(jsonBody, &data); err != nil {
		return nil, fmt.Errorf("input must be a JSON object: %w", err)
	}

	var pairs []string
	for key, value := range data {
		// URL encode both key and value for proper form encoding
		encodedKey := urlQueryEscape(key)
		encodedValue := urlQueryEscape(fmt.Sprintf("%v", value))
		pairs = append(pairs, encodedKey+"="+encodedValue)
	}
	return []byte(strings.Join(pairs, "&")), nil
}

// urlQueryEscape escapes a string for use in URL query parameters.
// This is a simplified implementation that handles common special characters.
func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteRune(c)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}
