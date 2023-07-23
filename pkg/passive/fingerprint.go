package passive

import (
	wappalyzer "github.com/projectdiscovery/wappalyzergo"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"strings"
)

type Fingerprint struct {
	Name    string
	Version string
}

func (f *Fingerprint) GetNucleiTags() string {
	splitName := strings.Split(f.Name, " ")
	firstWord := strings.ToLower(splitName[0])
	return firstWord
}

func FingerprintHistoryItems(items []*db.History) []Fingerprint {
	wappalyzerClient, _ := wappalyzer.New()

	allFingerprints := []string{}
	for _, item := range items {
		headers, _ := item.GetResponseHeadersAsMap()
		fingerprints := wappalyzerClient.Fingerprint(headers, []byte(item.ResponseBody))
		for key := range fingerprints {
			allFingerprints = append(allFingerprints, key)
		}
	}
	unique := lib.GetUniqueItems(allFingerprints)

	return parseFingerprints(unique)
}

func parseFingerprints(fpStrings []string) []Fingerprint {
	var fingerprints []Fingerprint
	for _, fpString := range fpStrings {
		splitFp := strings.Split(fpString, ":")
		fingerprint := Fingerprint{
			Name:    splitFp[0],
			Version: "",
		}
		if len(splitFp) > 1 {
			fingerprint.Version = splitFp[1]
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	return fingerprints
}

func GetUniqueNucleiTags(fingerprints []Fingerprint) []string {
	tags := []string{}
	for _, fingerprint := range fingerprints {
		tag := fingerprint.GetNucleiTags()
		tags = append(tags, tag)
	}
	uniqueTags := lib.GetUniqueItems(tags)

	return uniqueTags
}
