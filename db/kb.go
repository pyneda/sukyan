package db

import (
	"github.com/rs/zerolog/log"
)

type IssueCode string

var (
	SSRFCode                             IssueCode = "ssrf"
	Log4ShellCode                        IssueCode = "log4shell"
	OOBCommunicationsCode                IssueCode = "oob_communications"
	OSCmdInjectionCode                   IssueCode = "os_cmd_injection"
	BlindSQLInjectionCode                IssueCode = "blind_sql_injection"
	HTTPMethodsCode                      IssueCode = "http_methods"
	MixedContentCode                     IssueCode = "mixed_content"
	CorsCode                             IssueCode = "cors"
	PasswordFieldAutocompleteEnabledCode IssueCode = "password_field_autocomplete_enabled"
	SessionTokenInURLCode                IssueCode = "session_token_in_url"
	FileUploadDetectedCode               IssueCode = "file_upload_detected"
	DirectoryListingCode                 IssueCode = "directory_listing"
	EmailAddressesCode                   IssueCode = "email_addresses"
	PrivateIPsCode                       IssueCode = "private_ips"
	PrivateKeysCode                      IssueCode = "private_keys"
	DBConnectionStringsCode              IssueCode = "db_connection_strings"
	SNIInjectionCode                     IssueCode = "sni_injection"
	PasswordInGetRequestCode             IssueCode = "password_in_get_request"
	JavaSerializedObjectCode             IssueCode = "java_serialized_object_detected"
	StorageBucketDetectedCode            IssueCode = "storage_bucket_detected"
	XPoweredByHeaderCode                 IssueCode = "x_powered_by_header"
	XASPVersionHeaderCode                IssueCode = "x_asp_version_header"
	ServerHeaderCode                     IssueCode = "server_header"
	ContentTypeHeaderCode                IssueCode = "content_type_header"
	CacheControlHeaderCode               IssueCode = "cache_control_header"
	StrictTransportSecurityHeaderCode    IssueCode = "strict_transport_security_header"
	XFrameOptionsHeaderCode              IssueCode = "x_frame_options_header"
	XXSSProtectionHeaderCode             IssueCode = "x_xss_protection_header"
	AspNetMvcHeaderCode                  IssueCode = "asp_net_mvc_header"
)

var issueTemplates = []Issue{
	{
		Code:        SSRFCode,
		Title:       "Server Side Request Forgery",
		Description: "The application can be tricked into making arbitrary HTTP requests to internal services.",
		Remediation: "Ensure the application does not make requests based on user-supplied data. If necessary, use a whitelist of approved domains.",
		Cwe:         918,
		Severity:    "High",
	},
	{
		Code:        Log4ShellCode,
		Title:       "Log4Shell (Log4j Remote Code Execution)",
		Description: "The application uses a vulnerable version of Log4j that allows remote code execution.",
		Remediation: "Update Log4j to a patched version (2.15.0 or later).",
		Cwe:         502,
		Severity:    "Critical",
	},
	{
		Code:        OOBCommunicationsCode,
		Title:       "Out of Band Communications",
		Description: "The application sends sensitive information to an external server.",
		Remediation: "Ensure all sensitive information is kept within the application and not sent to external servers.",
		Cwe:         201,
		Severity:    "Medium",
	},
	{
		Code:        OSCmdInjectionCode,
		Title:       "OS Command Injection",
		Description: "The application allows the execution of arbitrary operating system commands.",
		Remediation: "Avoid using shell commands in application code. If unavoidable, use strongly typed parameter APIs to prevent injection.",
		Cwe:         78,
		Severity:    "High",
	},
	{
		Code:        BlindSQLInjectionCode,
		Title:       "Blind SQL Injection",
		Description: "The application does not properly sanitize user input, potentially allowing for SQL injection attacks.",
		Remediation: "Ensure all user-supplied input is properly sanitized before being used in SQL queries.",
		Cwe:         89,
		Severity:    "High",
	},
	{
		Code:        HTTPMethodsCode,
		Title:       "HTTP Methods",
		Description: "The application allows the use of potentially dangerous HTTP methods.",
		Remediation: "Make sure the HTTP Methods are properly configured and only the necessary ones are allowed.",
		Cwe:         20,
		Severity:    "Low",
	},
	{
		Code:        CorsCode,
		Title:       "Cross Origin Resource Sharing (CORS)",
		Description: "The application has misconfigured Cross-Origin Resource Sharing (CORS) policies, potentially allowing unauthorized domains access to sensitive data.",
		Remediation: "Ensure that the CORS policies are properly configured to only allow trusted domains to access resources. In many cases, it is advisable to use a whitelist approach where only specifically allowed domains are permitted access.",
		Cwe:         942,
		Severity:    "Medium",
	},
	{
		Code:        MixedContentCode,
		Title:       "Mixed Content",
		Description: "The application serves both secure (HTTPS) and insecure (HTTP) content, which may lead to some content being vulnerable to man-in-the-middle attacks.",
		Remediation: "Ensure all content is served over a secure connection. Use HTTPS for all resources and avoid linking to insecure (HTTP) resources.",
		Cwe:         16,
		Severity:    "Medium",
	},
	{
		Code:        PasswordFieldAutocompleteEnabledCode,
		Title:       "Password Field Autocomplete Enabled",
		Description: "The application's password fields have autocomplete enabled, which may pose a security risk by allowing password autofill on shared or public devices.",
		Remediation: "Disable autocomplete on password fields to prevent passwords from being stored and auto-filled by the browser.",
		Cwe:         200,
		Severity:    "Low",
	},
	{
		Code:        SessionTokenInURLCode,
		Title:       "Session Token In URL",
		Description: "The application includes session tokens in URLs, potentially exposing sensitive data and enabling session hijacking.",
		Remediation: "Do not include session tokens in URLs. Instead, use secure cookies to manage sessions.",
		Cwe:         200,
		Severity:    "Medium",
	},
	{
		Code:        FileUploadDetectedCode,
		Title:       "File Upload Detected",
		Description: "The application allows file uploads, which can expose it to various security vulnerabilities if not properly managed.",
		Remediation: "Ensure that file upload functionality is secured, including validating/sanitizing uploaded files, setting proper file permissions, and storing files in a secure location.",
		Cwe:         434,
		Severity:    "Info",
	},
	{
		Code:        DirectoryListingCode,
		Title:       "Directory Listing Enabled",
		Description: "The application allows directory listings, which could expose sensitive files or information to attackers.",
		Remediation: "Disable directory listings to prevent unauthorized access to file listings.",
		Cwe:         538,
		Severity:    "Low",
	},
	{
		Code:        EmailAddressesCode,
		Title:       "Email Addresses Detected",
		Description: "The application exposes email addresses, potentially making users or administrators targets for spam or phishing attacks.",
		Remediation: "Avoid displaying email addresses publicly, or use techniques to obfuscate them to make it harder for automated tools to collect them.",
		Cwe:         200,
		Severity:    "Low",
	},
	{
		Code:        PrivateIPsCode,
		Title:       "Private IPs Detected",
		Description: "The application exposes private IP addresses, which can provide useful information for potential attackers and expose the internal network structure.",
		Remediation: "Avoid exposing private IP addresses publicly to mitigate potential information leakage.",
		Cwe:         200,
		Severity:    "Low",
	},
	{
		Code:        PrivateKeysCode,
		Title:       "Private Keys Detected",
		Description: "The application exposes private keys, which can provide crucial information for potential attackers and expose the system to unauthorized access.",
		Remediation: "Private keys must be kept confidential and should never be exposed or sent over insecure channels. If a private key has been exposed, it should be considered compromised and a new key pair should be generated.",
		Cwe:         522,
		Severity:    "High",
	},
	{
		Code:        DBConnectionStringsCode,
		Title:       "Database Connection Strings Detected",
		Description: "The application exposes database connection strings, which can provide sensitive information about the database setup, including credentials.",
		Remediation: "Avoid exposing database connection strings publicly to mitigate potential information leakage.",
		Cwe:         200,
		Severity:    "High",
	},
	{
		Code:        SNIInjectionCode,
		Title:       "Server Name Indication (SNI) Injection",
		Description: "The application is vulnerable to Server Name Indication (SNI) Injection. This vulnerability occurs when the application does not validate or incorrectly processes the SNI during the TLS handshake process. An attacker can exploit this to inject arbitrary data, induce abnormal behavior in applications, or conduct Server-Side Request Forgery (SSRF) attacks.",
		Remediation: "Properly validate and sanitize the SNI during the TLS handshake process. Consider implementing additional security measures such as input validation, parameterized queries, or appropriate encoding to prevent injection attacks. Be wary of how your application handles SNI, especially if you are using a Web Application Server (WAS) or Ingress.",
		Cwe:         91,
		Severity:    "Medium",
	},
	{
		Code:        PasswordInGetRequestCode,
		Title:       "Password Submitted in GET Request",
		Description: "The application sends password using GET method, which can lead to sensitive information being logged or leaked.",
		Remediation: "Switch to POST method for submitting passwords or sensitive data, and make sure all such communications happen over a secure connection (HTTPS).",
		Cwe:         598,
		Severity:    "Low",
	},
	{
		Code:        JavaSerializedObjectCode,
		Title:       "Java serialized object resonse detected",
		Description: "A java serialized object response has been detected, this would require further manual investigation to check for possible deserialization vulnerabilities",
		Remediation: "N/A",
		Cwe:         0,
		Severity:    "Info",
	},
	{
		Code:        StorageBucketDetectedCode,
		Title:       "Storage Bucket Detected",
		Description: "The application exposes storage bucket URLs or errors in the response. This can provide sensitive information about the storage setup.",
		Remediation: "Avoid exposing storage bucket URLs or error messages publicly to mitigate potential information leakage. Make sure to handle errors gracefully and avoid revealing any sensitive information in error messages. It's recommended to manually review any detected storage buckets to verify if they are exposing sensitive information.",
		Cwe:         200,
		Severity:    "Info",
	},
	{
		Code:        XPoweredByHeaderCode,
		Title:       "X-Powered-By Header Disclosure",
		Description: "The application discloses the technology it's using through the X-Powered-By header, potentially aiding attackers in crafting specific exploits.",
		Remediation: "Remove the 'X-Powered-By' header or configure your technology to stop disclosing this information.",
		Cwe:         200,
		Severity:    "Low",
	},
	{
		Code:        XASPVersionHeaderCode,
		Title:       "X-AspNet-Version Header Disclosure",
		Description: "The application discloses the ASP.NET version it's using through the X-AspNet-Version header, potentially aiding attackers in crafting specific exploits.",
		Remediation: "Remove the 'X-AspNet-Version' header or configure your ASP.NET application to stop disclosing this information.",
		Cwe:         200,
		Severity:    "Low",
	},
	{
		Code:        ServerHeaderCode,
		Title:       "Server Header Disclosure",
		Description: "The application discloses the server it's using through the Server header, potentially aiding attackers in crafting specific exploits.",
		Remediation: "Remove the 'Server' header or configure your server to stop disclosing this information.",
		Cwe:         200, // Information Exposure
		Severity:    "Low",
	},
	{
		Code:        ContentTypeHeaderCode,
		Title:       "Content Type Header Missing or Incorrect",
		Description: "The application does not correctly specify the content type of the response, potentially leading to security vulnerabilities such as MIME sniffing attacks.",
		Remediation: "Always specify a correct 'Content-Type' header in the response. Use 'X-Content-Type-Options: nosniff' to prevent the browser from MIME-sniffing a response away from the declared content-type.",
		Cwe:         16,
		Severity:    "Medium",
	},
	{
		Code:        CacheControlHeaderCode,
		Title:       "Cache Control Header Misconfiguration",
		Description: "The application's response can be cached, potentially leading to information disclosure or stale content.",
		Remediation: "Configure your application's headers to prevent sensitive information from being cached. You can set 'Cache-Control: no-store' or 'Cache-Control: private' as needed.",
		Cwe:         524, // Information Exposure Through Caching
		Severity:    "Low",
	},
	{
		Code:        StrictTransportSecurityHeaderCode,
		Title:       "Strict-Transport-Security Header Misconfiguration",
		Description: "The application's HTTP Strict Transport Security (HSTS) policy is misconfigured, potentially leading to man-in-the-middle attacks.",
		Remediation: "Configure your application's headers to properly set the HSTS policy, including 'max-age' and optionally 'includeSubDomains' and 'preload'.",
		Cwe:         523, // Unprotected Transport of Credentials
		Severity:    "Low",
	},
	{
		Code:        XFrameOptionsHeaderCode,
		Title:       "X-Frame-Options Header Missing or Incorrect",
		Description: "The application does not correctly specify the X-Frame-Options header, potentially leading to clickjacking attacks.",
		Remediation: "Always specify a correct 'X-Frame-Options' header in the response. Recommended values are 'DENY' or 'SAMEORIGIN'.",
		Cwe:         346, // Origin Validation Error
		Severity:    "Low",
	},
	{
		Code:        XXSSProtectionHeaderCode,
		Title:       "X-XSS-Protection Header Missing or Incorrect",
		Description: "The application does not correctly specify the X-XSS-Protection header, potentially leading to cross-site scripting attacks.",
		Remediation: "Always specify 'X-XSS-Protection: 1; mode=block' in the response header to enable XSS filtering on the client side.",
		Cwe:         79, // Improper Neutralization of Input During Web Page Generation ('Cross-site Scripting')
		Severity:    "Info",
	},
	Issue{
		Code:        AspNetMvcHeaderCode,
		Title:       "ASP.NET MVC Header Disclosure",
		Description: "The application discloses the use of ASP.NET MVC. This could aid an attacker in crafting ASP.NET MVC-specific exploits.",
		Remediation: "Configure ASP.NET MVC to stop disclosing this information through headers.",
		Cwe:         200, // Information Exposure
		Severity:    "Low",
	},
}

func GetIssueTemplateByCode(code IssueCode) *Issue {
	for _, issue := range issueTemplates {
		if issue.Code == code {
			return &issue
		}
	}
	return nil
}

func CreateIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int) {
	issue := GetIssueTemplateByCode(code)
	issue.URL = history.URL
	// issue.Request = history.RawRequest
	// issue.Response = history.RawResponse
	issue.StatusCode = history.StatusCode
	issue.HTTPMethod = history.Method
	issue.Confidence = confidence
	issue.Details = details
	log.Warn().Str("issue", issue.Title).Str("url", history.URL).Msg("New issue found")
	Connection.CreateIssue(*issue)
}
