package lib

import (
	"encoding/base64"
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

// SanitizeUTF8 removes or replaces invalid UTF-8 byte sequences in a string
func SanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return string([]rune(s))
}
