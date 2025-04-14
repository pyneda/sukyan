package passive

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

// TODO: Refactor this
func CreateJavascriptSourcesAndSinksInformationalIssue(history *db.History, jsSources []string, jsSinks []string, jquerySinks []string) {
	sourcesFound := len(jsSources) > 0
	sinksFound := len(jsSinks) > 0 || len(jquerySinks) > 0
	title := ""
	description := ""

	if sourcesFound && sinksFound {
		title = "Potentially dangerous javascript sources and sinks detected"
		sourcesText := lib.StringsSliceToText(jsSources)
		description = description + "The following potentially dangerous sources have been identified:\n" + sourcesText + "\n\n"
		if len(jsSinks) > 0 {
			jsSinks := lib.StringsSliceToText(jsSinks)
			description = description + "The following potentially dangerous JavaScript sinks have been identified:\n" + jsSinks + "\n\n"
		}
		if len(jquerySinks) > 0 {
			jquerySinks := lib.StringsSliceToText(jquerySinks)
			description = description + "The following potentially dangerous JQuery sinks have been identified:\n" + jquerySinks + "\n\n"
		}
	} else if sourcesFound {
		title = "Potentially dangerous javascript sources detected"
		sourcesText := lib.StringsSliceToText(jsSources)
		description = description + "The following potentially dangerous sources have been identified:\n" + sourcesText + "\n\n"
	} else {
		title = "Potentially dangerous javascript sinks detected"
		if len(jsSinks) > 0 {
			jsSinks := lib.StringsSliceToText(jsSinks)
			description = description + "The following potentially dangerous JavaScript sinks have been identified:\n" + jsSinks + "\n\n"
		}
		if len(jquerySinks) > 0 {
			jquerySinks := lib.StringsSliceToText(jquerySinks)
			description = description + "The following potentially dangerous JQuery sinks have been identified:\n" + jquerySinks + "\n\n"
		}
	}

	description = description + "\n\nThis might need manual analysis."
	issue := db.Issue{
		Title:         title,
		Description:   description,
		Code:          "interesting-js",
		Cwe:           79,
		Payload:       "N/A",
		URL:           history.URL,
		StatusCode:    history.StatusCode,
		HTTPMethod:    history.Method,
		Request:       history.RawRequest,
		Response:      history.RawResponse,
		FalsePositive: false,
		Confidence:    90,
		Severity:      "Info",
	}
	db.Connection.CreateIssue(issue)
	log.Warn().Str("title", issue.Title).Str("url", issue.URL).Str("description", issue.Description).Msg("Created dangerous-js issue")
}
