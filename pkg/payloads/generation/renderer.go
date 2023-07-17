package generation

import (
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
		"base64encode":           lib.Base64Encode,
		"base64decode":           lib.Base64Decode,
		"genInteractionAddress":  t.genInteractionAddress,
		"genRandInt":             lib.GenerateRandInt,
		"genRandString":          lib.GenerateRandomString,
		"genrandLowercaseString": lib.GenerateRandomLowercaseString,
		"escapeDots":             lib.EscapeDots,
	}
}

func (t *TemplateRenderer) genInteractionAddress() string {
	data := t.interactionsManager.GetURL()
	t.interactionDomain = data
	return data.URL
}
