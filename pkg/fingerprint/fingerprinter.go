package fingerprint

import "github.com/pyneda/sukyan/db"

type Fingerprinter struct {
	fingerprints []FingerprintInterface
}

// EvaluateURL checks if the provided url match any fingerprint
func (a *Fingerprinter) EvaluateURL(url string) {
}

// EvaluateText checks if the provided text match any fingerprint
func (a *Fingerprinter) EvaluateText(text string) {
}

// EvaluateCookies checks if the provided cookies any fingerprint
func (a *Fingerprinter) EvaluateCookies(cookies string) {

}

// EvaluateResponseHeaders checks if the provided response headers match any fingerprint
func (a *Fingerprinter) EvaluateResponseHeaders(headers map[string]string) {

}

// EvaluateFaviconHash checks if the provided favicon hash match any known one
func (a *Fingerprinter) EvaluateFaviconHash(hash string) {

}

// EvaluateHistoryRecord checks for fingerprints in a history record which is stored in the database
func (a *Fingerprinter) EvaluateHistoryRecord(history db.History) {
	a.EvaluateURL(history.URL)
	a.EvaluateText(history.ResponseBody)
}
