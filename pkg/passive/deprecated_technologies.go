package passive

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var javaFileExtensions = []string{".jar", ".class", ".jnlp"}

var javaContentTypes = []string{
	"application/java-archive",
	"application/x-java-archive",
	"application/x-java-vm",
	"application/x-java-applet",
	"application/x-java-bean",
	"application/x-jardiff",
	"application/x-jnlp-file",
}

var flashFileExtensions = []string{".swf", ".flv", ".as", ".fla"}
var flashContentTypes = []string{
	"application/x-shockwave-flash",
	"video/x-flv",
	"application/x-flash",
}

var activeXFileExtensions = []string{".cab", ".ocx"}
var activeXContentTypes = []string{"application/x-oleobject"}

func JavaAppletDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)

	var sb strings.Builder
	confidence := 80
	issueDetected := false

	parsedURL, err := url.Parse(item.URL)
	if err == nil {
		for _, ext := range javaFileExtensions {
			if strings.HasSuffix(strings.ToLower(parsedURL.Path), ext) {
				issueDetected = true
				confidence = 100
				sb.WriteString(fmt.Sprintf("Java deployment file detected: %s extension found.\n", ext))
			}
		}
	}

	for _, contentType := range javaContentTypes {
		if strings.Contains(strings.ToLower(item.ResponseContentType), contentType) {
			if !issueDetected {
				issueDetected = true
			}
			confidence = 100
			sb.WriteString(fmt.Sprintf("Java content detected: Response Content-Type is '%s'.\n", contentType))
		}
	}

	for _, pattern := range javaAppletPatterns {
		if pattern.regex.MatchString(matchAgainst) {
			if !issueDetected {
				issueDetected = true
			}
			sb.WriteString(fmt.Sprintf("Java Applet usage detected: Found %s.\n", pattern.description))
		}
	}

	if issueDetected {
		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.JavaAppletDetectedCode,
			sb.String(),
			confidence,
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
			item.ScanID,
			item.ScanJobID,
		)
	}
}

func FlashDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)

	var sb strings.Builder
	confidence := 80
	issueDetected := false

	parsedURL, err := url.Parse(item.URL)
	if err == nil {
		for _, ext := range flashFileExtensions {
			if strings.HasSuffix(strings.ToLower(parsedURL.Path), ext) {
				issueDetected = true
				confidence = 100
				sb.WriteString(fmt.Sprintf("Flash file detected: URL ends with '%s' extension.\n", ext))
			}
		}
	}

	for _, contentType := range flashContentTypes {
		if strings.Contains(strings.ToLower(item.ResponseContentType), contentType) {
			issueDetected = true
			confidence = 100
			sb.WriteString(fmt.Sprintf("Flash content detected: Response Content-Type is '%s'.\n", contentType))
		}
	}

	for _, pattern := range flashPatterns {
		if pattern.regex.MatchString(matchAgainst) {
			issueDetected = true
			sb.WriteString(fmt.Sprintf("Flash usage detected: Found %s.\n", pattern.description))
		}
	}

	if issueDetected {

		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.FlashUsageDetectedCode,
			sb.String(),
			confidence,
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
			item.ScanID,
			item.ScanJobID,
		)
	}
}

func SilverlightDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)

	var sb strings.Builder
	confidence := 80
	issueDetected := false

	parsedURL, err := url.Parse(item.URL)
	if err == nil && strings.HasSuffix(strings.ToLower(parsedURL.Path), ".xap") {
		issueDetected = true
		confidence = 100
		sb.WriteString("Silverlight application package (.xap) detected.\n")
	}

	if strings.Contains(strings.ToLower(item.ResponseContentType), "application/x-silverlight-app") {
		issueDetected = true
		confidence = 100
		sb.WriteString("Silverlight content detected: Response Content-Type is 'application/x-silverlight-app'.\n")
	}

	for _, pattern := range silverlightPatterns {
		if pattern.regex.MatchString(matchAgainst) {
			issueDetected = true
			sb.WriteString(fmt.Sprintf("Silverlight usage detected: Found %s.\n", pattern.description))
		}
	}

	if issueDetected {

		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.SilverlightDetectedCode,
			sb.String(),
			confidence,
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
			item.ScanID,
			item.ScanJobID,
		)
	}
}

func ActiveXDetectionScan(item *db.History) {
	matchAgainst := string(item.RawResponse)

	var sb strings.Builder
	confidence := 80
	issueDetected := false

	parsedURL, err := url.Parse(item.URL)
	if err == nil {
		for _, ext := range activeXFileExtensions {
			if strings.HasSuffix(strings.ToLower(parsedURL.Path), ext) {
				issueDetected = true
				confidence = 100
				sb.WriteString(fmt.Sprintf("ActiveX component detected: URL ends with '%s' extension.\n", ext))
			}
		}
	}

	for _, contentType := range activeXContentTypes {
		if strings.Contains(strings.ToLower(item.ResponseContentType), contentType) {
			issueDetected = true
			confidence = 100
			sb.WriteString(fmt.Sprintf("ActiveX content detected: Response Content-Type is '%s'.\n", contentType))
		}
	}

	for _, pattern := range activeXPatterns {
		if pattern.regex.MatchString(matchAgainst) {
			issueDetected = true
			sb.WriteString(fmt.Sprintf("ActiveX usage detected: Found %s.\n", pattern.description))
		}
	}

	if issueDetected {
		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.ActivexDetectedCode,
			sb.String(),
			confidence,
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
			item.ScanID,
			item.ScanJobID,
		)
	}
}
