package web

import (
	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
)

// This should be the new version of InspectURL

type URLInspector struct {
	PageLoader  PageLoader
	SubmitForms bool
}

func (i *URLInspector) Run() {
	browser, _, err := i.PageLoader.GetPage()
	if err != nil {
		log.Error().Err(err).Msg("URLInspector error loading browser page")
	}
	// should use browser events handler
	eventsHandler := BrowserEventsHandler{
		ListenBackgroundServiceEvents: true,
		ListenIndexedDBEvents:         true,
		ListenDOMStorageEvents:        true,
	}
	eventsHandler.RunOnBrowser(browser)
}

func (i *URLInspector) InspectForms() {

}

func (i *URLInspector) InspectForm(form *rod.Element) {
	formHTML := form.MustHTML()
	formData := Form{
		html: formHTML,
	}
	// should try to guess if its common functionality

	log.Info().Interface("form", formData).Msg("Inspecting form")
	// if i.SubmitForms == true {

	// }
}
