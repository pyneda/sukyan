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
	Filename           []string          `json:"filename"`
	Filecontent        []string          `json:"filecontent"`
	FilecontentReplace []string          `json:"filecontentreplace"`
	Uri                []string          `json:"uri"`
	Hashes             map[string]string `json:"hashes"`
}

type Vulnerability struct {
	AtOrAbove   string      `json:"atOrAbove"`
	Below       string      `json:"below"`
	Excludes    []string    `json:"excludes"`
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
var pathSplitter = regexp.MustCompile(`[/\\]`)
var minSuffixPattern = regexp.MustCompile(`[.-]min$`)
var replacePatternRegex = regexp.MustCompile(`^/(.*[^\\])/([^/]+)/$`)

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
		return minSuffixPattern.ReplaceAllString(matches[1], "")
	}

	return ""
}

func extractVersionFromFilename(pattern, path string) string {
	segments := pathSplitter.Split(path, -1)
	if len(segments) == 0 {
		return ""
	}
	filename := segments[len(segments)-1]
	anchoredPattern := "^" + pattern + "$"
	return extractVersionFromMatch(anchoredPattern, filename)
}

func extractVersionFromReplace(pattern, content string) string {
	matches := replacePatternRegex.FindStringSubmatch(pattern)
	if len(matches) != 3 {
		return ""
	}
	searchPattern := matches[1]
	replacement := matches[2]

	regex, err := regexp.Compile(searchPattern)
	if err != nil {
		log.Debug().Err(err).Str("pattern", searchPattern).Msg("Failed to compile replacement regex")
		return ""
	}

	found := regex.FindString(content)
	if found == "" {
		return ""
	}

	version := regex.ReplaceAllString(found, replacement)
	return minSuffixPattern.ReplaceAllString(version, "")
}

func normalizeContent(content []byte) []byte {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
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

	for _, excluded := range vuln.Excludes {
		if version == excluded {
			return false
		}
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		log.Debug().Str("version", version).Err(err).Msg("Failed to parse version")
		return false
	}

	if vuln.Below != "" {
		belowVersion, err := semver.NewVersion(vuln.Below)
		if err != nil {
			return false
		}

		if !v.LessThan(belowVersion) {
			return false
		}
	}

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

type detection struct {
	library string
	version string
}

func (r *RetireScanner) HistoryScan(history *db.History) {
	body, err := history.ResponseBody()
	if err != nil {
		log.Error().Err(err).Str("url", history.URL).Msg("Failed to read response body")
		return
	}

	normalizedBody := normalizeContent(body)
	normalizedContent := string(normalizedBody)

	h := sha1.New()
	h.Write(normalizedBody)
	contentHash := fmt.Sprintf("%x", h.Sum(nil))

	detections := make(map[detection]bool)
	results := make(map[string]map[string][]Vulnerability)

	for library, entry := range r.repo {
		for _, pattern := range entry.Extractors.Filename {
			version := extractVersionFromFilename(pattern, history.URL)
			if version != "" {
				key := detection{library: library, version: version}
				if !detections[key] {
					detections[key] = true
					log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "filename").Msg("Extracted version from retirejs pattern")
					r.checkVulnerabilities(library, version, entry.Vulnerabilities, results)
				}
			}
		}

		for _, pattern := range entry.Extractors.Uri {
			version := extractVersionFromMatch(pattern, history.URL)
			if version != "" {
				key := detection{library: library, version: version}
				if !detections[key] {
					detections[key] = true
					log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "uri").Msg("Extracted version from retirejs pattern")
					r.checkVulnerabilities(library, version, entry.Vulnerabilities, results)
				}
			}
		}

		for _, pattern := range entry.Extractors.Filecontent {
			version := extractVersionFromMatch(pattern, normalizedContent)
			if version != "" {
				key := detection{library: library, version: version}
				if !detections[key] {
					detections[key] = true
					log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "filecontent").Msg("Extracted version from retirejs pattern")
					r.checkVulnerabilities(library, version, entry.Vulnerabilities, results)
				}
			}
		}

		for _, pattern := range entry.Extractors.FilecontentReplace {
			version := extractVersionFromReplace(pattern, normalizedContent)
			if version != "" {
				key := detection{library: library, version: version}
				if !detections[key] {
					detections[key] = true
					log.Debug().Str("url", history.URL).Str("pattern", pattern).Str("version", version).Str("type", "filecontentreplace").Msg("Extracted version from retirejs pattern")
					r.checkVulnerabilities(library, version, entry.Vulnerabilities, results)
				}
			}
		}

		if version, exists := entry.Extractors.Hashes[contentHash]; exists {
			key := detection{library: library, version: version}
			if !detections[key] {
				detections[key] = true
				log.Debug().Str("url", history.URL).Str("hash", contentHash).Str("version", version).Str("type", "hash").Msg("Matched retirejs hash")
				r.checkVulnerabilities(library, version, entry.Vulnerabilities, results)
			}
		}
	}

	r.reportFindings(history, results)
}

func (r *RetireScanner) checkVulnerabilities(library, version string, vulnerabilities []Vulnerability, results map[string]map[string][]Vulnerability) {
	for _, vuln := range vulnerabilities {
		if isVersionVulnerable(version, vuln) {
			if results[library] == nil {
				results[library] = make(map[string][]Vulnerability)
			}
			results[library][version] = append(results[library][version], vuln)
		}
	}
}

func (r *RetireScanner) reportFindings(history *db.History, results map[string]map[string][]Vulnerability) {
	for library, versionMap := range results {
		for version, vulnerabilities := range versionMap {
			if len(vulnerabilities) == 0 {
				continue
			}

			var detailsBuilder strings.Builder
			references := make([]string, 0)
			name := "vulnerabilities"
			if len(vulnerabilities) == 1 {
				name = "vulnerability"
			}

			detailsBuilder.WriteString(fmt.Sprintf("The detected version %s of %s is affected by %d %s:\n\n", version, library, len(vulnerabilities), name))

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
