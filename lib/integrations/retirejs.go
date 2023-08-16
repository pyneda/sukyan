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
	"strconv"
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

func (r *RetireScanner) HistoryScan(history *db.History) {
	var results = make(map[string][]Vulnerability)

	for library, entry := range r.repo {
		for _, pattern := range entry.Extractors.Filename {
			pattern = fixPattern(pattern)
			if match, err := regexp.MatchString(pattern, history.URL); err != nil {
				log.Error().Err(err).Str("pattern", pattern).Str("type", "filename").Msg("Failed to execute retirejs regex match")
			} else if match {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("type", "filename").Msg("Matched retirejs pattern")
				results[library] = append(results[library], entry.Vulnerabilities...)
			}
		}
		for _, pattern := range entry.Extractors.Filecontent {
			pattern = fixPattern(pattern)
			if match, err := regexp.MatchString(pattern, string(history.RawResponse)); err != nil {
				log.Error().Err(err).Str("pattern", pattern).Str("type", "filecontent").Msg("Failed to execute retirejs regex match")
			} else if match {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("type", "filecontent").Msg("Matched retirejs pattern")
				results[library] = append(results[library], entry.Vulnerabilities...)
			}
		}

		h := sha1.New()
		h.Write(history.ResponseBody)
		hash := fmt.Sprintf("%x", h.Sum(nil))
		if version, exists := entry.Extractors.Hashes[hash]; exists {
			for _, vulnerability := range entry.Vulnerabilities {
				if version >= vulnerability.AtOrAbove && version < vulnerability.Below {
					results[library] = append(results[library], vulnerability)

					log.Debug().Str("url", history.URL).Str("hash", hash).Str("version", version).Str("type", "hash").Msg("Matched retirejs pattern")
				}
			}
		}
	}
	for library, vulnerabilities := range results {
		if len(vulnerabilities) > 0 {
			var detailsBuilder strings.Builder
			references := make([]string, 0)
			name := "vulnerabilities"
			if len(vulnerabilities) == 1 {
				name = "vulnerability"
			}
			detailsBuilder.WriteString("The detected version of " + library + " is affected by " + strconv.Itoa(len(vulnerabilities)) + " " + name + ":\n\n")

			for _, vulnerability := range vulnerabilities {
				detailsBuilder.WriteString(vulnerability.Identifiers.Summary + "\n")

				if len(vulnerability.Identifiers.CVE) > 0 {
					detailsBuilder.WriteString(fmt.Sprintf("CVEs: %s\n", strings.Join(vulnerability.Identifiers.CVE, ", ")))
				}

				if vulnerability.AtOrAbove != "" {
					detailsBuilder.WriteString(fmt.Sprintf("At or Above: %s\n", vulnerability.AtOrAbove))
				}
				if vulnerability.Below != "" {
					detailsBuilder.WriteString(fmt.Sprintf("Below: %s\n", vulnerability.Below))
				}
				detailsBuilder.WriteString(fmt.Sprintf("Severity: %s\nCWEs: %s\nReferences:\n%s\n\n",
					lib.CapitalizeFirstLetter(vulnerability.Severity),
					strings.Join(vulnerability.Cwe, ", "),
					strings.Join(vulnerability.Info, "\n")),
				)

				if len(vulnerability.Info) > 0 {
					references = append(references, vulnerability.Info...)
				}
			}

			issue := db.FillIssueFromHistoryAndTemplate(history, db.VulnerableJavascriptDependencyCode, detailsBuilder.String(), 90, "", history.WorkspaceID)
			issue.References = append(issue.References, lib.GetUniqueItems(references)...)
			db.Connection.CreateIssue(*issue)
		}
	}

}
