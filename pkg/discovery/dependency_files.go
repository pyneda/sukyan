package discovery

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/dependency"
	"github.com/rs/zerolog/log"
)

// DependencyFilePaths contains paths for dependency manifest files
var DependencyFilePaths = []string{
	"package.json",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"composer.json",
	"composer.lock",
	"Gemfile",
	"Gemfile.lock",
	"requirements.txt",
	"Pipfile",
	"Pipfile.lock",
	"pyproject.toml",
	"go.mod",
	"go.sum",
	"Cargo.toml",
	"Cargo.lock",
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
}

// fileTypeToIssueCode maps file types to their corresponding issue codes
var fileTypeToIssueCode = map[dependency.FileType]db.IssueCode{
	dependency.FileTypePackageJSON:     db.ExposedPackageJsonCode,
	dependency.FileTypePackageLock:     db.ExposedPackageJsonCode,
	dependency.FileTypeYarnLock:        db.ExposedPackageJsonCode,
	dependency.FileTypePnpmLock:        db.ExposedPackageJsonCode,
	dependency.FileTypeComposerJSON:    db.ExposedComposerJsonCode,
	dependency.FileTypeComposerLock:    db.ExposedComposerJsonCode,
	dependency.FileTypeGemfile:         db.ExposedGemfileCode,
	dependency.FileTypeGemfileLock:     db.ExposedGemfileCode,
	dependency.FileTypeRequirementsTxt: db.ExposedRequirementsTxtCode,
	dependency.FileTypePipfile:         db.ExposedRequirementsTxtCode,
	dependency.FileTypePipfileLock:     db.ExposedRequirementsTxtCode,
	dependency.FileTypePyProjectTOML:   db.ExposedRequirementsTxtCode,
	dependency.FileTypeGoMod:           db.ExposedGoModCode,
	dependency.FileTypeGoSum:           db.ExposedGoModCode,
	dependency.FileTypeCargoTOML:       db.ExposedCargoTomlCode,
	dependency.FileTypeCargoLock:       db.ExposedCargoTomlCode,
	dependency.FileTypePomXML:          db.ExposedPomXmlCode,
	dependency.FileTypeBuildGradle:     db.ExposedPomXmlCode,
	dependency.FileTypeBuildGradleKts:  db.ExposedPomXmlCode,
}

// DependencyFileValidationResult holds validation results including confusion check
type DependencyFileValidationResult struct {
	Valid             bool
	Details           string
	Confidence        int
	FileType          dependency.FileType
	Packages          []dependency.PackageInfo
	MissingPackages   []dependency.PackageInfo
	ConfusionDetected bool
}

// validateDependencyFile validates a dependency file and checks for dependency confusion
func validateDependencyFile(history *db.History, httpClient *http.Client) DependencyFileValidationResult {
	result := DependencyFileValidationResult{}

	if history.StatusCode != 200 {
		return result
	}

	// CRITICAL: Check if response is HTML - most common cause of false positives
	// Servers often return their homepage or a default HTML page for any path
	if isHTMLResponse(history) {
		return result
	}

	body, err := history.ResponseBody()
	if err != nil {
		return result
	}

	bodyStr := string(body)
	trimmedBody := strings.TrimSpace(bodyStr)

	// Determine file type
	fileType, ok := dependency.GetFileType(history.URL)
	if !ok {
		return result
	}
	result.FileType = fileType

	// Strict content validation for each file type
	switch fileType {
	case dependency.FileTypePackageJSON:
		// Must be valid JSON with package.json specific fields
		if !strings.HasPrefix(trimmedBody, "{") {
			return result
		}
		// Must contain at least one of these package.json fields
		if !strings.Contains(bodyStr, "\"name\"") &&
			!strings.Contains(bodyStr, "\"version\"") &&
			!strings.Contains(bodyStr, "\"dependencies\"") &&
			!strings.Contains(bodyStr, "\"devDependencies\"") {
			return result
		}

	case dependency.FileTypePackageLock:
		// Must be valid JSON with lock file fields
		if !strings.HasPrefix(trimmedBody, "{") {
			return result
		}
		if !validateLockFileFormat(bodyStr, true) {
			return result
		}

	case dependency.FileTypeYarnLock:
		// Yarn lock has specific format: "package@version": followed by metadata
		if !strings.Contains(bodyStr, "# yarn lockfile") &&
			!strings.Contains(bodyStr, "version:") &&
			!strings.Contains(bodyStr, "resolved:") {
			// Try alternate format detection
			if !(strings.Contains(bodyStr, "@") && strings.Contains(bodyStr, "integrity ")) {
				return result
			}
		}

	case dependency.FileTypePnpmLock:
		// PNPM lock is YAML format
		if !strings.Contains(bodyStr, "lockfileVersion:") &&
			!strings.Contains(bodyStr, "packages:") &&
			!strings.Contains(bodyStr, "dependencies:") {
			return result
		}

	case dependency.FileTypeComposerJSON:
		// Must be valid JSON with composer.json fields
		if !strings.HasPrefix(trimmedBody, "{") {
			return result
		}
		if !strings.Contains(bodyStr, "\"require\"") &&
			!strings.Contains(bodyStr, "\"autoload\"") &&
			!strings.Contains(bodyStr, "\"name\"") {
			return result
		}

	case dependency.FileTypeComposerLock:
		// Must be valid JSON with composer.lock fields
		if !strings.HasPrefix(trimmedBody, "{") {
			return result
		}
		if !strings.Contains(bodyStr, "\"packages\"") &&
			!strings.Contains(bodyStr, "\"content-hash\"") {
			return result
		}

	case dependency.FileTypeGemfile:
		// Gemfile is Ruby DSL - must have source and gem declarations
		// Must contain actual Ruby Gemfile syntax, not just the words
		hasSource := strings.Contains(bodyStr, "source ") || strings.Contains(bodyStr, "source(")
		hasGem := strings.Contains(bodyStr, "gem ") || strings.Contains(bodyStr, "gem(")
		// Check for quoted strings typical in Gemfile
		hasRubyQuotes := strings.Contains(bodyStr, "'rubygems") || strings.Contains(bodyStr, "\"rubygems")

		if !hasSource && !hasGem {
			return result
		}
		// Need additional confirmation it's a real Gemfile
		if !hasRubyQuotes && !strings.Contains(bodyStr, "group :") && !strings.Contains(bodyStr, "ruby ") {
			// Check if it has typical Gemfile structure with version specs
			if !strings.Contains(bodyStr, "~>") && !strings.Contains(bodyStr, ">=") {
				return result
			}
		}

	case dependency.FileTypeGemfileLock:
		// Gemfile.lock has very specific format
		if !strings.Contains(bodyStr, "GEM") &&
			!strings.Contains(bodyStr, "PLATFORMS") &&
			!strings.Contains(bodyStr, "DEPENDENCIES") &&
			!strings.Contains(bodyStr, "BUNDLED WITH") {
			return result
		}
		// Need at least two of these sections for confidence
		sections := 0
		for _, section := range []string{"GEM", "PLATFORMS", "DEPENDENCIES", "BUNDLED WITH"} {
			if strings.Contains(bodyStr, section) {
				sections++
			}
		}
		if sections < 2 {
			return result
		}

	case dependency.FileTypeRequirementsTxt:
		// Requirements.txt has specific format: package==version or package>=version
		lines := strings.Split(bodyStr, "\n")
		validPackageLines := 0
		// Pattern: packagename followed by optional version specifier
		packagePattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*(\[[^\]]+\])?\s*(==|>=|<=|~=|!=|>|<|@)`)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			// Check if line looks like a package spec
			if packagePattern.MatchString(line) || (len(line) > 0 && !strings.Contains(line, "<") && !strings.Contains(line, ">") && strings.Contains(line, "==")) {
				validPackageLines++
			}
		}
		// Need at least a few valid package lines
		if validPackageLines < 2 {
			return result
		}

	case dependency.FileTypePipfile:
		// Pipfile is TOML format with specific sections
		if !strings.Contains(bodyStr, "[packages]") &&
			!strings.Contains(bodyStr, "[dev-packages]") &&
			!strings.Contains(bodyStr, "[[source]]") {
			return result
		}

	case dependency.FileTypePipfileLock:
		// Pipfile.lock is JSON with specific structure
		if !strings.HasPrefix(trimmedBody, "{") {
			return result
		}
		if !strings.Contains(bodyStr, "\"_meta\"") &&
			!strings.Contains(bodyStr, "\"default\"") &&
			!strings.Contains(bodyStr, "\"develop\"") {
			return result
		}

	case dependency.FileTypePyProjectTOML:
		// pyproject.toml is TOML with specific sections
		if !validateTOMLContent(bodyStr) {
			return result
		}
		// Must have project or build-system sections
		if !strings.Contains(bodyStr, "[project]") &&
			!strings.Contains(bodyStr, "[build-system]") &&
			!strings.Contains(bodyStr, "[tool.") {
			return result
		}

	case dependency.FileTypeGoMod:
		// Must have module declaration at the start
		if !strings.HasPrefix(trimmedBody, "module ") {
			// Check if module appears in first few lines
			lines := strings.Split(bodyStr, "\n")
			foundModule := false
			for i, line := range lines {
				if i > 5 {
					break
				}
				if strings.HasPrefix(strings.TrimSpace(line), "module ") {
					foundModule = true
					break
				}
			}
			if !foundModule {
				return result
			}
		}
		// Should also have go version or require block
		if !strings.Contains(bodyStr, "go ") && !strings.Contains(bodyStr, "require ") && !strings.Contains(bodyStr, "require (") {
			return result
		}

	case dependency.FileTypeGoSum:
		// go.sum has very specific format: module version hash
		lines := strings.Split(bodyStr, "\n")
		validLines := 0
		// Pattern: module/path vX.X.X h1:hash or module/path vX.X.X/go.mod h1:hash
		goSumPattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._/-]+ v[0-9]+\.[0-9]+\.[0-9]+[^\s]* h1:[a-zA-Z0-9+/=]+$`)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if goSumPattern.MatchString(line) || (strings.Contains(line, " v") && strings.Contains(line, " h1:")) {
				validLines++
			}
		}
		// Need several valid go.sum lines
		if validLines < 3 {
			return result
		}

	case dependency.FileTypeCargoTOML:
		// Must be valid TOML with Cargo-specific sections
		if !validateTOMLContent(bodyStr) {
			return result
		}
		if !strings.Contains(bodyStr, "[package]") &&
			!strings.Contains(bodyStr, "[dependencies]") &&
			!strings.Contains(bodyStr, "[workspace]") {
			return result
		}

	case dependency.FileTypeCargoLock:
		// Cargo.lock is TOML with [[package]] sections
		if !strings.Contains(bodyStr, "[[package]]") {
			return result
		}
		// Should have name and version fields
		if !strings.Contains(bodyStr, "name = ") || !strings.Contains(bodyStr, "version = ") {
			return result
		}

	case dependency.FileTypePomXML:
		// Must be valid XML with Maven-specific elements
		if !strings.Contains(bodyStr, "<project") {
			return result
		}
		// Should have groupId, artifactId, or dependencies
		if !strings.Contains(bodyStr, "<groupId>") &&
			!strings.Contains(bodyStr, "<artifactId>") &&
			!strings.Contains(bodyStr, "<dependencies>") &&
			!strings.Contains(bodyStr, "<modelVersion>") {
			return result
		}

	case dependency.FileTypeBuildGradle, dependency.FileTypeBuildGradleKts:
		// Gradle files use specific DSL
		gradleIndicators := 0
		gradlePatterns := []string{
			"dependencies {", "dependencies{",
			"plugins {", "plugins{",
			"implementation ", "implementation(",
			"testImplementation",
			"compile ", "compile(",
			"repositories {",
			"buildscript {",
			"apply plugin:",
		}
		for _, pattern := range gradlePatterns {
			if strings.Contains(bodyStr, pattern) {
				gradleIndicators++
			}
		}
		if gradleIndicators < 2 {
			return result
		}
	}

	result.Valid = true
	result.Confidence = 60

	var details strings.Builder
	details.WriteString(fmt.Sprintf("Dependency manifest file detected: %s\n\n", history.URL))
	details.WriteString(fmt.Sprintf("File type: %s\n", fileType))

	// Extract packages
	packages := dependency.ExtractPackages(bodyStr, fileType)
	result.Packages = packages

	if len(packages) > 0 {
		result.Confidence += 20
		details.WriteString(fmt.Sprintf("Total packages found: %d\n\n", len(packages)))

		// Check for dependency confusion (only for npm, pypi, rubygems, packagist)
		registry := dependency.FileTypeToRegistry[fileType]
		if registry == dependency.RegistryNPM || registry == dependency.RegistryPyPI ||
			registry == dependency.RegistryRubyGems || registry == dependency.RegistryPackagist {

			missingPackages := dependency.CheckPackages(packages, httpClient)
			result.MissingPackages = missingPackages

			if len(missingPackages) > 0 {
				result.ConfusionDetected = true
				result.Confidence = 95

				details.WriteString("POTENTIAL DEPENDENCY CONFUSION DETECTED\n")
				details.WriteString("The following packages are NOT found in the public registry:\n\n")
				for _, pkg := range missingPackages {
					details.WriteString(fmt.Sprintf("- %s (version: %s, source: %s)\n", pkg.Name, pkg.Version, pkg.Source))
				}
				details.WriteString("\nThese may be private/internal packages vulnerable to dependency confusion attacks.\n")
			}
		}

		// Show all packages found
		details.WriteString("\nPackages detected:\n")
		for _, pkg := range packages {
			details.WriteString(fmt.Sprintf("- %s: %s\n", pkg.Name, pkg.Version))
		}
	}

	result.Details = details.String()
	return result
}

// IsDependencyFileValidationFunc validates dependency files
func IsDependencyFileValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	result := validateDependencyFile(history, nil)
	return result.Valid, result.Details, result.Confidence
}

// DiscoverDependencyFiles discovers dependency manifest files
func DiscoverDependencyFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	output := DiscoverAndCreateIssueResults{
		DiscoverResults: DiscoverResults{
			Responses: make([]*db.History, 0),
			Errors:    make([]error, 0),
		},
		Issues: make([]db.Issue, 0),
		Errors: make([]error, 0),
	}

	// Discover paths
	discoverResults, err := DiscoverPaths(DiscoveryInput{
		URL:                    options.BaseURL,
		Method:                 "GET",
		Paths:                  DependencyFilePaths,
		Concurrency:            10,
		Timeout:                DefaultTimeout,
		HistoryCreationOptions: options.HistoryCreationOptions,
		HttpClient:             options.HttpClient,
		SiteBehavior:           options.SiteBehavior,
		ScanMode:               options.ScanMode,
	})

	if err != nil {
		return output, fmt.Errorf("dependency file discovery failed: %w", err)
	}

	output.DiscoverResults = discoverResults

	// Process each response
	for _, history := range discoverResults.Responses {
		if history.StatusCode != 200 {
			continue
		}

		result := validateDependencyFile(history, options.HttpClient)
		if !result.Valid {
			continue
		}

		// Get the appropriate issue code for this file type
		issueCode, ok := fileTypeToIssueCode[result.FileType]
		if !ok {
			log.Warn().Str("file_type", string(result.FileType)).Msg("Unknown file type for issue code mapping")
			continue
		}

		// Create the file exposure issue
		issue, err := db.CreateIssueFromHistoryAndTemplate(
			history,
			issueCode,
			result.Details,
			result.Confidence,
			"", // Use template severity
			&options.HistoryCreationOptions.WorkspaceID,
			&options.HistoryCreationOptions.TaskID,
			&options.HistoryCreationOptions.TaskJobID,
			&options.HistoryCreationOptions.ScanID,
			&options.HistoryCreationOptions.ScanJobID,
		)
		if err != nil {
			output.Errors = append(output.Errors, fmt.Errorf("failed to create issue: %w", err))
			continue
		}
		output.Issues = append(output.Issues, issue)

		// If dependency confusion was detected, also create a high-severity issue
		if result.ConfusionDetected && len(result.MissingPackages) > 0 {
			var confusionDetails strings.Builder
			confusionDetails.WriteString(fmt.Sprintf("Dependency confusion vulnerability detected in %s\n\n", history.URL))
			confusionDetails.WriteString("The following packages were NOT found in the public registry:\n\n")

			for _, pkg := range result.MissingPackages {
				confusionDetails.WriteString(fmt.Sprintf("Package: %s\n", pkg.Name))
				confusionDetails.WriteString(fmt.Sprintf("  Version: %s\n", pkg.Version))
				confusionDetails.WriteString(fmt.Sprintf("  Source: %s\n", pkg.Source))
				confusionDetails.WriteString(fmt.Sprintf("  Registry: %s\n\n", pkg.Registry))
			}

			confusionDetails.WriteString("These appear to be private/internal packages. An attacker could:\n")
			confusionDetails.WriteString("1. Register these package names on the public registry\n")
			confusionDetails.WriteString("2. Publish malicious versions with higher version numbers\n")
			confusionDetails.WriteString("3. Cause the build system to install the malicious package\n")

			confusionIssue, err := db.CreateIssueFromHistoryAndTemplate(
				history,
				db.DependencyConfusionCode,
				confusionDetails.String(),
				95,
				"", // Use template severity (High)
				&options.HistoryCreationOptions.WorkspaceID,
				&options.HistoryCreationOptions.TaskID,
				&options.HistoryCreationOptions.TaskJobID,
				&options.HistoryCreationOptions.ScanID,
				&options.HistoryCreationOptions.ScanJobID,
			)
			if err != nil {
				output.Errors = append(output.Errors, fmt.Errorf("failed to create dependency confusion issue: %w", err))
				continue
			}
			output.Issues = append(output.Issues, confusionIssue)
		}
	}

	return output, nil
}
