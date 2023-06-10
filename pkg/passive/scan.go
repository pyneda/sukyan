package passive

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"

	wappalyzer "github.com/projectdiscovery/wappalyzergo"
)



func ScanHistoryItem(item db.History) {
	headers, _ := item.GetResponseHeadersAsMap()
	wappalyzerClient, _ := wappalyzer.New()
	fingerprints := wappalyzerClient.Fingerprint(headers, []byte(item.ResponseBody))
	log.Info().Interface("fingerprints", fingerprints).Msg("Fingerprints found")
}

