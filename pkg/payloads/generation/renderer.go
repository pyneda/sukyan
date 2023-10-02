package generation

import (
	"github.com/projectdiscovery/dsl/deserialization"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
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
	}
}

func (t *TemplateRenderer) genInteractionAddress() string {
	data := t.interactionsManager.GetURL()
	t.interactionDomain = data
	return data.URL
}
