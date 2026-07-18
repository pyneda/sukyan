package web

import (
	"errors"
	"time"

	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
)

type InputNameValue struct {
	Name  string
	Value string
}

type InputTypeValue struct {
	Type  string
	Value string
}

var predefinedTypeValues = []InputTypeValue{
	{Type: "text", Value: "defaultText"},
	{Type: "password", Value: "password"},
	{Type: "email", Value: "test@example.com"},
	{Type: "number", Value: "12345"},
	{Type: "search", Value: "defaultSearch"},
	{Type: "tel", Value: "1234567890"},
	{Type: "url", Value: "http://www.example.com"},
	// {Type: "date", Value: "2023-06-16"},
	// {Type: "time", Value: "12:00"},
	// {Type: "datetime-local", Value: "2023-06-16T12:00"},
	// {Type: "month", Value: "2023-06"},
	{Type: "week", Value: "2023-W24"},
	{Type: "color", Value: "#ffffff"},
	{Type: "checkbox", Value: "true"}, // this could vary depending on implementation
	{Type: "radio", Value: "option1"}, // this could vary depending on implementation
	{Type: "range", Value: "50"},      // this could vary depending on implementation
	{Type: "hidden", Value: "defaultHidden"},
	// {Type: "file", Value: "/path/to/default/file"},
}

var timeInputs = []string{
	"date",
	"time",
	"datetime-local",
	"month",
}

var predefinedNameValues = []InputNameValue{
	{Name: "username", Value: "admin"},
	{Name: "password", Value: "password"},
	{Name: "email", Value: "test@example.com"},
	{Name: "firstName", Value: "John"},
	{Name: "lastName", Value: "Doe"},
	{Name: "phone", Value: "1234567890"},
	{Name: "address", Value: "123 Main St"},
	{Name: "city", Value: "Anytown"},
	{Name: "zip", Value: "12345"},
	{Name: "state", Value: "AnyState"},
	{Name: "country", Value: "AnyCountry"},
	{Name: "dateOfBirth", Value: "1990-01-01"},
	{Name: "gender", Value: "Other"},
	{Name: "maritalStatus", Value: "Single"},
	{Name: "nationality", Value: "AnyCountry"},
	{Name: "occupation", Value: "Unemployed"},
	{Name: "company", Value: "AnyCompany"},
	{Name: "jobTitle", Value: "None"},
	{Name: "education", Value: "Bachelor's Degree"},
	{Name: "website", Value: "http://www.example.com"},
	{Name: "bio", Value: "This is a default bio"},
	{Name: "securityQuestion", Value: "What is your mother's maiden name?"},
	{Name: "securityAnswer", Value: "DefaultAnswer"},
}

func SubmitForm(form *rod.Element, page *rod.Page) bool {
	// Prefer clicking the form's submit control: a real click fires the submit and
	// click event listeners that modern apps rely on (e.g. e.preventDefault() + fetch()).
	// Bare <button> elements default to type=submit but often lack the attribute, so
	// match both the explicit selector and any button inside the form.
	// Dispatch the click via JS rather than rod's Click(), which drives
	// Hover->ScrollIntoView->WaitStableRAF and hangs on a continuously-animating
	// button (WaitStableRAF's requestAnimationFrame loop runs on the deadline-less
	// root page). A JS-dispatched click still fires the button's click and the form's
	// submit listeners, and Element.Eval honors the element timeout.
	if submit, err := form.Element("[type=submit], button"); err == nil {
		log.Info().Interface("submit", submit).Msg("Submit control found, clicking it")
		if serr := SafeClick(submit); serr == nil {
			return true
		}
	}
	// requestSubmit() dispatches the submit event (running addEventListener('submit')
	// handlers and honoring preventDefault), unlike the native submit() which bypasses
	// them and would silently no-op JS-driven fetch/XHR forms. Fall back to submit()
	// only when requestSubmit is unavailable.
	_, serr := form.Timeout(2 * time.Second).Eval(`() => { if (this.requestSubmit) { this.requestSubmit(); } else { this.submit(); } }`)
	if serr == nil {
		log.Info().Interface("form", form).Msg("Form submitted using javascript")
		return true
	}
	log.Error().Err(serr).Msg("Could not submit form")
	return false
}

func AutoFillForm(form *rod.Element, page *rod.Page) {
	// Find all input elements within the form
	inputs, err := form.Elements("input")
	if err != nil {
		log.Debug().Msg("Could not find input elements")
	} else {
		for _, input := range inputs {
			AutoFillInput(input, page)
		}
	}

	textareas, err := form.Elements("textarea")
	if err != nil {
		log.Debug().Msg("Could not find textarea elements")
	} else {
		for _, textarea := range textareas {
			AutoFillTextarea(textarea, page)
		}
	}

}

func AutoFillInput(input *rod.Element, page *rod.Page) {
	// Get the name and type of the input element
	name, _ := input.Attribute("name")
	typeAttr, _ := input.Attribute("type")

	if typeAttr != nil && *typeAttr == "submit" {
		log.Debug().Msg("Form input auto filling skipped for input of type 'submit'")
		return
	}

	// Check if element is visible before proceeding
	visible, err := input.Visible()
	if err != nil || !visible {
		log.Debug().Msg("Form input auto filling skipped for invisible input")
		return
	}

	// Check if element is disabled
	disabled, err := input.Disabled()
	if err != nil || disabled {
		log.Debug().Msg("Form input auto filling skipped for disabled input")
		return
	}

	// Check if element is readonly
	readonly, _ := input.Property("readonly")
	if readonly.Bool() {
		log.Debug().Msg("Form input auto filling skipped for readonly input")
		return
	}

	// Quick interactability check with short timeout to avoid hanging
	_, interactErr := input.Timeout(500 * time.Millisecond).Interactable()
	if interactErr != nil {
		log.Debug().Err(interactErr).Msg("Form input auto filling skipped for non-interactable input")
		return
	}

	valuesByName := make(map[string]string)
	for _, v := range predefinedNameValues {
		valuesByName[v.Name] = v.Value
	}
	valuesByType := make(map[string]string)
	for _, v := range predefinedTypeValues {
		valuesByType[v.Type] = v.Value
	}

	// Try to get the value based on the input's name or, failing that, based on its type
	var value string
	var exists bool
	if name != nil {
		value, exists = valuesByName[*name]
	}
	if !exists && typeAttr != nil {
		value, exists = valuesByType[*typeAttr]
	}

	// If a predefined value was found, set the input value
	if exists {
		if err := setElementValue(input, value); err != nil {
			log.Info().Err(err).Msg("Failed to input value into form input field")
		} else {
			log.Info().Str("value", value).Msg("Form input auto-filled")
		}
	}
}

// SafeClick clicks an element by dispatching the click via JS instead of rod's
// Click(), which drives Hover->ScrollIntoView->WaitStableRAF and hangs on a
// continuously-animating element (its requestAnimationFrame loop runs on the
// deadline-less root page and never returns). A JS click still fires the element's
// click listeners, and Element.Eval honors the element timeout so this can't hang.
func SafeClick(el *rod.Element) error {
	// A JS click() on a disabled control is a silent no-op, so report it as an error
	// (mirroring rod's Click(), which fails on disabled elements). This lets callers
	// like SubmitForm fall through to an alternative rather than assume success.
	disabled, err := el.Timeout(2 * time.Second).Eval(`() => this.disabled === true`)
	if err != nil {
		return err
	}
	if disabled.Value.Bool() {
		return errDisabledElement
	}
	_, err = el.Timeout(2 * time.Second).Eval(`() => this.click()`)
	return err
}

var errDisabledElement = errors.New("element is disabled")

// setElementValue sets an input/textarea value via JS instead of rod's Input(),
// which drives Focus->ScrollIntoView->WaitStableRAF. WaitStableRAF's animation-frame
// loop runs on the deadline-less root page, so on a continuously-animating element it
// never returns and hangs the crawl. Setting .value directly and dispatching input/
// change events fills the field (and fires JS handlers) while honoring the element
// timeout, because Element.Eval runs under the element's own context.
func setElementValue(el *rod.Element, value string) error {
	// Use the element prototype's native value setter, not a direct `this.value = v`
	// instance assignment: frameworks like React override the instance value property
	// and track updates only through the native setter, so a plain assignment leaves
	// their controlled state (and onChange) untouched.
	_, err := el.Timeout(2*time.Second).Eval(
		`(v) => {
			const proto = Object.getPrototypeOf(this);
			const desc = Object.getOwnPropertyDescriptor(proto, 'value');
			if (desc && desc.set) { desc.set.call(this, v); } else { this.value = v; }
			this.dispatchEvent(new Event('input', {bubbles:true}));
			this.dispatchEvent(new Event('change', {bubbles:true}));
		}`,
		value,
	)
	return err
}

const defaultTextareaValue = "This is a default textarea input."

func AutoFillTextarea(textarea *rod.Element, page *rod.Page) {
	if textarea == nil {
		return
	}
	name, _ := textarea.Attribute("name")

	// Check if element is visible before proceeding
	visible, err := textarea.Visible()
	if err != nil || !visible {
		log.Debug().Msg("Form textarea auto filling skipped for invisible textarea")
		return
	}
	// Check if element is disabled
	disabled, err := textarea.Disabled()
	if err != nil || disabled {
		log.Debug().Msg("Form textarea auto filling skipped for disabled textarea")
		return
	}

	// Check if element is readonly
	readonly, _ := textarea.Property("readonly")
	if readonly.Bool() {
		log.Debug().Msg("Form textarea auto filling skipped for readonly textarea")
		return
	}

	// Quick interactability check with short timeout to avoid hanging
	_, interactErr := textarea.Timeout(500 * time.Millisecond).Interactable()
	if interactErr != nil {
		log.Debug().Err(interactErr).Msg("Form textarea auto filling skipped for non-interactable textarea")
		return
	}

	valuesByName := make(map[string]string)
	for _, v := range predefinedNameValues {
		valuesByName[v.Name] = v.Value
	}

	var value string
	var exists bool
	if name != nil {
		value, exists = valuesByName[*name]
	}

	if !exists {
		value = defaultTextareaValue
	}

	if err = setElementValue(textarea, value); err != nil {
		log.Error().Err(err).Msg("Failed to input value into textarea")
	} else {
		log.Info().Str("value", value).Msg("Textarea auto-filled")
	}
}
