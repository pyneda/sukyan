package web

import (
	"fmt"
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
	ListenForPageEvents(url, page)

	// Requesting page
	var e proto.NetworkResponseReceived
	// https://github.com/go-rod/rod/issues/213
	wait := page.WaitEvent(&e)
	navigateError := page.Navigate(url)
	if navigateError != nil {
		log.Error().Err(navigateError).Str("url", url).Msg("Error navigating to page")
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

	// Evaluate DOM
	// domAudit := web.PageDOMAudit{
	// 	URL:      url,
	// 	Page:     page,
	// 	MaxDepth: 20,
	// }
	// domAudit.Run()

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
		SubmitForm(form)

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
