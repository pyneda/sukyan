package db

var SourceScanner = "Scanner"
var SourceProxy = "Proxy"
var SourceCrawler = "Crawler"
var SourceHijack = "Hijack"
var SourceRepeater = "Repeater"
var SourceBrowser = "Browser"
var SourceFuzzer = "Fuzzer"

var Sources = []string{
	SourceScanner,
	SourceProxy,
	SourceCrawler,
	SourceHijack,
	SourceRepeater,
	SourceBrowser,
	SourceFuzzer,
}

func IsValidSource(source string) bool {
	for _, s := range Sources {
		if s == source {
			return true
		}
	}
	return false
}
