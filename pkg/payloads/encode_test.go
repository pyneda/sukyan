package payloads

import (
	"testing"
)

func TestBase64Encode(t *testing.T) {
	if EncodeBase64("test") != "dGVzdA==" {
		t.Error()
	}
}
