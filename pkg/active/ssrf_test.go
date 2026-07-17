package active

import "testing"

func TestSSRFCanaryMatch(t *testing.T) {
	const marker = "SUKYAN_SSRF_CANARY"
	token := "abcdef123456"

	tests := []struct {
		name  string
		body  string
		token string
		want  bool
	}{
		{
			name:  "canary body with marker and matching token",
			body:  `SUKYAN_SSRF_CANARY:abcdef123456:OK`,
			token: token,
			want:  true,
		},
		{
			name:  "marker embedded in JSON response with matching token",
			body:  `{"status":"fetched","content":"SUKYAN_SSRF_CANARY:abcdef123456:OK"}`,
			token: token,
			want:  true,
		},
		{
			name:  "url echo only - token present but marker absent (must NOT fire)",
			body:  `{"url":"http://sukyan.com/ssrf/abcdef123456","body":""}`,
			token: token,
			want:  false,
		},
		{
			name:  "marker present but different token",
			body:  `SUKYAN_SSRF_CANARY:zzzzzzzzzzzz:OK`,
			token: token,
			want:  false,
		},
		{
			name:  "neither marker nor token",
			body:  `{"error":"invalid url"}`,
			token: token,
			want:  false,
		},
		{
			name:  "empty body",
			body:  ``,
			token: token,
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ssrfCanaryMatch(tc.body, marker, tc.token); got != tc.want {
				t.Errorf("ssrfCanaryMatch(%q, %q, %q) = %v, want %v", tc.body, marker, tc.token, got, tc.want)
			}
		})
	}
}
