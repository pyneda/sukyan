package passive

import "github.com/pyneda/sukyan/db"

func getHeaderChecks() []HeaderCheck {
	// Here we could also load and create them from a file or database
	return []HeaderCheck{
		xPoweredByHeaderCheck,
		xAspNetVersionHeaderCheck,
		serverHeaderCheck,
		// contentTypeHeaderCheck,
		cacheControlHeaderCheck,
		strictTransportSecurityHeaderCheck,
		xFrameOptionsHeaderCheck,
		xXSSProtectionHeaderCheck,
		aspNetMvcHeaderCheck,
	}
}

var xPoweredByHeaderCheck = HeaderCheck{
	Headers:        []string{"X-Powered-By"},
	Matchers:       []HeaderCheckMatcher{headerMatchAny},
	MatchCondition: And,
	IssueCode:      db.XPoweredByHeaderCode,
}

var xAspNetVersionHeaderCheck = HeaderCheck{
	Headers:        []string{"X-AspNet-Version"},
	Matchers:       []HeaderCheckMatcher{headerMatchAny},
	MatchCondition: And,
	IssueCode:      db.XASPVersionHeaderCode,
}

var serverHeaderCheck = HeaderCheck{
	Headers:        []string{"Server"},
	Matchers:       []HeaderCheckMatcher{headerMatchAny},
	MatchCondition: And,
	IssueCode:      db.ServerHeaderCode,
}

// var contentTypeHeaderCheck = HeaderCheck{
// 	Headers: []string{"Content-Type"},
// 	Matchers: []HeaderCheckMatcher{
// 		{
// 			MatcherType: NotEquals,
// 			Value:       "text/html; charset=UTF-8", // replace with the expected content type
// 		},
// 	},
// 	MatchCondition: And,
// 	IssueCode: ContentTypeHeaderCode,
// }

var cacheControlHeaderCheck = HeaderCheck{
	Headers: []string{"Cache-Control"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: Contains,
			Value:       "no-store",
		},
		{
			MatcherType: Contains,
			Value:       "private",
		},
	},
	MatchCondition: Or,
	IssueCode:      db.CacheControlHeaderCode,
}

var strictTransportSecurityHeaderCheck = HeaderCheck{
	Headers: []string{"Strict-Transport-Security"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: Regex,
			Value:       "^max-age=\\d+; includeSubDomains; preload$", // Enforces HSTS with includeSubDomains and preload
		},
	},
	MatchCondition: And,
	IssueCode:      db.StrictTransportSecurityHeaderCode,
}

var xFrameOptionsHeaderCheck = HeaderCheck{
	Headers: []string{"X-Frame-Options"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: NotEquals,
			Value:       "DENY",
		},
		{
			MatcherType: NotEquals,
			Value:       "SAMEORIGIN",
		},
	},
	MatchCondition: And,
	IssueCode:      db.XFrameOptionsHeaderCode,
}

var xXSSProtectionHeaderCheck = HeaderCheck{
	Headers: []string{"X-XSS-Protection"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: NotEquals,
			Value:       "1; mode=block",
		},
	},
	MatchCondition: And,
	IssueCode:      db.XXSSProtectionHeaderCode,
}

var aspNetMvcHeaderCheck = HeaderCheck{
	Headers:        []string{"X-AspNetMvc-Version"},
	Matchers:       []HeaderCheckMatcher{headerMatchAny},
	MatchCondition: And,
	IssueCode:      db.AspNetMvcHeaderCode,
}
