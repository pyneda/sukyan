package lib

import (
	"net/url"
	"testing"
)

// `#` starts the URL fragment, which is a client-side construct that is never
// transmitted. Leaving it raw in a query value silently truncates every payload
// at the first `#` — the server receives only the prefix. Percent-encoding it
// delivers the identical byte to the application, so encoding is strictly better.
func TestEncodeQueryValueEncodesHashSoPayloadSurvivesTransmission(t *testing.T) {
	payloads := []string{
		"|abcd#{266*74}efgh",      // pug / ruby interpolation
		"abcd#set($x=266*74)efgh", // velocity
		"abcd#{266*74}efgh",       // generic hash-brace
	}

	for _, payload := range payloads {
		encoded := EncodeQueryValuePreservingPct(payload)

		u, err := url.Parse("http://example.com/x?p=" + encoded)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", encoded, err)
		}

		if u.Fragment != "" {
			t.Errorf("payload %q encoded to %q, which url.Parse split into fragment %q — the payload is truncated in transit",
				payload, encoded, u.Fragment)
		}

		got, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", u.RawQuery, err)
		}
		if got.Get("p") != payload {
			t.Errorf("payload %q did not survive a round trip: server would receive %q", payload, got.Get("p"))
		}
	}
}
