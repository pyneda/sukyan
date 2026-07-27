package active

import "testing"

func TestRedirectsOffOrigin(t *testing.T) {
	tests := []struct {
		name       string
		requestURL string
		location   string
		want       bool
	}{
		{
			name:       "absolute foreign host",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "https://sukyan.com/x",
			want:       true,
		},
		{
			name:       "protocol-relative foreign host",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "//sukyan.com/x",
			want:       true,
		},
		{
			name:       "protocol-relative foreign host without path",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "//sukyan.com",
			want:       true,
		},
		{
			name:       "absolute foreign host without path",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "https://sukyan.com",
			want:       true,
		},
		{
			name:       "uppercase foreign host",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "https://SUKYAN.COM/x",
			want:       true,
		},
		{
			name:       "reproduction: normalised relative path containing test domain substring",
			requestURL: "http://127.0.0.1:17080/https:/sukyan.com",
			location:   "/https:/sukyan.com?email=you%40example.com&password=",
			want:       false,
		},
		{
			name:       "plain relative redirect",
			requestURL: "http://127.0.0.1:17080/some/original/path",
			location:   "/some/path",
			want:       false,
		},
		{
			name:       "absolute but same host",
			requestURL: "http://127.0.0.1:17080/anything",
			location:   "https://127.0.0.1:17080/anything",
			want:       false,
		},
		{
			name:       "same host different case",
			requestURL: "http://127.0.0.1:17080/anything",
			location:   "https://127.0.0.1:17080/ANYTHING",
			want:       false,
		},
		{
			name:       "empty location",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "",
			want:       false,
		},
		{
			name:       "unparseable location",
			requestURL: "http://127.0.0.1:17080/redirect",
			location:   "//%5csukyan.com",
			want:       false,
		},
		{
			name:       "unparseable request url",
			requestURL: "://not-a-valid-url",
			location:   "https://sukyan.com/x",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redirectsOffOrigin(tt.requestURL, tt.location)
			if got != tt.want {
				t.Errorf("redirectsOffOrigin(%q, %q) = %v, want %v", tt.requestURL, tt.location, got, tt.want)
			}
		})
	}
}

// Browsers resolve these shapes off-origin while net/url alone does not: `\` is a
// path separator for http(s), leading slash runs collapse, and `https:/host` still
// names an authority. Each case here was a silent miss before browserNormalizedLocation.
func TestRedirectsOffOriginBrowserQuirks(t *testing.T) {
	const requestURL = "http://127.0.0.1:17080/account"
	offOrigin := []string{
		`https:/sukyan.com`,
		`https:sukyan.com`,
		`/\sukyan.com`,
		`\/\/sukyan.com`,
		`//sukyan.com`,
		`https://target.com@sukyan.com`,
		"https://sukyan.com\t",
		" https://sukyan.com",
	}
	for _, loc := range offOrigin {
		if !redirectsOffOrigin(requestURL, loc) {
			t.Errorf("redirectsOffOrigin(%q) = false, want true (a browser leaves the origin)", loc)
		}
	}

	sameOrigin := []string{
		`/https:/sukyan.com/`,
		`/account/sukyan.com`,
		`http://127.0.0.1:17080/sukyan.com`,
		`/`,
		``,
	}
	for _, loc := range sameOrigin {
		if redirectsOffOrigin(requestURL, loc) {
			t.Errorf("redirectsOffOrigin(%q) = true, want false (stays on the origin)", loc)
		}
	}
}
