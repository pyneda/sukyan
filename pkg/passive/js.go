package passive

import "regexp"

// 1. Outdated libraries matching could be based on retirejs dataset.
// For usage implementation can see:
// - https://github.com/FallibleInc/retirejslib
// - https://github.com/stamparm/DSJS/blob/master/dsjs.py

// 2. Should also have some regex or ways to detect unsafe JS code such as eval(), .innerHTML() or usage of user controllable inputs.
// https://github.com/wisec/domxsswiki/wiki/Finding-DOMXSS

// Regular expression patterns
const (
	CommonJsSourcesPattern   = `/(location\s*[\[.])|([.\[]\s*["']?\s*(arguments|dialogArguments|innerHTML|write(ln)?|open(Dialog)?|showModalDialog|cookie|URL|documentURI|baseURI|referrer|name|opener|parent|top|content|self|frames)\W)|(localStorage|sessionStorage|Database)/`
	CommonJsSinksPattern     = `/((src|href|data|location|code|value|action)\s*["'\]]*\s*\+?\s*=)|((replace|assign|navigate|getResponseHeader|open(Dialog)?|showModalDialog|eval|evaluate|execCommand|execScript|setTimeout|setInterval)\s*["'\]]*\s*\()/`
	CommonJquerySinksPattern = `/after\(|\.append\(|\.before\(|\.html\(|\.prepend\(|\.replaceWith\(|\.wrap\(|\.wrapAll\(|\$\(|\.globalEval\(|\.add\(|jQuery\(|\$\(|\.parseHTML\(/`
)

// Compiled regular expressions
var (
	CommonJsSourcesRegex   = regexp.MustCompile(CommonJsSourcesPattern)
	CommonJsSinksRegex     = regexp.MustCompile(CommonJsSinksPattern)
	CommonJquerySinksRegex = regexp.MustCompile(CommonJquerySinksPattern)
)

func match(text string, regex *regexp.Regexp) []string {
	parsed := regex.FindAllString(text, -1)
	return parsed
}

// FindJsSources searches for common javascript sources
func FindJsSources(text string) []string {
	return match(text, CommonJsSourcesRegex)
}

// FindJsSinks searches for common javascript sinks
func FindJsSinks(text string) []string {
	return match(text, CommonJsSinksRegex)
}

// FindJquerySinks searches for common jquery sinks
func FindJquerySinks(text string) []string {
	return match(text, CommonJquerySinksRegex)
}
