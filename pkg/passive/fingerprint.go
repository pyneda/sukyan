package passive

import (
	"errors"
	"fmt"
	wappalyzer "github.com/projectdiscovery/wappalyzergo"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"sort"

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

func (f *Fingerprint) BuildCPE() (string, error) {
	if f.Version == "" {
		return "", errors.New("version not available")
	}
	name := strings.ToLower(strings.ReplaceAll(f.Name, " ", "_"))
	return fmt.Sprintf("cpe:/a:%s:%s:%s", name, name, f.Version), nil
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

func ReportFingerprints(baseURL string, fingerprints []Fingerprint, workspaceID, taskID uint) {
	details := getFingerprintsReport(fingerprints)

	issue := db.GetIssueTemplateByCode(db.TechStackFingerprintCode)
	issue.Details = details
	issue.Confidence = 100
	issue.WorkspaceID = &workspaceID
	issue.URL = baseURL
	issue.TaskID = &taskID

	created, err := db.Connection.CreateIssue(*issue)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create TechStackFingerprintCode issue")
		return
	}

	log.Info().Msgf("Successfully created issue: %v", created)
}

func getFingerprintsReport(fingerprints []Fingerprint) string {
	var reportBuilder strings.Builder
	reportBuilder.WriteString("Find below a list of technologies found in the target application during the crawl phase.\n\n")

	// Sort fingerprints by their Name for better readability
	sort.Slice(fingerprints, func(i, j int) bool {
		return fingerprints[i].Name < fingerprints[j].Name
	})

	// Append each fingerprint to the report
	for _, fp := range fingerprints {
		fpInfo := fmt.Sprintf("  * %s", fp.Name)
		if fp.Version != "" {
			fpInfo += fmt.Sprintf(" (Version: %s)", fp.Version)
		}
		fpInfo += "\n"
		reportBuilder.WriteString(fpInfo)
	}

	return reportBuilder.String()
}
