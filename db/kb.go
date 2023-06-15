package db

var (
	SSRFCode              = "ssrf"
	Log4ShellCode         = "log4shell"
	OOBCommunicationsCode = "oob_communications"
	OSCmdInjectionCode    = "os_cmd_injection"
	BlindSQLInjectionCode = "blind_sql_injection"
	HTTPMethodsCode       = "http_methods"
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
}

func GetIssueTemplateByCode(code string) *Issue {
	for _, issue := range issueTemplates {
		if issue.Code == code {
			return &issue
		}
	}
	return nil
}
