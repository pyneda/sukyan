package generation

import (
	"fmt"
	"github.com/projectdiscovery/dsl/deserialization"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"strconv"
	"text/template"
)

type TemplateRenderer struct {
	interactionsManager integrations.InteractionsManager
	interactionDomain   integrations.InteractionDomain
}

func (t *TemplateRenderer) getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"base64encode":          lib.Base64Encode,
		"base64decode":          lib.Base64Decode,
		"interactionAddress":    t.genInteractionAddress,
		"randomInt":             lib.GenerateRandInt,
		"randomString":          lib.GenerateRandomString,
		"randomLowercaseString": lib.GenerateRandomLowercaseString,
		"escapeDots":            lib.EscapeDots,
		"generateJavaGadget":    deserialization.GenerateJavaGadget,
		"multiply":              multiply,
		"sum":                   sum,
		"divide":                divide,
		"subtract":              subtract,
	}
}

func (t *TemplateRenderer) genInteractionAddress() string {
	data := t.interactionsManager.GetURL()
	t.interactionDomain = data
	return data.URL
}

func toFloat64(i interface{}) (float64, error) {
	switch v := i.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("Unsupported type for conversion: %T", i)
	}
}

func multiply(a, b interface{}) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af * bf, nil
}

func sum(a, b interface{}) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af + bf, nil
}

func subtract(a, b interface{}) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af - bf, nil
}

func divide(a, b interface{}) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	if bf == 0 {
		return 0, fmt.Errorf("Division by zero")
	}
	return af / bf, nil
}
