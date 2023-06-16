package web

import (
	"encoding/json"
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/web/cookies"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

func InspectMultipleURLs(urls []string) (urlsData []WebPage) {
	for _, u := range urls {
		data := InspectURL(u)
		urlsData = append(urlsData, data)
	}
	return urlsData
}

// InspectURL : Inspects an url
func InspectURL(url string) WebPage {
	browser := rod.New().MustConnect()
	// defer browser.MustClose()
	//browser.MustIgnoreCertErrors(true)
	// Hijack requests
	Hijack(HijackConfig{AnalyzeJs: true, AnalyzeHTML: true}, browser)

	page := browser.MustPage("")
	IgnoreCertificateErrors(page)
	// Enabling audits, security, etc
	auditEnableError := proto.AuditsEnable{}.Call(page)
	if auditEnableError != nil {
		log.Error().Err(auditEnableError).Str("url", url).Msg("Error enabling browser audit events")
	}
	securityEnableError := proto.SecurityEnable{}.Call(page)
	if securityEnableError != nil {
		log.Error().Err(securityEnableError).Str("url", url).Msg("Error enabling browser security events")
	}

	go page.EachEvent(
		func(e *proto.BackgroundServiceBackgroundServiceEventReceived) {
			log.Warn().Interface("event", e).Msg("Received background service event")
		},
		func(e *proto.StorageIndexedDBContentUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
		},
		func(e *proto.StorageCacheStorageListUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageCacheStorageListUpdated event")
		},
		func(e *proto.StorageIndexedDBContentUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBContentUpdated event")
		},
		func(e *proto.StorageIndexedDBListUpdated) {
			log.Warn().Interface("event", e).Msg("Received StorageIndexedDBListUpdated event")
		},
		func(e *proto.DatabaseAddDatabase) {
			log.Warn().Interface("database", e).Msg("Client side database has been added")
			dbIssueDescription := fmt.Sprintf("Dynamic analisis has detected that a client side database with name %s and ID %s has been registered on domain %s. This is not an issue bug might require further investigation.", e.Database.Name, e.Database.ID, e.Database.Domain)
			dbAddedIssue := db.Issue{
				Code:        "client-db-added",
				URL:         url,
				Title:       "A client side database event has been triggered",
				Cwe:         1,
				StatusCode:  200,
				HTTPMethod:  "GET?",
				Description: dbIssueDescription,
				Payload:     "N/A",
				Confidence:  99,
				Severity:    "Info",
			}
			db.Connection.CreateIssue(dbAddedIssue)
		},
		// func(e *proto.DebuggerScriptParsed) {
		// 	log.Debug().Interface("parsed script data", e).Msg("Debugger script parsed")
		// },
		func(e *proto.AuditsIssueAdded) {
			log.Warn().Interface("issue", e.Issue).Str("url", url).Msg("Received a new browser audits issue")
			jsonDetails, err := json.Marshal(e.Issue.Details)
			if err != nil {
				log.Error().Err(err).Str("url", url).Msg("Could not convert browser audit issue event details to JSON")
			}
			// Assume it is a Mixed Content Issue Details
			// if e.Issue.Details.MixedContentIssueDetails != proto.AuditsMixedContentIssueDetails {
			// 	// if e.Issue.Details.MixedContentIssueDetails.InsecureURL != "" {

			// 	var description strings.Builder
			// 	description.WriteString("A mixed content issue has been found in " + url + "\nThe insecure content loaded url comes from: " + e.Issue.Details.MixedContentIssueDetails.InsecureURL)
			// 	if e.Issue.Details.MixedContentIssueDetails.Frame.FrameID != "" {
			// 		description.WriteString("\nAffected frame: " + string(e.Issue.Details.MixedContentIssueDetails.Frame.FrameID))
			// 	}
			// 	if e.Issue.Details.MixedContentIssueDetails.ResourceType != "" {
			// 		description.WriteString("\nResource type: " + string(e.Issue.Details.MixedContentIssueDetails.ResourceType))
			// 	}
			// 	if e.Issue.Details.MixedContentIssueDetails.ResolutionStatus != "" {
			// 		description.WriteString("\nResolution status: " + string(e.Issue.Details.MixedContentIssueDetails.ResolutionStatus))
			// 	}
			// 	browserAuditIssue := db.Issue{
			// 		Code:           string(e.Issue.Code),
			// 		URL:            url,
			// 		Title:          "Mixed Content Issue (Browser Audit)",
			// 		Cwe:            1,
			// 		StatusCode:     200,
			// 		HTTPMethod:     "GET?",
			// 		Description:    description.String(),
			// 		Payload:        "N/A",
			// 		Confidence:     80,
			// 		AdditionalInfo: jsonDetails,
			// 	}
			// 	db.Connection.CreateIssue(browserAuditIssue)
			// } else {
			// Generic while dont have customized for every event type
			browserAuditIssue := db.Issue{
				Code:           "browser-audit-" + string(e.Issue.Code),
				URL:            url,
				Title:          "Browser audit issue (classification needed)",
				Cwe:            1,
				StatusCode:     200,
				HTTPMethod:     "GET?",
				Description:    string(jsonDetails),
				Payload:        "N/A",
				Confidence:     80,
				AdditionalInfo: jsonDetails,
				Severity:       "Low",
			}
			db.Connection.CreateIssue(browserAuditIssue)
			// }

		},
		func(e *proto.SecuritySecurityStateChanged) (stop bool) {
			if e.Summary == "all served securely" {
				log.Warn().Interface("state", e).Str("url", url).Msg("Received a new browser SecuritySecurityStateChanged event without issues")
				return true
			} else {
				log.Warn().Interface("state", e).Str("url", url).Msg("Received a new browser SecuritySecurityStateChanged event")
			}
			return false
		},
		// func(e *proto.SecurityHandleCertificateError) {
		// 	log.Warn().Interface("issue", e).Str("url", url).Msg("Received a new browser SecurityHandleCertificateError")
		// },
		func(e *proto.DOMStorageDomStorageItemAdded) {
			log.Warn().Interface("dom_storage_item_added", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemAdded event")
		},
		func(e *proto.DOMStorageDomStorageItemRemoved) {
			log.Warn().Interface("dom_storage_item_removed", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemRemoved event")
		},
		func(e *proto.DOMStorageDomStorageItemsCleared) {
			log.Warn().Interface("dom_storage_items_cleared", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemsCleared event")
		},
		func(e *proto.DOMStorageDomStorageItemUpdated) {
			log.Warn().Interface("dom_storage_item_updated", e).Str("url", url).Msg("Received a new DOMStorageDomStorageItemUpdated event")
		},
		func(e *proto.SecurityCertificateError) bool {
			// If IgnoreCertificateErrors are permanently added, this can be deleted
			log.Warn().Interface("issue", e).Str("url", url).Msg("Received a new browser SecurityCertificateError")

			err := proto.SecurityHandleCertificateError{
				EventID: e.EventID,
				Action:  proto.SecurityCertificateErrorActionContinue,
			}.Call(page)
			if err != nil {
				log.Error().Err(err).Msg("Could not handle security certificate error")
			} else {
				log.Debug().Msg("Handled security certificate error")
			}

			// certificate, err := proto.NetworkGetCertificate{}.Call(page)
			// if err != nil {
			// 	log.Warn().Str("url", url).Msg("Error getting certificate data")
			// } else {
			// 	log.Info().Msgf("Certificate data gathered: %s", certificate)
			// }
			return true

		},
	// func(e *proto.NetworkAuthChallenge) {
	// Should probably listen to: proto.FetchAuthRequired
	// 	log.Warn().Str("source", string(e.Source)).Str("origin", e.Origin).Str("realm", e.Realm).Str("scheme", e.Scheme).Msg("Network auth challange received")
	// }
	)()

	// Requeesting page
	var e proto.NetworkResponseReceived
	// https://github.com/go-rod/rod/issues/213
	wait := page.WaitEvent(&e)
	navigateError := page.Navigate(url)
	if navigateError != nil {
		log.Error().Err(navigateError).Str("url", url).Msg("Error navigating to page")

		// page.MustNavigate(url)
	}

	wait()
	err := page.WaitLoad()

	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("Error waiting for page complete load")
	} else {
		log.Debug().Str("url", url).Msg("Page fully loaded on browser and ready to be analyzed")
	}

	// https://chromedevtools.github.io/devtools-protocol/tot/Runtime/#method-globalLexicalScopeNames
	globalScopeNames, err := proto.RuntimeGlobalLexicalScopeNames{}.Call(page)
	if err != nil {
		log.Info().Err(err).Msg("Could not get global scope names")
	}
	log.Info().Interface("names", globalScopeNames).Msg("Global scope names")

	//page.MustWaitLoad().MustScreenshot("a.png")
	// Print request headers
	// utils.Dump(e.Response.RequestHeaders)

	// Print response info. The page redirect from google.com to www.gogole.com
	// utils.Dump(e.Response.Status, e.Response.URL, e.Response.Headers)
	data := GetPageData(page, url)
	data.StatusCode = e.Response.Status
	data.ResponseURL = e.Response.URL
	data.RemoteIPAddress = e.Response.RemoteIPAddress
	data.Port = *e.Response.RemotePort
	data.MimeType = e.Response.MIMEType
	data.SecurityState = string(e.Response.SecurityState)
	// https://pkg.go.dev/github.com/go-rod/rod@v0.91.1/lib/proto#SecuritySecurityState
	if e.Response.SecurityState != "secure" {
		data.SecurityDetails = e.Response.SecurityDetails
	}
	data.Cookies = cookies.GetPageCookies(page)
	for _, c := range data.Cookies {
		log.Info().Interface("cookie", c).Str("url", url).Msg("Page cookie")
	}
	// log.Info().Str("url", url).Interface("data", data).Msgf("Web page data gathered")
	// Maybe should not defer

	// Get audit results
	// Enabling audits, etc
	// domAudit := web.PageDOMAudit{
	// 	URL:      url,
	// 	Page:     page,
	// 	MaxDepth: 20,
	// }
	// domAudit.Run()

	// Evaluate DOM
	browser.MustClose()

	return data
}

// GetPageData : Given a loaded rod page, parses its data
func GetPageData(p *rod.Page, url string) WebPage {
	data := WebPage{
		URL: url,
	}
	anchors, err := GetPageAnchors(p)
	if err != nil {
		log.Error().Msg("Could not get page anchors")
	} else {
		data.Anchors = anchors
	}

	forms, err := GetForms(p)
	if err != nil {
		log.Error().Msg("Could not get page forms")
	} else {
		data.Forms = forms
	}
	// iframes := GetIframes(p)
	// data.iframes = iframes
	//GetButtons(page)
	// GetInputs(p)

	// GetPageResources(page)
	// File upload - https://go-rod.github.io/#/input?id=set-files
	// log.Info().Interface("data", data).Msg("Web page basic data gathered")

	return data
}

// IgnoreCertificateErrors tells the browser to ignore certificate errors
func IgnoreCertificateErrors(p *rod.Page) {
	ignoreCertsError := proto.SecuritySetIgnoreCertificateErrors{Ignore: true}.Call(p)
	if ignoreCertsError != nil {
		log.Error().Err(ignoreCertsError).Msg("Could not handle SecuritySetIgnoreCertificateErrors")
	} else {
		log.Debug().Msg("Handled SecuritySetIgnoreCertificateErrors")
	}
}

// GetPageAnchors find anchors on the given page
func GetPageAnchors(p *rod.Page) (anchors []string, err error) {
	anchors = []string{}
	resp := p.MustEval(GetLinks)
	for _, link := range resp.Arr() {
		anchors = append(anchors, link.String())
	}
	log.Info().Strs("anchors", anchors).Int("count", len(anchors)).Msg("Page anchors gathered")
	return anchors, nil
}

// GetButtons Given a page, returns its forms
func GetButtons(p *rod.Page) {
	buttons, err := p.Elements("button")
	if err != nil {
		return
	}
	for _, btn := range buttons {
		data := btn.MustHTML()
		fmt.Println(data)
		btn.MustClick()
	}
	log.Info().Int("count", len(buttons)).Msg("Page buttons gathered")
}

// GetForms : Given a page, returns its forms
func GetForms(p *rod.Page) (forms []Form, err error) {
	//fmt.Println("Forms:")
	forms = []Form{}
	formElements, err := p.Elements("form")
	if err != nil {
		return
	}
	for _, form := range formElements {
		// data := form.MustDescribe()
		formHTML := form.MustHTML()
		formData := Form{
			html: formHTML,
		}
		forms = append(forms, formData)
		AutoFillForm(form)
		submit, err := form.Element("[type=submit]")
		if err != nil {
			log.Info().Interface("form", form).Msg("Could not find submit button")
		} else {
			log.Info().Interface("submit", submit).Msg("Submit button found, clicking it")
			submit.MustClick()
		}
	}
	log.Info().Int("count", len(forms)).Msg("Page forms gathered")
	return forms, err
}


// GetIframes : Given a page, returns its iframes
func GetIframes(p *rod.Page) (iframes []Iframe) {
	iframes = []Iframe{}
	iframeElements, err := p.Elements("iframe")
	if err != nil {
		return
	}
	for _, iframeElement := range iframeElements {
		// data := form.MustDescribe()
		log.Debug().Str("iframe", iframeElement.String()).Msg("Processing iframe")
		//iframe := iframeElement.MustFrame()
		iframeData := Iframe{}
		// iframeHTML := iframe.MustHTML()
		iframeResource, err := iframeElement.Resource()
		if err != nil {
			log.Info().Msg("Could not get iframe resource")
		} else {
			log.Warn().Msgf("Iframe resource: %s", iframeResource)
			iframeData.src = string(iframeResource)
			iframes = append(iframes, iframeData)
		}
		log.Debug().Str("iframe", iframeElement.String()).Msg("Iframe processed")

	}
	log.Info().Int("count", len(iframes)).Msg("Page iframes gathered")
	return iframes
}

// GetInputs : Given a page, returns its inputs
func GetInputs(p *rod.Page) {
	inputs, err := p.Elements("input")
	if err != nil {
		return
	}
	for _, input := range inputs {
		data := input.MustHTML()
		fmt.Println(data)
	}
	log.Info().Int("count", len(inputs)).Msg("Page inputs gathered")
}

// DoesHTMLChange Not implemented  yet
func DoesHTMLChange(p *rod.Page) {

}

// GetPageResources gets all the page loaded resources, not used by now and probably can be removed as all requests are already hijacked
func GetPageResources(p *rod.Page) ([]byte, error) {
	response, err := proto.PageGetResourceTree{}.Call(p)
	if err != nil {
		return nil, err
	}
	fmt.Println(response.FrameTree)
	return nil, nil
}
