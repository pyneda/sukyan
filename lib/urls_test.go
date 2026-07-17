package lib

import (
	"net/url"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildUrlWithParam(t *testing.T) {
	parsedUrl, err := BuildURLWithParam("https://test.com?s=search&results=10", "s", "alert(1)", false)
	if err != nil {
		t.Error()
	}
	if parsedUrl != "https://test.com?results=10&s=alert(1)" {
		t.Errorf("Expected https://test.com/?results=10&s=alert(1)  Received: %s", parsedUrl)
	}
}

func TestBuildUrlWithSingleParam(t *testing.T) {
	parsedUrl, err := BuildURLWithParam("https://test.com/?q=test", "q", "alert(1)", false)
	if err != nil {
		t.Error()
	}
	expected := "https://test.com/?q=alert(1)"
	if parsedUrl != expected {
		t.Errorf("Expected %s  Received: %s", expected, parsedUrl)
	}
}

func TestBuildUrlAddingParam(t *testing.T) {
	parsedUrl, err := BuildURLWithParam("https://test.com", "q", "alert(1)", false)
	if err != nil {
		t.Error()
	}
	expected := "https://test.com?q=alert(1)"
	if parsedUrl != expected {
		t.Errorf("Expected %s  Received: %s", expected, parsedUrl)
	}
}

func TestGetParametersToTest(t *testing.T) {
	emptyParams := []string{}
	testParams := []string{"q", "category", "hidden"}
	testVisibleParams := []string{"q", "num", "page", "category"}
	testAllParams := []string{"q", "num", "page", "category", "hidden"}
	// Only get the specified params
	params := GetParametersToTest("https://test.com/q?q=test&num=10&page=3&category=test", testParams, false)

	if reflect.DeepEqual(params, testParams) == false {
		t.Error()
	}
	// Get all the parameters
	all := GetParametersToTest("https://test.com/q?q=test&num=10&page=3&category=test", testParams, true)

	for _, v := range testAllParams {
		if Contains(all, v) != true {
			t.Error()
		}
	}
	// Get all the parametesr without specifying
	visible := GetParametersToTest("https://test.com/q?q=test&num=10&page=3&category=test", emptyParams, true)

	for _, v := range testVisibleParams {
		if Contains(visible, v) != true {
			t.Error()
		}
	}

}

func TestBuild404URL(t *testing.T) {
	original := "https://test.com/xyz/?q=test"
	result, err := Build404URL(original)
	if err != nil {
		t.Error()
	}
	if original == result {
		t.Error()
	}
	new, err := url.Parse(result)
	if err != nil {
		t.Error()
	}
	if len(new.Path) < 12 {
		t.Error()
	}
	// log.Info().Str("original", original).Str("result", result).Msg("404 url")
}

func TestGetURLWithoutQueryString(t *testing.T) {
	original := "https://test.com/xyz/?q=test"
	result, err := GetURLWithoutQueryString(original)
	if err != nil {
		t.Error()
	}
	if "https://test.com/xyz" == result {
		t.Error()
	}

}

func TestGetParentURL(t *testing.T) {
	testCases := []struct {
		name       string
		input      string
		wantURL    string
		wantIsRoot bool
		wantErr    bool
	}{
		{
			name:       "Normal URL",
			input:      "https://gorm.io/docs/belongs_to.html",
			wantURL:    "https://gorm.io/docs",
			wantIsRoot: false,
			wantErr:    false,
		},
		{
			name:       "Root URL",
			input:      "https://gorm.io/",
			wantURL:    "https://gorm.io/",
			wantIsRoot: true,
			wantErr:    false,
		},
		{
			name:       "Invalid URL",
			input:      "://gorm.io/",
			wantURL:    "",
			wantIsRoot: false,
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotIsRoot, err := GetParentURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("GetParentURL() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if gotURL != tc.wantURL {
				t.Errorf("GetParentURL() gotURL = %v, want %v", gotURL, tc.wantURL)
			}
			if gotIsRoot != tc.wantIsRoot {
				t.Errorf("GetParentURL() gotIsRoot = %v, want %v", gotIsRoot, tc.wantIsRoot)
			}
		})
	}
}

func TestIsRootURL(t *testing.T) {
	// define test cases
	tests := []struct {
		name     string
		input    string
		expected bool
		err      error
	}{
		{
			name:     "Root URL",
			input:    "https://example.com/",
			expected: true,
			err:      nil,
		},
		{
			name:     "Non-Root URL",
			input:    "https://example.com/path",
			expected: false,
			err:      nil,
		},
		{
			name:     "URL with query parameters",
			input:    "https://example.com/?param=value",
			expected: true,
			err:      nil,
		},
		{
			name:     "Invalid URL",
			input:    "not_a_valid_url",
			expected: false,
			err:      &url.Error{},
		},
	}

	// run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := IsRootURL(tc.input)
			if result != tc.expected || (err != nil) != (tc.err != nil) {
				t.Fatalf("%v expected %v and error status %v, but got %v and error status %v",
					tc.name, tc.expected, tc.err != nil, result, err != nil)
			}
		})
	}
}

func TestCalculateURLDepth(t *testing.T) {
	cases := []struct {
		url   string
		depth int
	}{
		{"http://example.com/path/to/resource", 3},
		{"http://example.com", 0},
		{"http://example.com/path//double", 2},
		{"http://example.com/path/to///triple", 3},
		{"http://example.com/path/with/empty//segments", 4},
		{"http://example.com/path/with/empty//segments/and/query?param=value", 6},
		{"http://example.com/path/with/empty//segments/and/fragment#anchor", 6},
		{"http://example.com/path/with/empty//segments/and/fragment#anchor/looks/like/path", 6},
		{"http://example.com/path///to/resource", 3},
		{"http://example.com/path/to///resource", 3},
		{"", 0},
		{"http://", 0},
		{"http://?id=42", 0},
		{"http://#anchor", 0},
		{"http://example.com//double/leading/slash", 3},
		{"http://example.com/triple///leading/slash", 3},
		{"http://example.com/quad////leading/slash", 3},
		{"http://example.com/////leading/slash", 2},
		{"https://www.example.com/path/to/resource.html", 3},
		{"https://www.example.com/path/to/resource.html?query=value", 3},
		{"https://www.example.com/path/to/resource.html#fragment", 3},
		{"https://www.example.com/path/to/resource.html?query=value#fragment", 3},
		{"https://www.example.com:8080/path/to/resource.html?query=value#fragment", 3},
		{"ftp://example.com/path/to/resource", 3},
		{"file:///path/to/resource", 3},
		{"//example.com/path/to/resource", 3},
		{"?query=value", 0},
		{"/?query=value", 0},
		{"/#fragment", 0},
		{"?query=value#fragment", 0},
		{"/?query=value#fragment", 0},
		{"http://example.com/path/to/resource#fragment/looks/like/path", 3},
		{"http://example.com/path/to/resource?query/looks/like/path=value", 3},
		{"http://example.com/path/to/resource?query=value#fragment/looks/like/path", 3},
	}

	for _, c := range cases {
		got := CalculateURLDepth(c.url)
		if got != c.depth {
			t.Errorf("CalculateURLDepth(%q) == %d, want %d", c.url, got, c.depth)
		}
	}
}

func TestGetUniqueBaseURLs(t *testing.T) {
	tests := []struct {
		name    string
		urls    []string
		want    []string
		wantErr bool
	}{
		{
			name: "Valid URLs",
			urls: []string{
				"http://example.com/path/to/resource",
				"http://example.com/path/to/another/resource",
				"http://example.com",
				"https://another.example.com",
			},
			want: []string{
				"http://example.com",
				"https://another.example.com",
			},
			wantErr: false,
		},
		{
			name: "Invalid URL",
			urls: []string{
				"://invalid.url",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUniqueBaseURLs(tt.urls)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUniqueBaseURLs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Since map iteration is random, we need to sort the slices before comparing.
			sort.Strings(got)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetUniqueBaseURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetHostFromURL(t *testing.T) {
	testCases := []struct {
		url      string
		expected string
		err      bool
	}{
		{
			url:      "http://example.com/path",
			expected: "example.com",
			err:      false,
		},
		{
			url:      "https://www.google.com",
			expected: "www.google.com",
			err:      false,
		},
		{
			url:      "https://192.168.1.1",
			expected: "192.168.1.1",
			err:      false,
		},
		{
			url:      "https://192.168.1.1:8000/aaaa",
			expected: "192.168.1.1",
			err:      false,
		},
		{
			url:      "https://[2001:db8::1]",
			expected: "2001:db8::1",
			err:      false,
		},
		{
			url:      "://invalid_url",
			expected: "",
			err:      true,
		},
	}

	for _, tc := range testCases {
		result, err := GetHostFromURL(tc.url)
		if tc.err && err == nil {
			t.Errorf("expected an error for url: %s", tc.url)
			continue
		}
		if !tc.err && err != nil {
			t.Errorf("did not expect an error for url: %s, got: %v", tc.url, err)
			continue
		}
		if result != tc.expected {
			t.Errorf("expected %s for url: %s, got: %s", tc.expected, tc.url, result)
		}
	}
}

func TestNormalizeURLParams(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple URL with one param",
			rawURL:   "https://example.com/page?param=value",
			expected: "https://example.com/page?param=X",
			wantErr:  false,
		},
		{
			name:     "URL with multiple params",
			rawURL:   "https://example.com/page?param1=value1&param2=value2",
			expected: "https://example.com/page?param1=X&param2=X",
			wantErr:  false,
		},
		{
			name:     "URL with no params",
			rawURL:   "https://example.com/page",
			expected: "https://example.com/page",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURLParams(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeURLParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		urlStr   string
		expected string
		wantErr  bool
	}{
		{
			name:     "URL with path and single param",
			urlStr:   "https://example.com/resource/42?param=value",
			expected: "https://example.com/resource/X?param=X",
			wantErr:  false,
		},
		// {
		// 	name:     "URL with multiple segments and params",
		// 	urlStr:   "https://example.com/dir/subdir/resource?id=123&data=value",
		// 	expected: "https://example.com/dir/subdir/X?id=X&data=X",
		// 	wantErr:  false,
		// },
		{
			name:     "URL with empty path and params",
			urlStr:   "https://example.com/?id=123",
			expected: "https://example.com/?id=X",
			wantErr:  false,
		},
		{
			name:     "Complex URL with multiple query parameters",
			urlStr:   "https://example.com/path/to/resource/page?query1=param1&query2=param2",
			expected: "https://example.com/path/to/resource/page?query1=X&query2=X",
			wantErr:  false,
		},
		{
			name:     "URL with no path and no query",
			urlStr:   "https://example.com",
			expected: "https://example.com",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.urlStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURLPath(t *testing.T) {
	tests := []struct {
		name     string
		urlStr   string
		expected string
		wantErr  bool
	}{
		{
			name:     "fixed route name is not collapsed",
			urlStr:   "https://example.com/resource",
			expected: "https://example.com/resource",
			wantErr:  false,
		},
		{
			name:     "numeric id last segment is collapsed",
			urlStr:   "https://example.com/path/to/12345",
			expected: "https://example.com/path/to/X",
			wantErr:  false,
		},
		{
			name:     "fixed route name in longer path is not collapsed",
			urlStr:   "https://example.com/path/to/resource",
			expected: "https://example.com/path/to/resource",
			wantErr:  false,
		},
		{
			name:     "URL with no path",
			urlStr:   "https://example.com/",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "fixed route name with query is not collapsed",
			urlStr:   "https://example.com/path/resource?query=param",
			expected: "https://example.com/path/resource?query=param",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURLPath(tt.urlStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeURLPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRelativeURL(t *testing.T) {
	relativeURLs := []string{
		"./script.js",
		"../styles/main.css",
		"../../images/logo.png",
		"index.html",
		"about/",
		"../",
		"../..",
	}

	for _, url := range relativeURLs {
		if !IsRelativeURL(url) {
			t.Errorf("Expected '%s' to be a relative URL, but it is not", url)
		}
	}

	absoluteURLs := []string{
		"/home",
		"http://example.com",
		"https://example.com",
	}

	for _, url := range absoluteURLs {
		if IsRelativeURL(url) {
			t.Errorf("Expected '%s' to be an absolute URL, but it is considered as relative", url)
		}
	}

	nonWebURLs := []string{
		"ftp://example.com",
		"mailto:user@example.com",
		"file:///path/to/file",
	}

	for _, url := range nonWebURLs {
		if IsRelativeURL(url) {
			t.Errorf("Expected '%s' to be a non-web URL, but it is considered as relative", url)
		}
	}
}

func TestEncodeQueryValuePreservingPct(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected string
	}{
		{
			name:     "erb/ejs SSTI payload with spaces, angle brackets and percent is made deliverable",
			payload:  "beap<%= 891*395 %>ljse",
			expected: "beap%3C%25%3D%20891*395%20%25%3Eljse",
		},
		{
			name:     "double-brace SSTI payload is left untouched",
			payload:  "beap{{891*395}}ljse",
			expected: "beap{{891*395}}ljse",
		},
		{
			name:     "dollar-brace and hash-brace metachars are preserved raw",
			payload:  "${7*7}#{7*7}*{7*7}@(7*7)",
			expected: "${7*7}#{7*7}*{7*7}@(7*7)",
		},
		{
			name:     "pre-encoded CRLF triplets are preserved, not double-encoded",
			payload:  "%0D%0AX-Injected: yes",
			expected: "%0D%0AX-Injected:%20yes",
		},
		{
			name:     "pre-encoded newline command injection is preserved",
			payload:  "%0A curl x.oast.live",
			expected: "%0A%20curl%20x.oast.live",
		},
		{
			name:     "pre-encoded nosqli payload with %20 %26 %00 is preserved",
			payload:  "'%20%26%26%20this.password.match(/.*/)//+%00",
			expected: "%27%20%26%26%20this.password.match(/.*/)//%2B%00",
		},
		{
			name:     "shell operator ampersands are encoded so they reach the sink literally",
			payload:  "&& curl x",
			expected: "%26%26%20curl%20x",
		},
		{
			name:     "sql like wildcard percent (not followed by two hex) is encoded",
			payload:  "%' AND 1=1",
			expected: "%25%27%20AND%201%3D1",
		},
		{
			name:     "path traversal slashes are preserved",
			payload:  "../../etc/passwd",
			expected: "../../etc/passwd",
		},
		{
			name:     "trailing lone percent is encoded",
			payload:  "abc%",
			expected: "abc%25",
		},
		{
			name:     "percent followed by single hex then non-hex is encoded",
			payload:  "x%2Gy",
			expected: "x%252Gy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeQueryValuePreservingPct(tt.payload)
			if got != tt.expected {
				t.Errorf("EncodeQueryValuePreservingPct(%q) = %q, want %q", tt.payload, got, tt.expected)
			}
		})
	}
}

func TestIsLikelyDynamicPathSegment(t *testing.T) {
	dynamic := []string{
		"123", "0", "42", "9999999",
		"550e8400-e29b-41d4-a716-446655440000",
		"d41d8cd98f00b204e9800998ecf8427e", // md5 hex
		"deadbeefcafebabe",                 // 16-char hex
		"2024-01-30-a1b2c3d4",              // date+hash mix
	}
	fixed := []string{
		"render", "echo", "safe-render", "header-sink", "cookie-sink",
		"json-deep", "oracle", "info", "ua-sink", "referer-sink",
		"ejs", "erb", "jinja", "handlebars", "users", "api", "v1",
		"login", "graphql", "admin", "search", "cafe", // short word, some hex chars but not all
	}
	for _, s := range dynamic {
		if !IsLikelyDynamicPathSegment(s) {
			t.Errorf("IsLikelyDynamicPathSegment(%q) = false, want true (dynamic/ID-like)", s)
		}
	}
	for _, s := range fixed {
		if IsLikelyDynamicPathSegment(s) {
			t.Errorf("IsLikelyDynamicPathSegment(%q) = true, want false (fixed route name)", s)
		}
	}
}

func TestNormalizeURLPathKeepsFixedRouteNames(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://x/engine/ejs/render", "http://x/engine/ejs/render"},
		{"http://x/engine/ejs/echo", "http://x/engine/ejs/echo"},
		{"http://x/engine/ejs/safe-render", "http://x/engine/ejs/safe-render"},
		{"http://x/products/123", "http://x/products/X"},
		{"http://x/users/550e8400-e29b-41d4-a716-446655440000", "http://x/users/X"},
		{"http://x/api/v1/render", "http://x/api/v1/render"},
	}
	for _, tt := range tests {
		got, err := NormalizeURLPath(tt.url)
		if err != nil {
			t.Fatalf("NormalizeURLPath(%q) error: %v", tt.url, err)
		}
		if got != tt.expected {
			t.Errorf("NormalizeURLPath(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}
