package db

import (
	"github.com/rs/zerolog/log"
)

var (
	SSRFCode                             = "ssrf"
	Log4ShellCode                        = "log4shell"
	OOBCommunicationsCode                = "oob_communications"
	OSCmdInjectionCode                   = "os_cmd_injection"
	BlindSQLInjectionCode                = "blind_sql_injection"
	HTTPMethodsCode                      = "http_methods"
	MixedContentCode                     = "mixed_content"
	CorsCode                             = "cors"
	PasswordFieldAutocompleteEnabledCode = "password_field_autocomplete_enabled"
	SessionTokenInURLCode                = "session_token_in_url"
	FileUploadDetectedCode               = "file_upload_detected"
	DirectoryListingCode                 = "directory_listing"
	EmailAddressesCode                   = "email_addresses"
	PrivateIPsCode                       = "private_ips"
	PrivateKeysCode                      = "private_keys"
	DBConnectionStringsCode              = "db_connection_strings"
	SNIInjectionCode                     = "sni_injection"
	PasswordInGetRequestCode             = "password_in_get_request"
	JavaSerializedObjectCode             = "java_serialized_object_detected"
	StorageBucketDetectedCode            = "storage_bucket_detected"
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
}

func GetIssueTemplateByCode(code string) *Issue {
	for _, issue := range issueTemplates {
		if issue.Code == code {
			return &issue
		}
	}
	return nil
}

func CreateIssueFromHistoryAndTemplate(history *History, code string, details string, confidence int) {
	issue := GetIssueTemplateByCode(code)
	issue.URL = history.URL
	issue.Request = history.RawRequest
	issue.Response = history.RawResponse
	issue.StatusCode = history.StatusCode
	issue.HTTPMethod = history.Method
	issue.Confidence = confidence
	issue.Details = details
	log.Warn().Interface("issue", issue).Msg("New issue found")
	Connection.CreateIssue(*issue)
}
