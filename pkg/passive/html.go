package passive

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

func UnencryptedPasswordFormDetectionScan(item *db.History) {
	body, err := item.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Str("historyID", fmt.Sprintf("%d", item.ID)).Msg("Failed to get response body")
		return
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Str("historyID", fmt.Sprintf("%d", item.ID)).Msg("Failed to parse HTML document")
		return
	}

	var details strings.Builder
	numIssues := 0
	confidence := 0

	doc.Find("input[type='password']").Each(func(i int, s *goquery.Selection) {
		inputHtml, err := goquery.OuterHtml(s)
		if err != nil {
			log.Error().Err(err).Uint("history", item.ID).Msg("Failed to get password input HTML")
		}
		form := s.Closest("form")

		action, _ := form.Attr("action")
		securityIssue, issueDescription := evaluateFormSecurity(item.URL, action)

		if securityIssue {
			numIssues++
			if inputHtml != "" {
				details.WriteString(fmt.Sprintf("The following password input has been detected:\n%s\n\n", inputHtml))
			} else {
				details.WriteString("A password input has been detected in the page.\n\n")
			}
			if form.Length() == 0 {
				details.WriteString("No parent form has been detected for the password input, but since the page is served over HTTP, if a request originates from this input, it is assumed to probably also be transmitted over HTTP.")
				confidence = 60
			} else {
				formAttrs := formAttributes(form)
				resolvedAction := resolveFormAction(item.URL, action)
				details.WriteString(fmt.Sprintf("Security issue: %s\n", issueDescription))
				details.WriteString(fmt.Sprintf("Form action resolves to: %s\n", resolvedAction))
				if formAttrs != "" {
					details.WriteString(fmt.Sprintf("Form attributes: %s\n", formAttrs))
				}
				confidence = 90
				// Also check if the form is submitted via GET request and report if so
				method, _ := form.Attr("method")
				if method == "GET" {
					var sb strings.Builder
					sb.WriteString("A form containing a password input is submitted via GET request.\n\n")
					sb.WriteString(fmt.Sprintf("Form action resolves to: %s\n", resolvedAction))
					if formAttrs != "" {
						sb.WriteString(fmt.Sprintf("Form attributes: %s\n", formAttrs))
					}
					db.CreateIssueFromHistoryAndTemplate(item, db.PasswordInGetRequestCode, sb.String(), 90, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
				}
			}
		}

	})

	if numIssues > 0 {
		db.CreateIssueFromHistoryAndTemplate(item, db.UnencryptedPasswordSubmissionCode, details.String(), confidence, "", item.WorkspaceID, item.TaskID, &defaultTaskJobID)
	}
}

func evaluateFormSecurity(pageURL, action string) (bool, string) {
	page, err := url.Parse(pageURL)
	if err != nil {
		return true, "Invalid page URL"
	}

	var actionURL *url.URL
	if action == "" {
		actionURL = page
	} else {
		actionURL, err = url.Parse(action)
		if err != nil {
			return true, "Invalid action URL"
		}
		actionURL = page.ResolveReference(actionURL)
	}

	if page.Scheme == "https" && actionURL.Scheme != "https" {
		return true, "Form submits from HTTPS to HTTP, compromising data security."
	}
	if page.Scheme == "http" && (actionURL.Scheme != "https" || action == "") {
		return true, "Form submits over HTTP, exposing data to interception."
	}

	return false, "Form is secure."
}

func formAttributes(form *goquery.Selection) string {
	var attrs []string
	form.Each(func(i int, s *goquery.Selection) {
		for _, node := range s.Nodes {
			for _, attr := range node.Attr {
				attrs = append(attrs, fmt.Sprintf("%s=\"%s\"", attr.Key, attr.Val))
			}
		}
	})
	return strings.Join(attrs, ", ")
}

func resolveFormAction(pageURL, action string) string {
	page, _ := url.Parse(pageURL)
	actionURL, _ := url.Parse(action)
	resolvedURL := page.ResolveReference(actionURL).String()
	return resolvedURL
}
