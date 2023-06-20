package passive

import (
	"regexp"
	"testing"
)

func TestSessionTokenRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		// Test cases with inputs that should match the regex
		{"https://example.com/?session_token=abc123", true},
		{"https://example.com/?auth_token=xyz987", true},
		{"https://example.com/?api_key=123456", true},
		{"https://example.com/?id_token=789xyz", true},
		{"https://example.com/?token=789xyz", true},
		{"https://example.com/?session_token=abc123&page=1", true},
		{"https://example.com/?session_token=abc123&access_token=xyz987", true},
		{"https://example.com/?session_cookie=xyz987", true},
		{"https://example.com/?tokenid=123456", true},
		{"https://example.com/?access_token=abcd", true},
		{"https://example.com/?session_tokenid=abc123", true},
		{"https://example.com/?jwt=xyz.xyz", true},
		{"https://example.com/?first=1&second=2&token=xyz.xyz", true},
		{"https://example.com/?authentication_token=abc123", true},
		{"https://example.com/?auth_key=abc123", true},
		{"https://example.com/?auth-code=abc123", true},
		{"https://example.com/?authcode=abc123", true},
		{"https://example.com/?session-key=abc123", true},
		{"https://example.com/?sessionkey=abc123", true},
		{"https://example.com/?auth_KEY=abc123", true},
		{"https://example.com/?page=1&session_token=abc123", true},
		{"https://example.com/?pagesize=10&session_token=abc123", true},
		// Test cases with inputs that should not match the regex
		{"https://example.com/?not_token=123456", false},
		{"https://example.com/", false},
		{"https://example.com/?page=1&pagesize=10", false},
		{"https://example.com/?csrf_token=asdfasf", false},
		{"https://example.com/?session_token", false},
		{"https://example.com/?session_token=", false},
		{"https://example.com/?=abc123", false},
	}

	for _, tc := range testCases {
		match := sessionTokenRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestEmailRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"example@example.com", true},
		{"abc123@gmail.com", true},
		{"first.last@domain.io", true},
		{"special_chars+%.-@example.co.uk", true},
		{"invalid_email.com", false},
		{"missing@sign", false},
		{"@noLocalPart.com", false},
		{"missingDomain@.com", false},
	}

	for _, tc := range testCases {
		match := emailRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestPrivateIPRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},

		{"172.32.0.1", false},
		{"192.169.1.1", false},
		{"256.0.0.1", false},
		{"10.0.0.256", false},
		{"192.168.1.500", false},
	}

	for _, tc := range testCases {
		match := privateIPRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestFileUploadRegex(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"<input type='file'>", true},
		{"<input type=\"file\">", true},
		{"<input type=FILE>", true},
		{"<input type='file' id='upload'>", true},
		{"<input type='file' id='upload'/>", true},

		{"<input type='text'>", false},
		{"<input type=\"submit\">", false},
		{"<input>", false},
		{"<input type='file", false},
	}

	for _, tc := range testCases {
		match := fileUploadRegex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestPrivateKeyRegexes(t *testing.T) {
	testCases := []struct {
		regex    *regexp.Regexp
		input    string
		expected bool
	}{
		{rsaPrivateKeyRegex, "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKqVkA==\n-----END RSA PRIVATE KEY-----", true},
		{dsaPrivateKeyRegex, "-----BEGIN DSA PRIVATE KEY-----\nMIIBvAIBAAKBgQCqVkA==\n-----END DSA PRIVATE KEY-----", true},
		{ecPrivateKeyRegex, "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIKqVkA==\n-----END EC PRIVATE KEY-----", true},
		{opensshPrivateKeyRegex, "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ==\n-----END OPENSSH PRIVATE KEY-----", true},
		{pemPrivateKeyRegex, "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEAqpWQA==\n-----END PRIVATE KEY-----", true},

		// Negative cases
		{rsaPrivateKeyRegex, "-----BEGIN RSA PUBLIC KEY-----\nMIIBOgIBAAJBAKqVkA==\n-----END RSA PUBLIC KEY-----", false},
	}

	for _, tc := range testCases {
		match := tc.regex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Input: %s, Expected: %t, Got: %t", tc.input, tc.expected, match)
		}
	}
}

func TestConnectionStringRegex(t *testing.T) {
	testCases := []struct {
		regex    *regexp.Regexp
		input    string
		expected bool
	}{
		{mongoDBConnectionStringRegex, "mongodb://user:password@localhost:27017/database", true},
		{postgreSQLConnectionStringRegex, "postgres://user:password@localhost:5432/database", true},
		{postGISConnectionStringRegex, "postgis://user:password@localhost:5432/database", true},
		{mySQLConnectionStringRegex, "mysql://user:password@localhost:3306/database", true},
		{msSQLConnectionStringRegex, "Server=localhost;Database=database;User ID=user;Password=password;", true},
		{oracleConnectionStringRegex, "Data Source=localhost;User ID=user;Password=password;", true},
		{sqliteConnectionStringRegex, "Data Source=/path/to/database.db;Version=3;", true},
		{redisConnectionStringRegex, "redis://user:password@localhost:6379/0", true},
		{rabbitMQConnectionStringRegex, "amqp://user:password@localhost:5672/vhost", true},
		{cassandraConnectionStringRegex, "cassandra://user:password@localhost:9042/database", true},
		{neo4jConnectionStringRegex, "bolt://user:password@localhost:7687", true},
		{couchDBConnectionStringRegex, "couchdb://user:password@localhost:5984/database", true},
		{influxDBConnectionStringRegex, "influxdb://user:password@localhost:8086/database", true},
		{memcachedConnectionStringRegex, "memcached://user:password@localhost:11211", true},

		{mongoDBConnectionStringRegex, "https://user:password@localhost:27017/database", false},
		{postgreSQLConnectionStringRegex, "https://user:password@localhost:5432/database", false},
		{postGISConnectionStringRegex, "https://user:password@localhost:5432/database", false},
		{mySQLConnectionStringRegex, "https://user:password@localhost:3306/database", false},
		{msSQLConnectionStringRegex, "Server=localhost;Database=database;Username=user;Password=password;", false},
		{oracleConnectionStringRegex, "DataSource=localhost;User ID=user;Password=password;", false},
		{sqliteConnectionStringRegex, "Data Source=/path/to/database.db;Version=;", false},
		{redisConnectionStringRegex, "https://user:password@localhost:6379/0", false},
		{rabbitMQConnectionStringRegex, "https://user:password@localhost:5672/vhost", false},
		{cassandraConnectionStringRegex, "https://user:password@localhost:9042/database", false},
		{neo4jConnectionStringRegex, "https://user:password@localhost:7687", false},
		{couchDBConnectionStringRegex, "https://user:password@localhost:5984/database", false},
		{influxDBConnectionStringRegex, "https://user:password@localhost:8086/database", false},
		{memcachedConnectionStringRegex, "https://user:password@localhost:11211", false},
	}

	for _, tc := range testCases {
		match := tc.regex.MatchString(tc.input)
		if match != tc.expected {
			t.Errorf("Regex: %v, Input: %s, Expected: %t, Got: %t", tc.regex, tc.input, tc.expected, match)
		}
	}
}

func TestBucketsURIspatternsMap(t *testing.T) {
	tests := map[string]struct {
		pattern *regexp.Regexp
		urls    []string
	}{
		"S3Bucket":     {S3BucketPattern, []string{"https://bucket-name.s3.amazonaws.com/object-name", "bucket-name.s3.amazonaws.com/object-name", "s3.amazonaws.com/bucket-name/object-name"}},
		"GoogleBucket": {GoogleBucketPattern, []string{"https://bucket-name.storage.googleapis.com/object-name", "console.cloud.google.com/storage/browser/bucket-name/object-name", "gs://bucket-name/object-name"}},
		"GcpFirebase":  {GcpFirebase, []string{"https://firebase-project-id.firebaseio.com/data", "firebase-project-id.firebaseio.com/data", "firebase-project-id.firebaseio.com/data"}},
		"GcpFirestore": {GcpFirestorePattern, []string{"https://firestore.googleapis.com/v1/projects/project-id/databases/(default)/documents/collection-id/document-id", "firestore.googleapis.com/v1/projects/project-id/databases/(default)/documents/collection-id/document-id", "firestore.googleapis.com/v1/projects/project-id/databases/(default)/documents/collection-id"}},
		"AzureBucket":  {AzureBucketPattern, []string{"https://myaccount.blob.core.windows.net/mycontainer/myblob", "myaccount.blob.core.windows.net/mycontainer/myblob", "myaccount.blob.core.windows.net/mycontainer/myblob"}},
		"AzureTable":   {AzureTablePattern, []string{"https://myaccount.table.core.windows.net/mytable", "myaccount.table.core.windows.net/mytable", "myaccount.table.core.windows.net/mytable"}},
		"AzureQueue":   {AzureQueuePattern, []string{"https://myaccount.queue.core.windows.net/myqueue", "myaccount.queue.core.windows.net/myqueue", "myaccount.queue.core.windows.net/myqueue"}},
		"AzureFile":    {AzureFilePattern, []string{"https://myaccount.file.core.windows.net/myshare/mydirectory/myfile", "myaccount.file.core.windows.net/myshare/mydirectory/myfile", "myaccount.file.core.windows.net/myshare/mydirectory"}},
		"AzureCosmos":  {AzureCosmosPattern, []string{"https://myaccount.documents.azure.com/mycosmosdb", "myaccount.documents.azure.com/mycosmosdb", "myaccount.documents.azure.com/mycosmosdb"}},
		"CloudflareR2": {CloudflareR2Pattern, []string{"https://bucket-name.r2.dev/object-name", "bucket-name.r2.dev/object-name", "<a href='test.r2.dev/bucket-name/object-name'>Test</a>"}},
	}

	for name, test := range tests {
		for _, url := range test.urls {
			match := test.pattern.MatchString(url)
			if !match {
				t.Errorf("Pattern %s did not match URL %s", name, url)
			}
		}
	}
}

func TestBucketBodyPatterns(t *testing.T) {
	invalidURITests := []struct {
		input    string
		expected bool
	}{
		{"<Code>InvalidURI</Code>", true},
		{"Code: InvalidURI", true},
		{"NoSuchKey", true},
		{"<Code>SomeOtherCode</Code>", false},
		{"Code: SomeOtherCode", false},
	}

	accessDeniedTests := []struct {
		input    string
		expected bool
	}{
		{"<Code>AccessDenied</Code>", true},
		{"Code: AccessDenied", true},
		{"<Code>InvalidURI</Code>", false},
		{"Code: InvalidURI", false},
		{"NoSuchKey", false},
	}

	for _, test := range invalidURITests {
		match := BucketInvalidURIPattern.MatchString(test.input)
		if match != test.expected {
			t.Errorf("For BucketInvalidURIPattern, expected %v for input %s but got %v", test.expected, test.input, match)
		}
	}

	for _, test := range accessDeniedTests {
		match := BucketAccessDeniedPattern.MatchString(test.input)
		if match != test.expected {
			t.Errorf("For BucketAccessDeniedPattern, expected %v for input %s but got %v", test.expected, test.input, match)
		}
	}
}
