package payloads

import "regexp"

// TemplateLanguageSyntax used to generate payloads for basic SSTI validation
type TemplateLanguageSyntax struct {
	Opener string
	Closer string
}

// TemplateLanguagePayload Holds a payload and a regex pattern to verify it
type TemplateLanguagePayload struct {
	Value string
	Regex string
}

// MatchAgainstString Checks if the payload match against a string
func (p TemplateLanguagePayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Regex, text)
}

// GetValue gets the payload value
func (p TemplateLanguagePayload) GetValue() string {
	return p.Value
}

// TemplateLanguage Holds different information about a template language, still not used
type TemplateLanguage struct {
	Name             string
	AcceptedSyntaxes []TemplateLanguageSyntax
	FingerprintTags  []TemplateLanguagePayload
	// ExploitTags     []TemplateLanguagePayload
}

// -----------------------------------------
// Common Syntaxes definition
// -----------------------------------------

// BracketsSyntax Used in Django, Jinja, Tornado,  ERB (Ruby)..
var BracketsSyntax = TemplateLanguageSyntax{
	Opener: "{{",
	Closer: "}}",
}

// DollarBracketsSyntax Used in Tornado, ERB (Ruby)..
var DollarBracketsSyntax = TemplateLanguageSyntax{
	Opener: "${{",
	Closer: "}}",
}

// DollarBracketSyntax Used in Handlebars, Thymeleaf(Java)...
var DollarBracketSyntax = TemplateLanguageSyntax{
	Opener: "${",
	Closer: "}",
}

// HashBracketSyntax Used in PugJS
var HashBracketSyntax = TemplateLanguageSyntax{
	Opener: "#{",
	Closer: "}",
}

// RazorSyntax Used in Razor (.NET)
var RazorSyntax = TemplateLanguageSyntax{
	Opener: "@(",
	Closer: ")",
}

// ERBSyntax Used in ERB (Ruby), Perl...
var ERBSyntax = TemplateLanguageSyntax{
	Opener: "<%=",
	Closer: "%>",
}

// GetTemplateLanguageSyntaxes Returns the available template language syntaxes
func GetTemplateLanguageSyntaxes() (results []TemplateLanguageSyntax) {
	results = append(results, BracketsSyntax)
	results = append(results, DollarBracketSyntax)
	results = append(results, DollarBracketsSyntax)
	results = append(results, ERBSyntax)
	results = append(results, HashBracketSyntax)
	results = append(results, HashBracketSyntax)
	return results
}

// GenerateSSTIPayloads generates payloads for different template language syntaxes
func GenerateSSTIPayloads() (payloads []TemplateLanguagePayload) {
	syntaxes := GetTemplateLanguageSyntaxes()
	for _, syntax := range syntaxes {
		payloads = append(payloads, TemplateLanguagePayload{
			Value: syntax.Opener + "8263+8263" + syntax.Closer,
			Regex: "16526",
		})
		payloads = append(payloads, TemplateLanguagePayload{
			Value: syntax.Opener + "839*839" + syntax.Closer,
			Regex: "703921",
		})
	}
	return payloads
}
