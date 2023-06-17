package lib

import (
	"net/url"
	"reflect"
	"testing"
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
	parsedUrl, err := BuildURLWithParam("https://test.com?q=test", "q", "alert(1)", false)
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
	expected := "https://test.com/?q=alert(1)"
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