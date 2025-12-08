package integrations

import (
	"crypto/sha1"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type RetireJsRepo map[string]RetireJsEntry

//go:embed jsrepository.json
var retireRepoContent []byte

func loadRetireJsRepo() (RetireJsRepo, error) {
	var repo RetireJsRepo
	err := json.Unmarshal(retireRepoContent, &repo)
	return repo, err
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
	repo      RetireJsRepo
	taskJobID uint
}

var fixRepeats = regexp.MustCompile(`\{0,([0-9]{4,})\}`)
var fixRepeats2 = regexp.MustCompile(`\{1,([0-9]{4,})\}`)

func fixPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "§§version§§", `[0-9]+(?:\.[0-9]+)*(?:[-_][a-zA-Z0-9.]+)*(?:\+[a-zA-Z0-9.-]+)?`)

	pattern = fixRepeats.ReplaceAllString(pattern, "{0,1000}")
	pattern = fixRepeats2.ReplaceAllString(pattern, "{1,1000}")

	return pattern
}

func extractVersionFromMatch(pattern, content string) string {
	regexPattern := strings.ReplaceAll(pattern, "§§version§§", `([0-9]+(?:\.[0-9]+)*(?:[-_][a-zA-Z0-9.]+)*(?:\+[a-zA-Z0-9.-]+)?)`)

	regexPattern = fixRepeats.ReplaceAllString(regexPattern, "{0,1000}")
	regexPattern = fixRepeats2.ReplaceAllString(regexPattern, "{1,1000}")

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		log.Debug().Err(err).Str("pattern", regexPattern).Msg("Failed to compile regex for version extraction")
		return ""
	}

	matches := regex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1] // Return the first capture group (the version)
	}

	return ""
}

func isValidVulnerability(vuln Vulnerability) bool {
	// Skip vulnerabilities without proper version constraints
	if vuln.Below == "" && vuln.AtOrAbove == "" {
		return false
	}

	// Validate version format
	if vuln.Below != "" {
		if _, err := semver.NewVersion(vuln.Below); err != nil {
			log.Debug().Str("version", vuln.Below).Msg("Invalid 'below' version format")
			return false
		}
	}

	if vuln.AtOrAbove != "" {
		if _, err := semver.NewVersion(vuln.AtOrAbove); err != nil {
			log.Debug().Str("version", vuln.AtOrAbove).Msg("Invalid 'atOrAbove' version format")
			return false
		}
	}

	return true
}

func isVersionVulnerable(version string, vuln Vulnerability) bool {
	if !isValidVulnerability(vuln) {
		return false
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		log.Debug().Str("version", version).Err(err).Msg("Failed to parse version")
		return false
	}

	// Check if version is below the fixed version
	if vuln.Below != "" {
		belowVersion, err := semver.NewVersion(vuln.Below)
		if err != nil {
			return false
		}

		if !v.LessThan(belowVersion) {
			return false
		}
	}

	// Check if version is at or above the vulnerable range start
	if vuln.AtOrAbove != "" {
		atOrAboveVersion, err := semver.NewVersion(vuln.AtOrAbove)
		if err != nil {
			return false
		}
		if v.LessThan(atOrAboveVersion) {
			return false
		}
	}

	return true
}

func (r *RetireScanner) HistoryScan(history *db.History) {
	var results = make(map[string][]Vulnerability)

	for library, entry := range r.repo {
		// Check filename patterns for version extraction
		for _, pattern := range entry.Extractors.Filename {
			version := extractVersionFromMatch(pattern, history.URL)
			if version != "" {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "filename").Msg("Extracted version from retirejs pattern")
				for _, vulnerability := range entry.Vulnerabilities {
					if isVersionVulnerable(version, vulnerability) {
						results[library] = append(results[library], vulnerability)
					}
				}
			}
		}

		// Check filecontent patterns for version extraction
		for _, pattern := range entry.Extractors.Filecontent {
			version := extractVersionFromMatch(pattern, string(history.RawResponse))
			if version != "" {
				log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "filecontent").Msg("Extracted version from retirejs pattern")
				for _, vulnerability := range entry.Vulnerabilities {
					if isVersionVulnerable(version, vulnerability) {
						results[library] = append(results[library], vulnerability)
					}
				}
			}
		}

		// Check hash-based detection
		h := sha1.New()
		body, err := history.ResponseBody()
		if err != nil {
			log.Error().Err(err).Str("url", history.URL).Msg("Failed to read response body")
			continue
		}
		h.Write(body)
		hash := fmt.Sprintf("%x", h.Sum(nil))
		if version, exists := entry.Extractors.Hashes[hash]; exists {
			for _, vulnerability := range entry.Vulnerabilities {
				if isVersionVulnerable(version, vulnerability) {
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

			// Try to get version info from the first vulnerability entry for display
			var detectedVersion string
			for libName, entry := range r.repo {
				if libName == library {
					// Try to extract version from filename patterns
					for _, pattern := range entry.Extractors.Filename {
						if version := extractVersionFromMatch(pattern, history.URL); version != "" {
							detectedVersion = version
							break
						}
					}
					// Try to extract version from filecontent patterns if not found in filename
					if detectedVersion == "" {
						for _, pattern := range entry.Extractors.Filecontent {
							if version := extractVersionFromMatch(pattern, string(history.RawResponse)); version != "" {
								detectedVersion = version
								break
							}
						}
					}
					// Try to get version from hash if still not found
					if detectedVersion == "" {
						h := sha1.New()
						if body, err := history.ResponseBody(); err == nil {
							h.Write(body)
							hash := fmt.Sprintf("%x", h.Sum(nil))
							if version, exists := entry.Extractors.Hashes[hash]; exists {
								detectedVersion = version
							}
						}
					}
					break
				}
			}

			if detectedVersion != "" {
				detailsBuilder.WriteString(fmt.Sprintf("The detected version %s of %s is affected by %d %s:\n\n", detectedVersion, library, len(vulnerabilities), name))
			} else {
				detailsBuilder.WriteString(fmt.Sprintf("The detected version of %s is affected by %d %s:\n\n", library, len(vulnerabilities), name))
			}

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

			issue := db.FillIssueFromHistoryAndTemplate(history, db.VulnerableJavascriptDependencyCode, detailsBuilder.String(), 90, "", history.WorkspaceID, history.TaskID, &r.taskJobID, history.ScanID, history.ScanJobID)
			issue.References = append(issue.References, lib.GetUniqueItems(references)...)
			issue.Requests = []db.History{*history}
			db.Connection().CreateIssue(*issue)
		}
	}

}
