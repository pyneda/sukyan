package integrations

import (
	"crypto/sha1"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"regexp"
	"strings"
)

type RetireJsRepo map[string]RetireJsEntry

//go:embed jsrepository.json
var retireRepoContent []byte

func loadRetireJsRepo() (RetireJsRepo, error) {
	var repo RetireJsRepo
	// data, err := ioutil.ReadFile("jsrepository.json")
	// if err != nil {
	// 	return nil, err
	// }
	json.Unmarshal(retireRepoContent, &repo)
	return repo, nil
}

type RetireJsEntry struct {
	Extractors      Extractors      `json:"extractors"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

type Extractors struct {
	Filename    []string          `json:"filename"`
	Filecontent []string          `json:"filecontent"`
	Hashes      map[string]string `json:"hashes"`
}

type Vulnerability struct {
	AtOrAbove   string      `json:"atOrAbove"`
	Below       string      `json:"below"`
	Severity    string      `json:"severity"`
	Cwe         []string    `json:"cwe"`
	Identifiers Identifiers `json:"identifiers"`
	Info        []string    `json:"info"`
}

type Identifiers struct {
	Summary  string   `json:"summary"`
	CVE      []string `json:"CVE"`
	GithubID string   `json:"githubID"`
}

func NewRetireScanner() RetireScanner {
	repo, err := loadRetireJsRepo()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load retirejs repository")
	}
	return RetireScanner{
		repo: repo,
	}
}

type RetireScanner struct {
	repo RetireJsRepo
}

var fixRepeats = regexp.MustCompile("{0,[0-9]{4,}}")
var fixRepeats2 = regexp.MustCompile("{1,[0-9]{4,}}")

func fixPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "§§version§§", `[0-9][0-9.a-z_\\-]+`)
	pattern = fixRepeats.ReplaceAllString(pattern, "{0,1000}")
	pattern = fixRepeats2.ReplaceAllString(pattern, "{1,1000}")
	return pattern
}

func (r *RetireScanner) HistoryScan(history *db.History) ([]Vulnerability, error) {
	var vulnerabilities []Vulnerability

	for _, entry := range r.repo {
		for _, pattern := range entry.Extractors.Filename {
			pattern = fixPattern(pattern)
			if match, err := regexp.MatchString(pattern, history.URL); err != nil {
				log.Error().Err(err).Str("pattern", pattern).Str("type", "filename").Msg("Failed to execute retirejs regex match")
			} else if match {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("type", "filename").Msg("Matched retirejs pattern")
				vulnerabilities = append(vulnerabilities, entry.Vulnerabilities...)
			}
		}
		for _, pattern := range entry.Extractors.Filecontent {
			pattern = fixPattern(pattern)
			if match, err := regexp.MatchString(pattern, string(history.RawResponse)); err != nil {
				log.Error().Err(err).Str("pattern", pattern).Str("type", "filecontent").Msg("Failed to execute retirejs regex match")
			} else if match {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("type", "filecontent").Msg("Matched retirejs pattern")
				vulnerabilities = append(vulnerabilities, entry.Vulnerabilities...)
			}
		}

		h := sha1.New()
		h.Write(history.ResponseBody)
		hash := fmt.Sprintf("%x", h.Sum(nil))
		if version, exists := entry.Extractors.Hashes[hash]; exists {
			for _, vulnerability := range entry.Vulnerabilities {
				if version >= vulnerability.AtOrAbove && version < vulnerability.Below {
					vulnerabilities = append(vulnerabilities, vulnerability)
					log.Debug().Str("url", history.URL).Str("hash", hash).Str("version", version).Str("type", "hash").Msg("Matched retirejs pattern")
				}
			}
		}
	}

	if len(vulnerabilities) > 0 {
		var detailsBuilder strings.Builder
		references := make([]string, 0)

		for _, vulnerability := range vulnerabilities {
			detailsBuilder.WriteString(fmt.Sprintf("Summary: %s\nAt or Above: %s\nBelow: %s\nCWE: %s\nSeverity: %s\nInfo: %s\n\n",
				vulnerability.Identifiers.Summary,
				vulnerability.AtOrAbove,
				vulnerability.Below,
				vulnerability.Cwe,
				lib.CapitalizeFirstLetter(vulnerability.Severity),
				strings.Join(vulnerability.Info, ", ")))

			if len(vulnerability.Info) > 0 {
				references = append(references, vulnerability.Info...)
			}
		}

		issue := db.FillIssueFromHistoryAndTemplate(history, db.VulnerableJavascriptDependencyCode, detailsBuilder.String(), 90, "")
		issue.References = append(issue.References, lib.GetUniqueItems(references)...)
		db.Connection.CreateIssue(*issue)
		log.Warn().Str("issue", issue.Title).Str("url", history.URL).Str("details", issue.Details).Msg("New issue found")
	}

	return vulnerabilities, nil
}
