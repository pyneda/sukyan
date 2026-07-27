package scan

import "testing"

func TestIsCommonSSRFParameter(t *testing.T) {
	shouldMatch := []string{"url", "src", "host", "uri", "dest", "target", "callback_url", "webhook", "imageurl", "feed"}
	for _, param := range shouldMatch {
		if !IsCommonSSRFParameter(param) {
			t.Errorf("expected %q to be recognized as a common SSRF parameter", param)
		}
	}

	shouldNotMatch := []string{"color", "size", "quantity", "sort_order"}
	for _, param := range shouldNotMatch {
		if IsCommonSSRFParameter(param) {
			t.Errorf("expected %q to NOT be recognized as a common SSRF parameter", param)
		}
	}
}

func TestIsLikelySSRFParameter(t *testing.T) {
	likely := []string{"url", "src", "host", "passport_url", "avatarUrl", "jku", "callbackEndpoint", "imageSrc", "webhook_uri"}
	for _, p := range likely {
		if !IsLikelySSRFParameter(p) {
			t.Errorf("expected %q to be treated as a likely SSRF parameter", p)
		}
	}
	unlikely := []string{"q", "page", "sort", "email", "quantity", "port"}
	for _, p := range unlikely {
		if IsLikelySSRFParameter(p) {
			t.Errorf("expected %q not to be treated as a likely SSRF parameter", p)
		}
	}
}
