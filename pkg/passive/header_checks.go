package passive

import "github.com/pyneda/sukyan/db"

func getHeaderChecks() []HeaderCheck {
	// Here we could also load and create them from a file or database
	return []HeaderCheck{
		xPoweredByHeaderCheck,
		xAspNetVersionHeaderCheck,
		serverHeaderCheck,
		// contentTypeHeaderCheck,
		missingContentTypeHeaderCheck,
		cacheControlHeaderCheck,
		strictTransportSecurityHeaderCheck,
		xFrameOptionsHeaderCheck,
		xXSSProtectionHeaderCheck,
		aspNetMvcHeaderCheck,
		esiDetectionHeaderCheck,
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
	IssueCode:      db.XAspVersionHeaderCode,
}

var serverHeaderCheck = HeaderCheck{
	Headers: []string{"Server"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType:     Regex,
			Value:           "Jetty\\.([\\d\\.]+)",
			CustomIssueCode: db.JettyServerHeaderCode,
		},
		{
			MatcherType:     Regex,
			Value:           "java\\/([\\d\\.\\_]+)",
			CustomIssueCode: db.JavaServerHeaderCode,
		},
		headerMatchAny,
	},
	MatchCondition: Or,
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

var missingContentTypeHeaderCheck = HeaderCheck{
	Headers: []string{"Content-Type"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: NotExists,
		},
	},
	MatchCondition: And,
	IssueCode:      db.MissingContentTypeHeaderCode,
}

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
	IssueCode:      db.XXssProtectionHeaderCode,
}

var aspNetMvcHeaderCheck = HeaderCheck{
	Headers:        []string{"X-AspNetMvc-Version"},
	Matchers:       []HeaderCheckMatcher{headerMatchAny},
	MatchCondition: And,
	IssueCode:      db.AspNetMvcHeaderCode,
}

var esiDetectionHeaderCheck = HeaderCheck{
	Headers: []string{"Surrogate-Control"},
	Matchers: []HeaderCheckMatcher{
		{
			MatcherType: Equals,
			Value:       "content=\"ESI/1.0\"",
		},
	},
	MatchCondition: And,
	IssueCode:      db.EsiDetectedCode,
}
