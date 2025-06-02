package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	// "github.com/pyneda/sukyan/lib"
	"time"

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
	submit, err := form.Element("[type=submit]")
	// page.Timeout(200 * time.Millisecond).Activate()
	if err == nil {
		log.Info().Interface("submit", submit).Msg("Submit button found, clicking it")
		serr := submit.Timeout(2*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if serr == nil {
			return true
		}
	}
	_, serr := form.Timeout(2 * time.Second).Eval(`() => this.submit()`)
	if serr == nil {
		log.Info().Interface("form", form).Msg("Form submitted using javascript")
		return true
	} else {
		log.Error().Err(serr).Msg("Could not submit form")
	}
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
		err := input.Timeout(2 * time.Second).Input(value)
		if err != nil {
			log.Info().Err(err).Msg("Failed to input value into form input field")
		} else {
			log.Info().Str("value", value).Msg("Form input auto-filled")
		}
	}
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

	err = textarea.Timeout(2 * time.Second).Input(value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to input value into textarea")
	} else {
		log.Info().Str("value", value).Msg("Textarea auto-filled")
	}
}
