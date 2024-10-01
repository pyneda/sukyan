package scan

// GetCommonOpenRedirectParameters returns a list of common parameters known to be used in open redirect vulnerabilities
func GetCommonOpenRedirectParameters() []string {
	return []string{
		"next",
		"url",
		"target",
		"rurl",
		"dest",
		"destination",
		"redir",
		"redirect_uri",
		"redirect_url",
		"redirect",
		"redirect_to",
		"ReturnUrl",
		"return_to",
		"checkout_url",
		"continue",
		"return_path",
		"success",
		"data",
		"qurl",
		"login",
		"logout",
		"ext",
		"clickurl",
		"goto",
		"rit_url",
		"forward_url",
		"forward",
		"pic",
		"callback_url",
		"jump",
		"jump_url",
		"clicku",
		"originUrl",
		"origin",
		"Url",
		"desturl",
		"u",
		"page",
		"u1",
		"action",
		"action_url",
		"Redirect",
		"sp_url",
		"service",
		"recurl",
		"jurl",
		"uri",
		"allinurl",
		"q",
		"link",
		"src",
		"tcsrc",
		"linkAddress",
		"location",
		"burl",
		"request",
		"backurl",
		"RedirectUrl",
		"ReturnUrl",
		"returnUrl",
		"ret",
		"path",
		"ref",
		"callback",
		"referrer",
		"return",
		"out",
		"view",
		"redirector",
		"redir_uri",
		"redir_url",
		"continueUrl",
		"jumpTo",
		"redirectPath",
		"route",
		"redirectAfter",
	}
}

func IsCommonOpenRedirectParameter(param string) bool {
	for _, p := range GetCommonOpenRedirectParameters() {
		if p == param {
			return true
		}
	}
	return false
}