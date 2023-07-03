package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func InteractWithPage(p *rod.Page) {
	if viper.GetBool("crawl.interaction.submit_forms") {
		GetAndSubmitForms(p)
	}
	if viper.GetBool("crawl.interaction.click_buttons") {
		GetAndClickButtons(p)
	}
}

// GetForms : Given a page, returns its forms
func GetAndSubmitForms(p *rod.Page) (err error) {
	formElements, err := p.Elements("form")
	if err != nil {
		return err
	}
	for _, form := range formElements {
		// p.Activate()
		AutoFillForm(form, p)
		SubmitForm(form, p)

	}
	return err
}

func GetAndClickButtons(p *rod.Page) {
	getAndClickElements("button", p)
	// getAndClickElements("input[type=submit]", p)
	// getAndClickElements("input[type=button]", p)
	// getAndClickElements("a", p)
	log.Debug().Msg("Finished clicking all elements")

}

func getAndClickElements(selector string, p *rod.Page) {
	var clickedButtons []string
	elements, err := p.Elements(selector)

	if err == nil {
		// p.Activate()

		for _, btn := range elements {
			xpath, err := btn.GetXPath(true)
			if err != nil {
				continue
			}

			if !lib.SliceContains(clickedButtons, xpath) {
				// p.HandleDialog()
				err = btn.Click(proto.InputMouseButtonLeft, 1)
				if err != nil {
					log.Error().Err(err).Str("xpath", xpath).Str("selector", selector).Msg("Error clicking element")
				} else {
					log.Info().Str("xpath", xpath).Str("selector", selector).Msg("Clicked button")
					clickedButtons = append(clickedButtons, xpath)

				}
			}
		}
	}
	log.Debug().Int("total", len(clickedButtons)).Str("selector", selector).Msg("Finished clicking elements")
}
