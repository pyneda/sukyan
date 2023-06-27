package lib

import (
	"testing"
)

func TestBase64Encode(t *testing.T) {
	if Base64Encode("test") != "dGVzdA==" {
		t.Error()
	}
}
