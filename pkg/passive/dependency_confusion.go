package passive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

var (
	// Regular expressions for detecting package.json and package-lock.json files
	// These patterns ensure we match files that end with the exact names
	packageJsonRegex = regexp.MustCompile(`(?i)package\.json$`)
	packageLockRegex = regexp.MustCompile(`(?i)package-lock\.json$`)
	yarnLockRegex    = regexp.MustCompile(`(?i)yarn\.lock$`)

	// Regular expressions for extracting package names and versions from package.json
	packageDependencyRegex = regexp.MustCompile(`"([^"@][^"]+)"\s*:\s*"([^"]+)"`)
	scopedPackageRegex     = regexp.MustCompile(`"(@[^"]+/[^"]+)"\s*:\s*"([^"]+)"`)
)

// PackageInfo represents information about an npm package
type PackageInfo struct {
	Name    string
	Version string
	Source  string // where it was found (package.json, package-lock.json, etc.)
}

// NpmRegistryResponse represents a simplified npm registry response
type NpmRegistryResponse struct {
	Name     string                 `json:"name"`
	Versions map[string]interface{} `json:"versions"`
	Error    string                 `json:"error,omitempty"`
}

// DependencyConfusionScan checks for potential dependency confusion vulnerabilities
func DependencyConfusionScan(item *db.History) {
	log.Debug().Str("url", item.URL).Msg("Scanning for dependency confusion vulnerabilities")

	parsedURL, err := url.Parse(item.URL)
	if err != nil {
		return
	}

	// Check if this is a package.json, package-lock.json, or yarn.lock file
	isPackageFile := false
	fileType := ""

	if packageJsonRegex.MatchString(parsedURL.Path) {
		isPackageFile = true
		fileType = "package.json"
	} else if packageLockRegex.MatchString(parsedURL.Path) {
		isPackageFile = true
		fileType = "package-lock.json"
	} else if yarnLockRegex.MatchString(parsedURL.Path) {
		isPackageFile = true
		fileType = "yarn.lock"
	}

	if !isPackageFile {
		return
	}

	body, err := item.ResponseBody()
	if err != nil {
		log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to get response body")
		return
	}

	bodyStr := string(body)
	packages := extractPackageNames(bodyStr, fileType)

	if len(packages) == 0 {
		return
	}

	missingPackages := []PackageInfo{}

	for _, pkg := range packages {
		exists, _ := checkPackageInNpmRegistry(pkg.Name)
		if !exists {
			missingPackages = append(missingPackages, pkg)
		}
	}

	if len(missingPackages) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("During analysis of %s, the following npm packages were found to be missing from the public npm registry:\n\n", fileType))

		sb.WriteString("Missing Packages:\n")
		for _, pkg := range missingPackages {
			sb.WriteString(fmt.Sprintf("• Package Name: %s\n", pkg.Name))
			sb.WriteString(fmt.Sprintf("  Declared Version: %s\n", pkg.Version))
			sb.WriteString(fmt.Sprintf("  Found in: %s\n", pkg.Source))
			sb.WriteString("  Status: Not found in public npm registry\n\n")
		}

		sb.WriteString("Analysis Details:\n")
		sb.WriteString(fmt.Sprintf("• Total packages analyzed: %d\n", len(packages)))
		sb.WriteString(fmt.Sprintf("• Missing packages found: %d\n", len(missingPackages)))
		sb.WriteString("• Registry checked: npm public registry (registry.npmjs.org) and unpkg.com\n\n")

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
		)
	}
}

// extractPackageNames extracts package names and versions from the file content
func extractPackageNames(content, fileType string) []PackageInfo {
	var packages []PackageInfo

	switch fileType {
	case "package.json":
		packages = extractFromPackageJson(content)
	case "package-lock.json":
		packages = extractFromPackageLock(content)
	case "yarn.lock":
		packages = extractFromYarnLock(content)
	}

	return packages
}

// extractFromPackageJson extracts packages from package.json content
func extractFromPackageJson(content string) []PackageInfo {
	var packages []PackageInfo
	var packageData map[string]interface{}

	if err := json.Unmarshal([]byte(content), &packageData); err != nil {
		// Fallback to regex if JSON parsing fails
		return extractPackagesWithRegex(content, "package.json")
	}

	// Extract dependencies
	if deps, ok := packageData["dependencies"].(map[string]interface{}); ok {
		for name, version := range deps {
			if versionStr, ok := version.(string); ok {
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "package.json (dependencies)",
				})
			}
		}
	}

	// Extract devDependencies
	if devDeps, ok := packageData["devDependencies"].(map[string]interface{}); ok {
		for name, version := range devDeps {
			if versionStr, ok := version.(string); ok {
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "package.json (devDependencies)",
				})
			}
		}
	}

	return packages
}

// extractFromPackageLock extracts packages from package-lock.json content
func extractFromPackageLock(content string) []PackageInfo {
	var packages []PackageInfo
	var lockData map[string]interface{}

	if err := json.Unmarshal([]byte(content), &lockData); err != nil {
		return extractPackagesWithRegex(content, "package-lock.json")
	}

	// Extract from dependencies
	if deps, ok := lockData["dependencies"].(map[string]interface{}); ok {
		for name, depInfo := range deps {
			if depMap, ok := depInfo.(map[string]interface{}); ok {
				if version, ok := depMap["version"].(string); ok {
					packages = append(packages, PackageInfo{
						Name:    name,
						Version: version,
						Source:  "package-lock.json",
					})
				}
			}
		}
	}

	return packages
}

// extractFromYarnLock extracts packages from yarn.lock content (simplified parsing)
func extractFromYarnLock(content string) []PackageInfo {
	var packages []PackageInfo

	// Simple regex-based extraction for yarn.lock format
	// Format: "package-name@version":
	yarnPackageRegex := regexp.MustCompile(`^"?([^@"\s]+)@([^":\s]+)"?:`)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := yarnPackageRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			packages = append(packages, PackageInfo{
				Name:    matches[1],
				Version: matches[2],
				Source:  "yarn.lock",
			})
		}
	}

	return packages
}

// extractPackagesWithRegex is a fallback method using regex when JSON parsing fails
func extractPackagesWithRegex(content, source string) []PackageInfo {
	var packages []PackageInfo

	// Find regular packages
	matches := packageDependencyRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) == 3 {
			packages = append(packages, PackageInfo{
				Name:    match[1],
				Version: match[2],
				Source:  source + " (regex)",
			})
		}
	}

	// Find scoped packages
	scopedMatches := scopedPackageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range scopedMatches {
		if len(match) == 3 {
			packages = append(packages, PackageInfo{
				Name:    match[1],
				Version: match[2],
				Source:  source + " (regex)",
			})
		}
	}

	return packages
}

// checkPackageInNpmRegistry checks if a package exists in the public npm registry
// Uses multiple methods for better reliability: official registry and unpkg.com
func checkPackageInNpmRegistry(packageName string) (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// First try the official npm registry
	exists, version := checkOfficialNpmRegistry(client, packageName)
	if exists {
		return true, version
	}

	// Fallback to unpkg.com
	return checkUnpkgRegistry(client, packageName)
}

// checkOfficialNpmRegistry checks the official npm registry
func checkOfficialNpmRegistry(client *http.Client, packageName string) (bool, string) {
	registryURL := fmt.Sprintf("https://registry.npmjs.org/%s", packageName)

	resp, err := client.Get(registryURL)
	if err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to check official npm registry")
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, ""
	}

	if resp.StatusCode != 200 {
		log.Debug().Int("status", resp.StatusCode).Str("package", packageName).Msg("Unexpected status from official npm registry")
		return false, ""
	}

	var npmResp NpmRegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&npmResp); err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to decode npm registry response")
		return false, ""
	}

	if npmResp.Error != "" {
		return false, ""
	}

	// Get latest version
	latestVersion := "unknown"
	if len(npmResp.Versions) > 0 {
		latestVersion = fmt.Sprintf("%d versions available", len(npmResp.Versions))
	}

	return true, latestVersion
}

// checkUnpkgRegistry checks unpkg.com
func checkUnpkgRegistry(client *http.Client, packageName string) (bool, string) {
	unpkgURL := fmt.Sprintf("https://unpkg.com/%s", packageName)

	resp, err := client.Get(unpkgURL)
	if err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to check unpkg registry")
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, ""
	}

	if resp.StatusCode == 200 {
		return true, "available on unpkg"
	}

	log.Debug().Int("status", resp.StatusCode).Str("package", packageName).Msg("Unexpected status from unpkg registry")
	return false, ""
}
