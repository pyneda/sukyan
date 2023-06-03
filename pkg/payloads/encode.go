package payloads

import (
	b64 "encoding/base64"
)

// EncodeBase64 just returns a text encoded to base 64
func EncodeBase64(text string) string {
	return b64.StdEncoding.EncodeToString([]byte(text))
}
