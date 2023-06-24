package lib

import (
	"encoding/base64"
)

// Helper function to base64 decode a string
func Base64Decode(input string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(input)
}
