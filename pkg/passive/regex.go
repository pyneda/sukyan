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

var apiKeysPatternsMap = map[string]*regexp.Regexp{
	"Amazon MWS Auth Token":             regexp.MustCompile(`amzn\.mws\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),
	"AWS API Key":                       regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	"AWS AppSync GraphQL Key":           regexp.MustCompile(`da2-[a-z0-9]{26}`),
	"Artifactory":                       regexp.MustCompile(`artifactory.{0,50}(\\\"|'|')?[a-zA-Z0-9=]{112}(\\\"|'|')?`),
	"Code Clima":                        regexp.MustCompile(`codeclima.{0,50}(\\\"|'|')?[0-9a-f]{64}(\\\"|'|')?`),
	"Cloudinary Basic Auth":             regexp.MustCompile(`(?i)cloudinary:\\/[0-9]{15}:[0-9A-Za-z]+@[a-z]+`),
	"Facebook Access Token":             regexp.MustCompile(`EAACEdEose0cBA[0-9A-Za-z]+`),
	"Facebook OAuth":                    regexp.MustCompile(`[fF][aA][cC][eE][bB][oO][oO][kK].*['|\"][0-9a-f]{32}['|\"]`),
	"GitHub":                            regexp.MustCompile(`[gG][iI][tT][hH][uU][bB].*['|\"][0-9a-zA-Z]{35,40}['|\"]`),
	"Generic API Key":                   regexp.MustCompile(`[aA][pP][iI]_?[kK][eE][yY].*['|\"][0-9a-zA-Z]{32,45}['|\"]`),
	"Generic Secret":                    regexp.MustCompile(`[sS][eE][cC][rR][eE][tT].*['|\"][0-9a-zA-Z]{32,45}['|\"]`),
	"Google API Key":                    regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`),
	"Google Cloud Platform API Key":     regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`),
	"Google Cloud Platform OAuth":       regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\\.apps\\.googleusercontent\\.com`),
	"Google Drive API Key":              regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`),
	"Google Drive OAuth":                regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\\.apps\\.googleusercontent\\.com`),
	"Google (GCP) Service-account":      regexp.MustCompile(`"type": "service_account"`),
	"Google Gmail API Key":              regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`),
	"Google Gmail OAuth":                regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\\.apps\\.googleusercontent\\.com`),
	"Google OAuth Access Token":         regexp.MustCompile(`ya29\\.[0-9A-Za-z\\-_]+`),
	"Google YouTube API Key":            regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`),
	"Google YouTube OAuth":              regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\\.apps\\.googleusercontent\\.com`),
	"Heroku API Key":                    regexp.MustCompile(`[hH][eE][rR][oO][kK][uU].*[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}`),
	"Hockeyapp":                         regexp.MustCompile(`(?i)hockey.{0,50}(\\\"|'|')?[0-9a-f]{32}(\\\"|'|')?`),
	"MailChimp API Key":                 regexp.MustCompile(`[0-9a-f]{32}-us[0-9]{1,2}`),
	"Mailgun API Key":                   regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
	"New Relic Admin API Key":           regexp.MustCompile(`NRAA-[a-f0-9]{27}`),
	"New Relic Insights Key":            regexp.MustCompile(`NRI(?:I|Q)-[A-Za-z0-9\-_]{32}`),
	"New Relic REST API Key":            regexp.MustCompile(`NRRA-[a-f0-9]{42}`),
	"New Relic Synthetics Location Key": regexp.MustCompile(`NRSP-[a-z]{2}[0-9]{2}[a-f0-9]{31}`),
	"Notion Integration Token":          regexp.MustCompile(`(secret_)([a-zA-Z0-9]{43})`),
	"NuGet API Key":                     regexp.MustCompile(`oy2[a-z0-9]{43}`),
	"Outlook team":                      regexp.MustCompile(`https\\://outlook\\.office.com/webhook/[0-9a-f-]{36}\\@`),
	"OpenAI API Key":                    regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
	"PayPal Braintree Access Token":     regexp.MustCompile(`access_token\\$production\\$[0-9a-z]{16}\\$[0-9a-f]{32}`),
	"Password in URL":                   regexp.MustCompile(`[a-zA-Z]{3,10}://[^/\\s:@]{3,20}:[^/\\s:@]{3,20}@.{1,100}[\"'\\s]`),
	"Picatic API Key":                   regexp.MustCompile(`sk_live_[0-9a-z]{32}`),
	"Riot Games Developer API Key":      regexp.MustCompile(`RGAPI-[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`),
	"Sauce":                             regexp.MustCompile(`(?i)sauce.{0,50}(\\\"|'|')?[0-9a-f-]{36}(\\\"|'|')?`),
	"Shopify Private App Access Token":  regexp.MustCompile(`shppa_[a-fA-F0-9]{32}`),
	"Shopify Custom App Access Token":   regexp.MustCompile(`shpca_[a-fA-F0-9]{32}`),
	"Shopify Access Token":              regexp.MustCompile(`shpat_[a-fA-F0-9]{32}`),
	"Shopify Shared Secret":             regexp.MustCompile(`shpss_[a-fA-F0-9]{32}`),
	"Slack Token":                       regexp.MustCompile(`(xox[pborsa]-[0-9]{12}-[0-9]{12}-[0-9]{12}-[a-z0-9]{32})`),
	"Slack Webhook":                     regexp.MustCompile(`https://hooks\\.slack\\.com/services/T[a-zA-Z0-9_]{8}/B[a-zA-Z0-9_]{8}/[a-zA-Z0-9_]{24}`),
	"Stripe API Key":                    regexp.MustCompile(`(?i)stripe.{0,50}(\\\"|'|')?sk_live_[0-9a-zA-Z]{24}(\\\"|'|')?`),
	"Square Access Token":               regexp.MustCompile(`sq0atp-[0-9A-Za-z\\-_]{22}`),
	"Square Oauth Secret":               regexp.MustCompile(`sq0csp-[0-9A-Za-z\\-_]{43}`),
	"Telegram Bot API Key":              regexp.MustCompile(`[0-9]+:AA[0-9A-Za-z\\-_]{33}`),
	"Twilio API Key":                    regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
	"Twitter Access Token":              regexp.MustCompile(`[tT][wW][iI][tT][tT][eE][rR].*[0-9a-zA-Z]{35,44}`),
}
