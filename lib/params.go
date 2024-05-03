package lib

// var defaultDiscoveryParams[]string {"name", "q", "query", "search", "page", "type", "view", "callback", "url", "lang", "keyword", "keywords", "year", "email", "api_key", "api", "l"}

// ParameterAuditItem struct
type ParameterAuditItem struct {
	Parameter string
	Payload   string
	URL       string
	TestURL   string
	URLEncode bool
}

// ParameterValidValue struct
type ParameterValidValue struct {
	Type        string
	Value       string
	IsDynamic   bool
	IsReflected bool
}
