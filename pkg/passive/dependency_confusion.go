package passive

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/dependency"
	"github.com/rs/zerolog/log"
)

// DependencyConfusionScan checks for potential dependency confusion vulnerabilities
// in npm package files (package.json, package-lock.json, yarn.lock)
func DependencyConfusionScan(item *db.History) {
	log.Debug().Str("url", item.URL).Msg("Scanning for dependency confusion vulnerabilities")

	parsedURL, err := url.Parse(item.URL)
	if err != nil {
		return
	}

	// Check if this is a dependency file we support
	fileType, ok := dependency.GetFileType(parsedURL.Path)
	if !ok {
		return
	}

	// Only check npm files in passive scan (pypi, rubygems, etc. handled by discovery module)
	registry := dependency.FileTypeToRegistry[fileType]
	if registry != dependency.RegistryNPM {
		return
	}

	body, err := item.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to get response body")
		return
	}

	bodyStr := string(body)

	// Extract packages using the shared dependency package
	packages := dependency.ExtractPackages(bodyStr, fileType)
	if len(packages) == 0 {
		return
	}

	// Check for missing packages in the npm registry
	missingPackages := dependency.CheckPackages(packages, nil)

	if len(missingPackages) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("During analysis of %s, the following npm packages were found to be missing from the public npm registry:\n\n", fileType))

		sb.WriteString("Missing Packages:\n")
		for _, pkg := range missingPackages {
			sb.WriteString(fmt.Sprintf("- Package Name: %s\n", pkg.Name))
			sb.WriteString(fmt.Sprintf("  Declared Version: %s\n", pkg.Version))
			sb.WriteString(fmt.Sprintf("  Found in: %s\n", pkg.Source))
			sb.WriteString("  Status: Not found in public npm registry\n\n")
		}

		sb.WriteString("Analysis Details:\n")
		sb.WriteString(fmt.Sprintf("- Total packages analyzed: %d\n", len(packages)))
		sb.WriteString(fmt.Sprintf("- Missing packages found: %d\n", len(missingPackages)))
		sb.WriteString("- Registry checked: npm public registry (registry.npmjs.org) and unpkg.com\n\n")

		confidence := 85

		db.CreateIssueFromHistoryAndTemplate(
			item,
			db.DependencyConfusionCode,
			sb.String(),
			confidence,
			"",
			item.WorkspaceID,
			item.TaskID,
			&defaultTaskJobID,
			item.ScanID,
			item.ScanJobID,
		)
	}
}
