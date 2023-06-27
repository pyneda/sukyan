package lib

import (
	"encoding/base64"
)

// Helper function to base64 decode a string
func Base64Decode(text string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(text)
}

// Base64Encode just returns a text encoded to base 64
func Base64Encode(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}
