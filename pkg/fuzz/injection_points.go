package fuzz

import (
	"fmt"
	"net/url"
	"strings"
	"sukyan/db"
	"sukyan/lib"
	"sukyan/pkg/web"

	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
)

const DefaultInjectionPointReplaceValue = "FUZZ"

const (
	VectorParameterCode        = "parameter"
	VectorParameterPrependCode = "parameter-prepend"
	VectorParameterAppendCode  = "parameter-append"
	VectorParameterArrayCode   = "parameter-array"
	VectorFragment             = "fragment"
)

type InjectionPointGatherer struct {
	ParamsExtensive bool
	ReplaceValue    string
}

func (g *InjectionPointGatherer) checkConfig() {
	if strings.TrimSpace(g.ReplaceValue) == "" {
		g.ReplaceValue = DefaultInjectionPointReplaceValue
	}
}

func (g *InjectionPointGatherer) GetFromURL(path string) []URLInjectionPoint {
	g.checkConfig()
	var injectionPoints []URLInjectionPoint
	// parse URL
	parsedURL, err := url.Parse(path)
	if err != nil {
		log.Error().Str("url", path).Err(err).Msg("Could not parse url to get injection points")
	}
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Warn().Str("url", path).Msg("Could not parse url query")
	}
	// Build injection points for each parameter value
	for param, values := range query {
		for _, value := range values {
			// Build parameter replace injection point
			paramReplace := g.buildParamInjectionPoint(parsedURL, param, value, VectorParameterCode)
			injectionPoints = append(injectionPoints, paramReplace)
			if g.ParamsExtensive {
				// Build parameters prepend injection point
				paramPrepend := g.buildParamInjectionPoint(parsedURL, param, value, VectorParameterPrependCode)
				injectionPoints = append(injectionPoints, paramPrepend)
				// Build parameters append injection point
				paramAppend := g.buildParamInjectionPoint(parsedURL, param, value, VectorParameterAppendCode)
				injectionPoints = append(injectionPoints, paramAppend)
				// Build parameters array injection point
				paramArray := g.buildParamInjectionPoint(parsedURL, param, value, VectorParameterArrayCode)
				injectionPoints = append(injectionPoints, paramArray)
			}
		}
	}
	// hash(fragment)
	if parsedURL.Fragment != "" {
		fragmentURL := lib.CloneURL(parsedURL)
		fragmentURL.Fragment = g.ReplaceValue
		injectionPoints = append(injectionPoints, URLInjectionPoint{
			URL:           fragmentURL.String(),
			Code:          VectorFragment,
			Title:         "fragment replace",
			ReplaceValue:  g.ReplaceValue,
			OriginalValue: parsedURL.Fragment,
		})
	}
	// path

	return injectionPoints
}

func (g *InjectionPointGatherer) buildParamInjectionPoint(original *url.URL, param string, value string, code string) URLInjectionPoint {
	vector := lib.CloneURL(original)
	query := vector.Query()
	var title string
	switch code {
	case VectorParameterCode:
		title = fmt.Sprintf("parameter %s", param)
		query.Set(param, g.ReplaceValue)
	case VectorParameterAppendCode:
		title = fmt.Sprintf("parameter %s append", param)
		query.Set(param, value+g.ReplaceValue)
	case VectorParameterPrependCode:
		title = fmt.Sprintf("parameter %s prepend", param)
		query.Set(param, g.ReplaceValue+value)
	case VectorParameterArrayCode:
		arrayParam := param + "[0]"
		title = fmt.Sprintf("parameter %s array index", arrayParam)
		query.Add(arrayParam, g.ReplaceValue)
	}
	vector.RawQuery = query.Encode()
	unescaped, _ := url.QueryUnescape(vector.String())

	return URLInjectionPoint{
		URL:           unescaped,
		Code:          VectorParameterCode,
		Title:         title,
		ReplaceValue:  g.ReplaceValue,
		OriginalValue: value,
	}

}

func (g *InjectionPointGatherer) GetFromHistory(history *db.History) {

}

func (g *InjectionPointGatherer) GetFromBrowserPage(page *rod.Page) {

}

func (g *InjectionPointGatherer) GetFromWebPage(page *web.WebPage) {

}
