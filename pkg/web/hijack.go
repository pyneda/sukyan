package web

import (
	"encoding/json"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/passive"
	"net/http"
	"strings"

	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// HijackConfig represents a hijack configuration to apply when using the browser
type HijackConfig struct {
	AnalyzeJs   bool
	AnalyzeHTML bool
}

func Hijack(config HijackConfig, browser *rod.Browser) {
	router := browser.HijackRequests()
	// if config.AnalyzeJs {
	// 	router.MustAdd("*.js", func(ctx *rod.Hijack) {
	// 		ctx.MustLoadResponse()
	// 		responseBody := ctx.Response.Body()
	// 		jsSources := passive.FindJsSources(responseBody)
	// 		jsSinks := passive.FindJsSinks(responseBody)
	// 		jquerySinks := passive.FindJquerySinks(responseBody)
	// 		CreateHistoryFromHijack(ctx.Request, ctx.Response, "Javascript file")
	// 		log.Info().Str("url", ctx.Request.URL().String()).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked JS file")
	// 	})
	// }
	ignoreKeywords := []string{"google", "pinterest", "facebook", "instagram", "127.0.0.2"}

	router.MustAdd("*", func(ctx *rod.Hijack) {

		// ctx.MustLoadResponse()
		err := ctx.LoadResponse(http.DefaultClient, true)

		if err != nil {
			log.Error().Err(err).Str("url", ctx.Request.URL().String()).Msg("Error loading hijacked response")
		}

		contentType := ctx.Response.Headers().Get("Content-Type")
		mustSkip := false
		for _, skipWord := range ignoreKeywords {
			if strings.Contains(ctx.Request.URL().Host, skipWord) == true {
				mustSkip = true
			}
		}
		if mustSkip {
			log.Debug().Str("url", ctx.Request.URL().String()).Msg("Skipping processing of hijacked response")
		} else {
			if strings.Contains(contentType, "text/html") {
				responseBody := ctx.Response.Body()
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "HTML Response")
				jsSources := passive.FindJsSources(responseBody)
				jsSinks := passive.FindJsSinks(responseBody)
				jquerySinks := passive.FindJquerySinks(responseBody)
				log.Info().Str("url", ctx.Request.URL().String()).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked HTML response")
				if len(jsSources) > 0 || len(jsSinks) > 0 || len(jquerySinks) > 0 {
					CreateJavascriptSourcesAndSinksInformationalIssue(history, jsSources, jsSinks, jquerySinks)
				}
			} else if strings.Contains(contentType, "javascript") {
				// responseBody := ctx.Response.Body()
				// history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "Javascript Response")
				// jsSources := passive.FindJsSources(responseBody)
				// jsSinks := passive.FindJsSinks(responseBody)
				// jquerySinks := passive.FindJquerySinks(responseBody)
				// log.Info().Str("url", ctx.Request.URL().String()).Strs("sources", jsSources).Strs("jsSinks", jsSinks).Strs("jquerySinks", jquerySinks).Msg("Hijacked Javascript response")
				// if len(jsSources) > 0 || len(jsSinks) > 0 || len(jquerySinks) > 0 {
				// 	CreateJavascriptSourcesAndSinksInformationalIssue(history, jsSources, jsSinks, jquerySinks)
				// }
			} else if strings.Contains(contentType, "application/json") {
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked JSON response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "JSON Response")
			} else if strings.Contains(contentType, "application/ld+json") {
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked JSON-LD response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "JSON-LD Response")
			} else if strings.Contains(contentType, "application/xml") {
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked application/xml response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "")
			} else if strings.Contains(contentType, "text/xml") {
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked text/xml response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "")
			} else if strings.Contains(contentType, "text/csv") {
				log.Warn().Str("url", ctx.Request.URL().String()).Msg("Hijacked CSV response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "")
			} else if strings.Contains(contentType, "text/css") {
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked CSS response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "CSS Response")
			} else if strings.Contains(contentType, "application/x-java-serialized-object") {
				log.Warn().Str("url", ctx.Request.URL().String()).Msg("Hijacked java serialized object response")
				history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "CSS Response")
				issue := db.Issue{
					Code:          "java-serialized-object-detection",
					Title:         "Java serialized object resonse detected",
					Description:   "A java serialized object response has been detected, this would require further manual investigation to check for possible deserialization vulnerabilities",
					Cwe:           0,
					URL:           history.URL,
					StatusCode:    history.StatusCode,
					HTTPMethod:    history.Method,
					Request:       "Not implemented yet",
					Response:      history.ResponseBody,
					Confidence:    90,
					FalsePositive: false,
					Severity:      "Info",
				}
				db.Connection.CreateIssue(issue)
			} else {
				// responseBody := ctx.Response.Body()
				// history := CreateHistoryFromHijack(ctx.Request, ctx.Response, "Non common response")
				CreateHistoryFromHijack(ctx.Request, ctx.Response, "Non common response")
				// jsSources := passive.FindJsSources(responseBody)
				// jsSinks := passive.FindJsSinks(responseBody)
				// jquerySinks := passive.FindJquerySinks(responseBody)
				log.Info().Str("url", ctx.Request.URL().String()).Msg("Hijacked non common response")
				// if len(jsSources) > 0 || len(jsSinks) > 0 || len(jquerySinks) > 0 {
				// 	CreateJavascriptSourcesAndSinksInformationalIssue(history, jsSources, jsSinks, jquerySinks)
				// }
			}
		}

	})
	go router.Run()
}

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
		Request:       "Not implemented",
		Response:      history.ResponseBody,
		FalsePositive: false,
		Confidence:    90,
		Severity:      "Info",
	}
	db.Connection.CreateIssue(issue)
	log.Warn().Str("title", issue.Title).Str("url", issue.URL).Str("description", issue.Description).Msg("Created dangerous-js issue")
}

// CreateHistoryFromHijack saves a history request from hijack request/response items.
func CreateHistoryFromHijack(request *rod.HijackRequest, response *rod.HijackResponse, note string) *db.History {
	requestHeaders, err := json.Marshal(request.Headers())
	if err != nil {
		log.Error().Err(err).Msg("Error converting request headers to json")
	}
	responseHeaders, err := json.Marshal(response.Headers())
	if err != nil {
		log.Error().Err(err).Msg("Error converting response headers to json")
	}
	history := db.History{
		StatusCode:           response.Payload().ResponseCode,
		URL:                  request.URL().String(),
		RequestHeaders:       datatypes.JSON(requestHeaders),
		RequestContentLength: request.Req().ContentLength,
		ResponseHeaders:      datatypes.JSON(responseHeaders),
		ResponseBody:         response.Body(),
		ContentType:          response.Headers().Get("Content-Type"),
		Evaluated:            false,
		Method:               request.Method(),
		Note:                 note,
		Source:               db.SourceHijack,
		// ResponseContentLength: response.ContentLength,

	}
	createdHistory, _ := db.Connection.CreateHistory(&history)
	log.Debug().Interface("history", history).Msg("New history record created")

	return createdHistory
}
