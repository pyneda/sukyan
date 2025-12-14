package http_utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscoverQueryParamsFromBody(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		expectedParams []string
	}{
		{
			name: "URLSearchParams.get with single quotes",
			responseBody: `<script>
				const urlParams = new URL(location.href).searchParams;
				const query = urlParams.get('query');
				document.write(query);
			</script>`,
			expectedParams: []string{"query"},
		},
		{
			name: "URLSearchParams.get with double quotes",
			responseBody: `<script>
				const params = new URLSearchParams(location.search);
				const id = params.get("id");
				const name = params.get("name");
			</script>`,
			expectedParams: []string{"id", "name"},
		},
		{
			name: "URLSearchParams.has method",
			responseBody: `<script>
				if (urlParams.has('redirect')) {
					window.location = urlParams.get('redirect');
				}
			</script>`,
			expectedParams: []string{"redirect"},
		},
		{
			name: "URLSearchParams.getAll method",
			responseBody: `<script>
				const tags = params.getAll('tag');
				const filters = params.getAll("filter");
			</script>`,
			expectedParams: []string{"tag", "filter"},
		},
		{
			name: "mixed patterns",
			responseBody: `<script>
				const search = urlParams.get('q');
				if (urlParams.has('callback')) {
					eval(urlParams.get('callback'));
				}
				const items = params.getAll("items");
			</script>`,
			expectedParams: []string{"q", "callback", "items"},
		},
		{
			name:           "no parameters found",
			responseBody:   `<script>console.log("hello world");</script>`,
			expectedParams: []string{},
		},
		{
			name: "invalid param names filtered out",
			responseBody: `<script>
				params.get('valid_param');
				params.get('also-valid');
				params.get('with.dot');
				params.get('has spaces');
				params.get('has<special>chars');
				params.get('');
			</script>`,
			expectedParams: []string{"valid_param", "also-valid", "with.dot"},
		},
		{
			name: "very long param name filtered",
			responseBody: `<script>
				params.get('thisparamnameiswaytooolongtobearealisticparameternameandshouldbefilteredout');
				params.get('normal');
			</script>`,
			expectedParams: []string{"normal"},
		},
		{
			name: "real world DOM XSS pattern - level4/5/6 style",
			responseBody: `<script>
				const urlParams = new URL(location.href).searchParams;
				const query = urlParams.get('query');
				document.location.href = query;
			</script>`,
			expectedParams: []string{"query"},
		},
		{
			name: "jQuery style param access",
			responseBody: `<script>
				var url = new URL(window.location);
				var search = url.searchParams.get('search');
				var page = url.searchParams.get('page');
				$('#results').html(search);
			</script>`,
			expectedParams: []string{"search", "page"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiscoverQueryParamsFromBody(tt.responseBody)

			// Check that all expected params are found
			for _, expected := range tt.expectedParams {
				assert.Contains(t, result, expected, "expected param %s not found", expected)
			}

			// Check that we don't have extra unexpected params
			assert.Equal(t, len(tt.expectedParams), len(result),
				"unexpected number of params: got %v, want %v", result, tt.expectedParams)
		})
	}
}

func TestIsValidParamName(t *testing.T) {
	tests := []struct {
		name     string
		param    string
		expected bool
	}{
		{"simple lowercase", "query", true},
		{"simple uppercase", "QUERY", true},
		{"mixed case", "searchQuery", true},
		{"with underscore", "user_id", true},
		{"with hyphen", "user-id", true},
		{"with dot", "config.value", true},
		{"with numbers", "page2", true},
		{"starts with number", "2page", true},
		{"empty string", "", false},
		{"with space", "my param", false},
		{"with special char", "param<script>", false},
		{"with quotes", "param'test", false},
		{"too long", "thisparamnameiswaytooolongtobearealisticparameternameandshouldbefilteredout", false},
		{"exactly 50 chars", "12345678901234567890123456789012345678901234567890", true},
		{"51 chars", "123456789012345678901234567890123456789012345678901", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidParamName(tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiscoverStorageKeysFromBody(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		storageType  string
		expectedKeys []string
	}{
		{
			name: "localStorage.getItem",
			responseBody: `<script>
				var user = localStorage.getItem('user');
				var token = localStorage.getItem("token");
			</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{"user", "token"},
		},
		{
			name: "localStorage.setItem",
			responseBody: `<script>
				localStorage.setItem('config', JSON.stringify(data));
				localStorage.setItem("session", sessionId);
			</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{"config", "session"},
		},
		{
			name: "sessionStorage.getItem",
			responseBody: `<script>
				var temp = sessionStorage.getItem('tempData');
				var form = sessionStorage.getItem("formState");
			</script>`,
			storageType:  "sessionStorage",
			expectedKeys: []string{"tempData", "formState"},
		},
		{
			name: "bracket notation",
			responseBody: `<script>
				var val1 = localStorage['myKey'];
				var val2 = localStorage["otherKey"];
			</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{"myKey", "otherKey"},
		},
		{
			name: "mixed patterns",
			responseBody: `<script>
				localStorage.getItem('key1');
				localStorage.setItem('key2', 'value');
				localStorage['key3'] = 'value';
				var x = localStorage["key4"];
			</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{"key1", "key2", "key3", "key4"},
		},
		{
			name: "only matches specified storage type",
			responseBody: `<script>
				localStorage.getItem('localKey');
				sessionStorage.getItem('sessionKey');
			</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{"localKey"},
		},
		{
			name:         "no keys found",
			responseBody: `<script>console.log("no storage access");</script>`,
			storageType:  "localStorage",
			expectedKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiscoverStorageKeysFromBody(tt.responseBody, tt.storageType)

			for _, expected := range tt.expectedKeys {
				assert.Contains(t, result, expected, "expected key %s not found", expected)
			}

			assert.Equal(t, len(tt.expectedKeys), len(result),
				"unexpected number of keys: got %v, want %v", result, tt.expectedKeys)
		})
	}
}

func TestIsValidStorageKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{"simple key", "user", true},
		{"camelCase", "userName", true},
		{"with underscore", "user_name", true},
		{"with hyphen", "user-name", true},
		{"with dot", "app.config", true},
		{"with numbers", "item123", true},
		{"empty string", "", false},
		{"with space", "my key", false},
		{"with special char", "key<>", false},
		{"too long", "thiskeynameiswaytoolongtobearealisticstoragekeynameandshouldbefilteredout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidStorageKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
