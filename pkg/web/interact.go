package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func InteractWithPage(p *rod.Page) {
	GetAndSubmitForms(p)
	// GetAndClickButtons(p)
}

// GetForms : Given a page, returns its forms
func GetAndSubmitForms(p *rod.Page) (err error) {
	formElements, err := p.Elements("form")
	if err != nil {
		return
	}
	for _, form := range formElements {
		p.Activate()
		AutoFillForm(form, p)
		p.Activate()
		SubmitForm(form, p)

	}
	return err
}

func GetAndClickButtons(p *rod.Page) {
	buttons, err := p.Elements("button")
	if err == nil {
		for _, btn := range buttons {
			p.Activate()
			btn.Click(proto.InputMouseButtonLeft, 1)
		}
	}
	buttons2, err := p.Elements(`[type="button"]`)
	if err == nil {
		for _, btn := range buttons2 {
			p.Activate()
			btn.Click(proto.InputMouseButtonLeft, 1)
		}
	}
}
