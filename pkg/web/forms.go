package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"time"
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

func SubmitForm(form *rod.Element) {
	submit, err := form.Element("[type=submit]")
	if err == nil {
		log.Info().Interface("submit", submit).Msg("Submit button found, clicking it")
		submit.Click(proto.InputMouseButtonRight, 1)
		return
	}
	_, serr := form.Eval(`() => this.submit()`)
	if serr == nil {
		log.Info().Interface("form", form).Msg("Form submitted using javascript")
		return
	} else {
		log.Error().Err(serr).Msg("Could not submit form")
	}

}

func AutoFillForm(form *rod.Element) {
	// Find all input elements within the form
	inputs, err := form.Elements("input")
	if err != nil {
		log.Error().Msg("Could not find input elements")
		return
	}

	// Iterate over each input element
	for _, input := range inputs {
		AutoFillInput(input)
	}
}

func AutoFillInput(input *rod.Element) {
	// Get the name and type of the input element
	name, _ := input.Attribute("name")
	typeAttr, _ := input.Attribute("type")

	// handle time inputs
	if lib.SliceContains(timeInputs, *typeAttr) {
		input.InputTime(time.Now().Add(24 * time.Hour))
		return
	}

	if *typeAttr == "checkbox" && !input.MustProperty("checked").Bool() {
		input.MustClick()
		return
	}

	// if typeAttr == "file" {
	// 	input.MustSetFiles("/path/to/default/file")
	// }

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
		input.MustInput(value)
	}
}
