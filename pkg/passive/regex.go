package passive

import "regexp"

var privateIPRegex = regexp.MustCompile(`\b((10\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(172\.(1[6-9]|2\d|3[01])\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(192\.168\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(127\.0\.0\.1))\b`)
var fileUploadRegex = regexp.MustCompile(`(?i)<input[^>]*type\s*=\s*["']?file["']?[^>]*>`)
var emailRegex = regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
var sessionTokenRegex = regexp.MustCompile(`(?i)[?&](auth|token|session(?:[_-])?id|jwt|access[_-]token|refresh[_-]token|apikey|api[_-]key|auth[_-]token|login[_-]token|auth[_-]code|client[_-]token|id[_-]token|session[_-]token|security[_-]token|session[_-]id|session[_-]key|sso[_-]token|oauth[_-]token|bearer[_-]token|account[_-]token|session[_-]auth|signature[_-]token|nonce|ticket|code|saml[_-]token|samltoken|jwt[_-]token|verification[_-]token|session[_-]cookie|access[_-]token|session[_-]id[_-]token|tokenid|sso[_-]auth[_-]token|authorization[_-]token|access[_-]key|session[_-]tokenid|authentication[_-]token|auth[_-]key|auth[_-]code|session[_-]key|authcode|sessionkey)=[-\w]*\b`)

var rsaPrivateKeyRegex = regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----[\s\S]*-----END RSA PRIVATE KEY-----`)
var dsaPrivateKeyRegex = regexp.MustCompile(`-----BEGIN DSA PRIVATE KEY-----[\s\S]*-----END DSA PRIVATE KEY-----`)
var ecPrivateKeyRegex = regexp.MustCompile(`-----BEGIN EC PRIVATE KEY-----[\s\S]*-----END EC PRIVATE KEY-----`)
var opensshPrivateKeyRegex = regexp.MustCompile(`-----BEGIN OPENSSH PRIVATE KEY-----[\s\S]*-----END OPENSSH PRIVATE KEY-----`)
var pemPrivateKeyRegex = regexp.MustCompile(`-----BEGIN PRIVATE KEY-----[\s\S]*-----END PRIVATE KEY-----`)

var mongoDBConnectionStringRegex = regexp.MustCompile(`mongodb(\+srv)?:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var postgreSQLConnectionStringRegex = regexp.MustCompile(`postgres:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var postGISConnectionStringRegex = regexp.MustCompile(`postgis:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var mySQLConnectionStringRegex = regexp.MustCompile(`mysql:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var msSQLConnectionStringRegex = regexp.MustCompile(`Server=[\w\.-]+;Database=[a-zA-Z0-9]+;User ID=[a-zA-Z0-9]+;Password=[a-zA-Z0-9]+;`)
var oracleConnectionStringRegex = regexp.MustCompile(`Data Source=(\w+);User ID=(\w+);Password=(\w+);`)
var sqliteConnectionStringRegex = regexp.MustCompile(`Data Source=[\w\.-\/]+;Version=\d+;`)
var redisConnectionStringRegex = regexp.MustCompile(`redis:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var rabbitMQConnectionStringRegex = regexp.MustCompile(`amqp:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var cassandraConnectionStringRegex = regexp.MustCompile(`cassandra:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?\/[a-zA-Z0-9]+`)
var neo4jConnectionStringRegex = regexp.MustCompile(`bolt:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?`)
var couchDBConnectionStringRegex = regexp.MustCompile(`couchdb:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?`)
var influxDBConnectionStringRegex = regexp.MustCompile(`influxdb:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?`)
var memcachedConnectionStringRegex = regexp.MustCompile(`memcached:\/\/[a-zA-Z0-9]+:[a-zA-Z0-9]+@[\w\.-]+(:\d+)?`)

var urlRegex = regexp.MustCompile(`(?:"|')((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,}|(?:/|\.\./|\./)[^"'><,;| *()(%%$^/\\\[\]][^"'><,;|()]{1,}|[a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|/][^"|']{0,}|)|[a-zA-Z0-9_\-]{1,}\.(?:php|asp|aspx|jsp|json|action|html|js|txt|xml)(?:\?[^"|']{0,}|))(?:"|')`)
var jwtRegex = regexp.MustCompile(`[A-Za-z0-9-_]{20,}\.[A-Za-z0-9-_]{20,}\.[A-Za-z0-9-_]{20,}`)

var S3BucketPattern = regexp.MustCompile(`((?:\w+://)?(?:([\w.-]+)\.s3[\w.-]*\.amazonaws\.com|s3(?:[\w.-]*\.amazonaws\.com(?:(?::\d+)?\\?/)*|://)([\w.-]+))(?:(?::\d+)?\\?/)?(?:.*?\?.*Expires=(\d+))?)`)
var GoogleBucketPattern = regexp.MustCompile(`((?:\w+://)?(?:([\w.-]+)\.storage[\w-]*\.googleapis\.com|(?:(?:console\.cloud\.google\.com/storage/browser/|storage\.cloud\.google\.com|storage[\w-]*\.googleapis\.com)(?:(?::\d+)?\\?/)*|gs://)([\w.-]+))(?:(?::\d+)?\\?/([^\\s?'\"#]*))?(?:.*\?.*Expires=(\d+))?)`)
var GcpFirebase = regexp.MustCompile(`([\w.-]+\.firebaseio\.com)`)
var GcpFirestorePattern = regexp.MustCompile(`(firestore\.googleapis\.com.*)`)
var AzureBucketPattern = regexp.MustCompile(`(([\w.-]+\.blob\.core\.windows\.net(?::\d+)?\\?/[\w.-]+)(?:.*?\?.*se=([\w%-]+))?)`)
var AzureTablePattern = regexp.MustCompile(`(([\w.-]+\.table\.core\.windows\.net(?::\d+)?\\?/[\w.-]+)(?:.*?\?.*se=([\w%-]+))?)`)
var AzureQueuePattern = regexp.MustCompile(`(([\w.-]+\.queue\.core\.windows\.net(?::\d+)?\\?/[\w.-]+)(?:.*?\?.*se=([\w%-]+))?)`)
var AzureFilePattern = regexp.MustCompile(`(([\w.-]+\.file\.core\.windows\.net(?::\d+)?\\?/[\w.-]+)(?:.*?\?.*se=([\w%-]+))?)`)
var AzureCosmosPattern = regexp.MustCompile(`(([\w.-]+\.documents\.azure\.com(?::\d+)?\\?/[\w.-]+)(?:.*?\?.*se=([\w%-]+))?)`)
var CloudflareR2Pattern = regexp.MustCompile(`(?:\w+://)?([\w.-]+)\.r2\.dev(/.*)?`)

var bucketsURlsPatternsMap = map[string]*regexp.Regexp{
	"S3Bucket":     S3BucketPattern,
	"GoogleBucket": GoogleBucketPattern,
	"GcpFirebase":  GcpFirebase,
	"GcpFirestore": GcpFirestorePattern,
	"AzureBucket":  AzureBucketPattern,
	"AzureTable":   AzureTablePattern,
	"AzureQueue":   AzureQueuePattern,
	"AzureFile":    AzureFilePattern,
	"AzureCosmos":  AzureCosmosPattern,
	"CloudflareR2": CloudflareR2Pattern,
}

var BucketInvalidURIPattern = regexp.MustCompile(`(?i)(<Code>InvalidURI</Code>|Code: InvalidURI|NoSuchKey)`)
var BucketAccessDeniedPattern = regexp.MustCompile(`(?i)(<Code>AccessDenied</Code>|Code: AccessDenied)`)

var bucketBodyPatternsMap = map[string]*regexp.Regexp{
	"BucketInvalidURI":   BucketInvalidURIPattern,
	"BucketAccessDenied": BucketAccessDeniedPattern,
}
