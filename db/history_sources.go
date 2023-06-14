package db

var SourceScanner = "Scanner"
var SourceProxy = "Proxy"
var SourceCrawler = "Crawler"
var SourceHijack = "Hijack"
var SourceRepeater = "Repeater"

var Sources = []string{
	SourceScanner,
	SourceProxy,
	SourceCrawler,
	SourceHijack,
	SourceRepeater,
}


func IsValidSource(source string) bool {
	for _, s := range Sources {
		if s == source {
			return true
		}
	}
	return false
}