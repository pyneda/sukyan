package http_utils

// HeadersModifier should be a generic interface to modify headers when using net/http and rod to make requests
type HeadersModifier struct {
	rules []ModifierRule
}

type ModifierRule struct {
	key   string
	value string
}
