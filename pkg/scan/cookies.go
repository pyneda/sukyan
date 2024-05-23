package scan

func CommonSessionCookies() []string {
	return []string{
		"ASP.NET_SessionId",
		"ASPSESSIONID",
		"SITESERVER",
		"cfid",
		"cftoken",
		"jsessionid",
		"sessid",
		"sid",
		"viewstate",
		"zenid",
		"PHPSESSID",
		"JSESSIONID",
		"sessionid",
		"session_id",
	}
}

func CommonUsernames() []string {
	return []string{
		"root",
		"admin",
		"test",
		"guest",
		"info",
		"adm",
		"mysql",
		"user",
		"administrator",
		"oracle",
		"ftp",
		"manager",
		"operator",
		"supervisor",
		"debug",
	}
}
