package generation

import (
	"fmt"
	"github.com/projectdiscovery/dsl/deserialization"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"math"
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

// multiply returns an integer when both operands are integral. Arithmetic-oracle
// payloads inject `N*M` into a target that does integer arithmetic and then match
// the target's output as a string, but Go renders a float64 in scientific notation
// once it exceeds ~1e6 ("9980010" becomes "9.98001e+06"). That marker can never
// match what the target prints, so the whole detection silently stops working
// above that magnitude.
func multiply(a, b interface{}) (interface{}, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}

	product := af * bf
	if product == math.Trunc(product) && math.Abs(product) <= maxExactFloat64Int {
		return int64(product), nil
	}
	return product, nil
}

// Beyond 2^53 a float64 can no longer represent every integer, so converting to
// int64 would print a value the target never produced.
const maxExactFloat64Int = 1 << 53

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
