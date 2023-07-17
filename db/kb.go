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
	JwtDetectedCode                      IssueCode = "jwt_detected"
	WebSocketDetectedCode                IssueCode = "websocket_detected"
	ApacheStrutsDevModeCode              IssueCode = "apache_struts_dev_mode"
	ApacheTapestryExceptionCode          IssueCode = "apache_tapestry_exception"
	GrailsExceptionCode                  IssueCode = "grails_exception"
	DjangoDebugExceptionCode             IssueCode = "django_debug_exception"
	JavaServerHeaderCode                 IssueCode = "java_server_header"
	JettyServerHeaderCode                IssueCode = "jetty_server_header"
	DatabaseErrorsCode                   IssueCode = "database_errors"
	NoSqlInjectionCode                   IssueCode = "nosql_injection"
	SqlInjectionCode                     IssueCode = "sql_injection"
	SecretsInJsCode                      IssueCode = "secrets_in_js"
	XPathInjectionCode                   IssueCode = "xpath_injection"
	RemoteFileInclusionCode              IssueCode = "remote_file_inclusion"
	CSRFCode                             IssueCode = "csrf"
	CRLFInjectionCode                    IssueCode = "crlf_injection"
	ServerSidePrototypePollutionCode     IssueCode = "server_side_prototype_pollution"
	ClientSidePrototypePollutionCode     IssueCode = "client_side_prototype_pollution"
)

type IssueTemplate struct {
	Code        IssueCode `json:"code"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Remediation string    `json:"remediation"`
	Cwe         int       `json:"cwe"`
	Severity    string    `json:"severity"`
	References  []string  `json:"references"`
}

var issueTemplates = []IssueTemplate{
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
	{
		Code:        AspNetMvcHeaderCode,
		Title:       "ASP.NET MVC Header Disclosure",
		Description: "The application discloses the use of ASP.NET MVC. This could aid an attacker in crafting ASP.NET MVC-specific exploits.",
		Remediation: "Configure ASP.NET MVC to stop disclosing this information through headers.",
		Cwe:         200, // Information Exposure
		Severity:    "Low",
	},
	{
		Code:        JwtDetectedCode,
		Title:       "JSON Web Token (JWT) Detected",
		Description: "The application appears to use JSON Web Tokens (JWT). If not properly secured, JWTs can lead to various security issues, including token-based authentication vulnerabilities and identity spoofing.",
		Remediation: "Ensure that JWTs are used securely. Implement proper validation and handling mechanisms. Consider using additional safeguards such as Token Binding, and never expose sensitive information in the payload of the JWT. Always verify the signature of incoming JWTs to confirm they were issued by your system.",
		Cwe:         347, // Improper Verification of Cryptographic Signature
		Severity:    "Info",
	},
	{
		Code:        WebSocketDetectedCode,
		Title:       "WebSockets in Use",
		Description: "The application appears to be using WebSockets. This could potentially lead to real-time interaction, data streaming, or possibly real-time vulnerability exploitation if not properly secured.",
		Remediation: "Ensure that the WebSocket connection is secure (wss://) and that appropriate authentication, authorization, and validation measures are in place for any data transmitted over the WebSocket.",
		Cwe:         749, // Exposed Dangerous Method or Function
		Severity:    "Info",
	},
	{
		Code:        ApacheStrutsDevModeCode,
		Title:       "Apache Struts Dev Mode Detected",
		Description: "The application is running in Apache Struts development mode, which could expose sensitive information or debugging data.",
		Remediation: "Ensure the application is running in production mode to prevent the exposure of sensitive information.",
		Cwe:         215, // Information Exposure Through Debug Information
		Severity:    "Medium",
	},
	{
		Code:        ApacheTapestryExceptionCode,
		Title:       "Apache Tapestry Exception Detected",
		Description: "The application exposes Apache Tapestry exceptions, potentially revealing sensitive information or system details.",
		Remediation: "Configure the application to not expose detailed error messages to end users.",
		Cwe:         209, // Information Exposure Through an Error Message
		Severity:    "Medium",
	},
	{
		Code:        GrailsExceptionCode,
		Title:       "Grails Runtime Exception Detected",
		Description: "The application exposes Grails runtime exceptions, which could provide an attacker with valuable system information.",
		Remediation: "Configure the application to not expose detailed error messages to end users.",
		Cwe:         209, // Information Exposure Through an Error Message
		Severity:    "Medium",
	},
	{
		Code:        DjangoDebugExceptionCode,
		Title:       "Django Debug Page Exception Detected",
		Description: "The application is running in Django's debug mode, which could expose sensitive information or debugging data.",
		Remediation: "Ensure the application is running in production mode to prevent the exposure of sensitive information.",
		Cwe:         215, // Information Exposure Through Debug Information
		Severity:    "Medium",
	},
	{
		Code:        JavaServerHeaderCode,
		Title:       "Java Version Detected",
		Description: "The application's server response header discloses the version of Java in use. This could potentially provide valuable information to an attacker seeking to exploit a known vulnerability in the disclosed Java version.",
		Remediation: "Configure your server to not disclose software version information in its response headers. Alternatively, ensure your software versions are regularly updated to the latest versions, mitigating the risk of known vulnerabilities.",
		Cwe:         200, // CWE-200: Information Exposure
		Severity:    "Low",
	},
	{
		Code:        JettyServerHeaderCode,
		Title:       "Jetty Version Detected",
		Description: "The application's server response header discloses the version of Jetty in use. An attacker can exploit this information to target known vulnerabilities in the disclosed Jetty version.",
		Remediation: "Configure your server to not disclose software version information in its response headers. Alternatively, regularly update your Jetty version to the latest one to reduce the risk of known vulnerabilities.",
		Cwe:         200, // CWE-200: Information Exposure
		Severity:    "Low",
	},
	{
		Code:        DatabaseErrorsCode,
		Title:       "Database Errors Detected",
		Description: "The application exposes database errors, which can leak sensitive information about the database setup and potentially the structure of the underlying data model. This could be valuable information for an attacker looking to exploit the application.",
		Remediation: "Avoid exposing database errors publicly. Consider implementing a global exception handler that can catch any unhandled exceptions and return a generic error message to the user. Detailed error information should be logged for debugging, but should not be exposed to the user or over insecure channels. Regular code reviews and penetration testing can help to identify and mitigate such issues.",
		Cwe:         209,
		Severity:    "Medium",
	},
	{
		Code:        NoSqlInjectionCode,
		Title:       "NoSQL Injection Detected",
		Description: "The application appears to be vulnerable to NoSQL injection attacks. This vulnerability occurs when the application uses user-supplied input to construct NoSQL queries without properly sanitizing or validating the input first. An attacker can exploit this vulnerability to manipulate queries, potentially leading to unauthorized data access, data loss, or data corruption.",
		Remediation: "To mitigate this vulnerability, avoid constructing queries with user-supplied input whenever possible. Instead, use parameterized queries, which can help ensure that user input is not interpreted as part of the query. Implement proper input validation and sanitization procedures. Also, ensure that the least privilege principle is followed, and each function of the application has only the necessary access rights it needs to perform its tasks.",
		Cwe:         943,
		Severity:    "High",
	},
	{
		Code:        SqlInjectionCode,
		Title:       "SQL Injection Detected",
		Description: "The application appears to be vulnerable to SQL injection attacks. This vulnerability occurs when the application uses user-supplied input to construct SQL queries without properly sanitizing or validating the input first. An attacker can exploit this vulnerability to manipulate queries, potentially leading to unauthorized data access, data loss, or data corruption.",
		Remediation: "To mitigate this vulnerability, avoid constructing SQL queries with user-supplied input whenever possible. Instead, use parameterized queries or prepared statements, which can help ensure that user input is not interpreted as part of the SQL command. Implement proper input validation and sanitization procedures. Also, ensure that the least privilege principle is followed, and each function of the application has only the necessary access rights it needs to perform its tasks.",
		Cwe:         89,
		Severity:    "High",
	},
	{
		Code:        SecretsInJsCode,
		Title:       "Exposed Secrets in Javascript",
		Description: "The application appears to contain sensitive data, such as API keys, passwords or cryptographic keys, directly within the JavaScript code. This exposure can lead to critical vulnerabilities as it provides potential attackers with sensitive details that can be used to exploit the application or other related systems.",
		Remediation: "To mitigate this issue, never hard-code secrets into your JavaScript or any other client-side code. Instead, store secrets server-side and ensure they are securely transmitted and only to authenticated and authorized entities. Implement strict access controls and consider using secret management solutions. Regular code reviews can help to identify and remove any accidentally committed secrets.",
		Cwe:         615,
		Severity:    "Medium",
	},
	{
		Code:        XPathInjectionCode,
		Title:       "XPath Injection Detected",
		Description: "The application appears to be vulnerable to XPath injection attacks. This vulnerability occurs when the application uses user-supplied input to construct XPath queries without properly sanitizing or validating the input first. An attacker can exploit this vulnerability to manipulate queries, potentially leading to unauthorized data access, data loss, or data corruption.",
		Remediation: "To mitigate this vulnerability, avoid constructing XPath queries with user-supplied input whenever possible. Instead, use parameterized queries or prepared statements, which can help ensure that user input is not interpreted as part of the XPath command. Implement proper input validation and sanitization procedures. Also, ensure that the least privilege principle is followed, and each function of the application has only the necessary access rights it needs to perform its tasks.",
		Cwe:         643,
		Severity:    "High",
	},
	{
		Code:        RemoteFileInclusionCode,
		Title:       "Remote File Inclusion Detected",
		Description: "The application appears to be vulnerable to Remote File Inclusion (RFI) attacks. This vulnerability occurs when the application uses user-supplied input to include remote files without properly sanitizing or validating the input first. An attacker can exploit this vulnerability to inject malicious scripts, potentially leading to unauthorized data access, data corruption, or even server takeover.",
		Remediation: "To mitigate this vulnerability, avoid including files from remote servers whenever possible. When it is necessary to do so, ensure that the remote file's location is hard-coded or otherwise not influenced by user input. Also, implement proper input validation and sanitization procedures. Regular code reviews and penetration testing can help to identify and mitigate such issues.",
		Cwe:         98, // CWE-98: Remote File Inclusion (RFI)
		Severity:    "High",
		References: []string{
			"https://owasp.org/www-project-web-security-testing-guide/v42/4-Web_Application_Security_Testing/07-Input_Validation_Testing/11.2-Testing_for_Remote_File_Inclusion",
		},
	},
	{
		Code:        CSRFCode,
		Title:       "Cross-Site Request Forgery Detected",
		Description: "The application appears to be vulnerable to Cross-Site Request Forgery (CSRF) attacks. This vulnerability occurs when the application allows an attacker to trick an authenticated user into performing an unwanted action without their consent. An attacker can exploit this vulnerability to carry out actions with the same permissions as the victim, potentially leading to unauthorized data access, data loss, or account compromise.",
		Remediation: "To mitigate this vulnerability, ensure that the application uses anti-CSRF tokens in every form or state changing request. These tokens should be tied to a user's session and included in every form or AJAX request that might result in a change of state for the user's data or settings. Also, make sure the application checks for the presence and correctness of this token before processing any such requests.",
		Cwe:         352, // CWE-352: Cross-Site Request Forgery (CSRF)
		Severity:    "High",
		References: []string{
			"https://owasp.org/www-community/attacks/csrf",
			"https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html",
		},
	},
	{
		Code:        CRLFInjectionCode,
		Title:       "CRLF Injection Detected",
		Description: "The application appears to be vulnerable to CRLF (Carriage Return Line Feed) injection attacks. This vulnerability occurs when the application does not properly sanitize user-supplied input that is then used in HTTP headers. An attacker can exploit this vulnerability to manipulate HTTP headers and control the HTTP response body, potentially leading to HTTP response splitting, session hijacking, cross-site scripting (XSS) attacks, or other injection attacks.",
		Remediation: "To mitigate this vulnerability, sanitize and validate all user-supplied inputs that are incorporated into HTTP headers. Remove or escape CRLF sequences and other control characters. Use allowlists of acceptable inputs, rather than denylists of bad inputs. In addition, configure your web server to ignore or reject HTTP headers that contain CR or LF characters. Regular code reviews and penetration testing can help to identify and mitigate such issues.",
		Cwe:         93, // CWE-93: Improper Neutralization of CRLF Sequences ('CRLF Injection')
		Severity:    "Medium",
		References: []string{
			"https://owasp.org/www-community/vulnerabilities/CRLF_Injection",
		},
	},
	{
		Code:        ServerSidePrototypePollutionCode,
		Title:       "Server-Side Prototype Pollution Detected",
		Description: "The application appears to be vulnerable to Server-Side Prototype Pollution (SSPP) attacks. This vulnerability occurs when the application allows modification of a JavaScript object prototype. When a function traverses the entire prototype chain, an attacker can inject properties into this chain, potentially leading to various impacts, such as denial-of-service, property overwrite, or even remote code execution if the polluted properties are used unsafely.",
		Remediation: "To mitigate this vulnerability, avoid using user-supplied input in the object manipulation functions without proper validation. Validate and sanitize the inputs that are used for configuration. Be aware of the libraries or dependencies that your application uses and keep them updated. Regular code reviews and penetration testing can also help to identify and mitigate such issues.",
		Cwe:         400, // CWE-400: Uncontrolled Resource Consumption
		Severity:    "High",
		References: []string{
			"https://portswigger.net/web-security/prototype-pollution/server-side",
			"https://portswigger.net/research/server-side-prototype-pollution",
			"https://arxiv.org/pdf/2207.11171.pdf",
		},
	},
	{
		Code:        ClientSidePrototypePollutionCode,
		Title:       "Client-Side Prototype Pollution Detected",
		Description: "The application appears to be vulnerable to Client-Side Prototype Pollution (CSPP) attacks. This vulnerability occurs when the application processes user-supplied input with the JavaScript function `Object.assign()`, or uses it to clone an object. An attacker can inject properties into object prototypes, potentially leading to a variety of impacts, including denial-of-service, alteration of script behavior, or cross-site scripting (XSS) if the polluted properties are used in a DOM context.",
		Remediation: "To mitigate this vulnerability, avoid using the `Object.assign()` function with user-supplied input. If user input must be used, ensure it is thoroughly validated and sanitized first. Implement proper input validation and sanitization procedures. Also, be aware of how your client-side code handles object properties and ensure that all code which reads from object properties handles unexpected values correctly.",
		Cwe:         20, // CWE-20: Improper Input Validation
		Severity:    "Low",
		References: []string{
			"https://portswigger.net/web-security/prototype-pollution/client-side",
			"https://book.hacktricks.xyz/pentesting-web/deserialization/nodejs-proto-prototype-pollution/client-side-prototype-pollution",
		},
	},
}

func GetIssueTemplateByCode(code IssueCode) *Issue {
	for _, issueTemplate := range issueTemplates {
		if issueTemplate.Code == code {
			return &Issue{
				Code:        string(issueTemplate.Code),
				Title:       issueTemplate.Title,
				Description: issueTemplate.Description,
				Remediation: issueTemplate.Remediation,
				Cwe:         issueTemplate.Cwe,
				Severity:    issueTemplate.Severity,
				References:  StringSlice(issueTemplate.References),
			}
		}
	}
	return nil
}

func CreateIssueFromHistoryAndTemplate(history *History, code IssueCode, details string, confidence int, severity string) {
	issue := GetIssueTemplateByCode(code)
	issue.URL = history.URL
	issue.Request = history.RawRequest
	issue.Response = history.RawResponse
	issue.StatusCode = history.StatusCode
	issue.HTTPMethod = history.Method
	issue.Confidence = confidence
	issue.Details = details
	if severity != "" {
		issue.Severity = severity
	}
	log.Warn().Str("issue", issue.Title).Str("url", history.URL).Msg("New issue found")
	Connection.CreateIssue(*issue)
}
