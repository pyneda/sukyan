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
}

func GetIssueTemplateByCode(code string) *Issue {
	for _, issue := range issueTemplates {
		if issue.Code == code {
			return &issue
		}
	}
	return nil
}

func CreateIssueFromHistoryAndTemplate(history *History, code string, confidence int) {
	issue := GetIssueTemplateByCode(code)
	issue.URL = history.URL
	issue.Request = history.RawRequest
	issue.Response = history.RawResponse
	issue.StatusCode = history.StatusCode
	issue.HTTPMethod = history.Method
	issue.Confidence = confidence
	log.Warn().Interface("issue", issue).Msg("New issue found")
	Connection.CreateIssue(*issue)
}
