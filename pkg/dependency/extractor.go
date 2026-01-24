package dependency

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// npm package patterns
	packageDependencyRegex = regexp.MustCompile(`"([^"@][^"]+)"\s*:\s*"([^"]+)"`)
	scopedPackageRegex     = regexp.MustCompile(`"(@[^"]+/[^"]+)"\s*:\s*"([^"]+)"`)

	// Gemfile patterns
	gemRegex      = regexp.MustCompile(`(?m)^\s*gem\s+['"]([^'"]+)['"](?:\s*,\s*['"]([^'"]+)['"])?`)
	gemGroupRegex = regexp.MustCompile(`(?m)^\s*gem\s+['"]([^'"]+)['"]`)
	gemLockRegex  = regexp.MustCompile(`(?m)^\s{4}([a-zA-Z0-9_-]+)\s+\(([^)]+)\)`)
	gemLockNameRe = regexp.MustCompile(`(?m)^    ([a-zA-Z][a-zA-Z0-9_-]*)\s+\(`)

	// requirements.txt patterns
	requirementRegex = regexp.MustCompile(`(?m)^([a-zA-Z0-9_-][a-zA-Z0-9._-]*)\s*(?:[=<>!~]+\s*([^\s#,;]+))?`)

	// Yarn lock pattern
	yarnPackageRegex = regexp.MustCompile(`^"?([^@"\s]+)@([^":\s]+)"?:`)

	// Go mod pattern
	goModRequireRegex = regexp.MustCompile(`(?m)^\s*require\s+([^\s]+)\s+([^\s]+)`)
	goModBlockRegex   = regexp.MustCompile(`(?s)require\s*\(\s*(.*?)\s*\)`)
	goModLineRegex    = regexp.MustCompile(`(?m)^\s*([^\s]+)\s+([^\s]+)`)

	// Cargo.toml pattern
	cargoDepRegex = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_-]+)\s*=\s*(?:"([^"]+)"|{[^}]*version\s*=\s*"([^"]+)"[^}]*})`)
)

// ExtractPackages extracts package information from file content based on file type
func ExtractPackages(content string, fileType FileType) []PackageInfo {
	registry := FileTypeToRegistry[fileType]

	var packages []PackageInfo

	switch fileType {
	case FileTypePackageJSON:
		packages = ExtractFromPackageJSON(content)
	case FileTypePackageLock:
		packages = ExtractFromPackageLock(content)
	case FileTypeYarnLock:
		packages = ExtractFromYarnLock(content)
	case FileTypeComposerJSON:
		packages = ExtractFromComposerJSON(content)
	case FileTypeComposerLock:
		packages = ExtractFromComposerLock(content)
	case FileTypeGemfile:
		packages = ExtractFromGemfile(content)
	case FileTypeGemfileLock:
		packages = ExtractFromGemfileLock(content)
	case FileTypeRequirementsTxt:
		packages = ExtractFromRequirementsTxt(content)
	case FileTypePipfile:
		packages = ExtractFromPipfile(content)
	case FileTypeGoMod:
		packages = ExtractFromGoMod(content)
	case FileTypeCargoTOML:
		packages = ExtractFromCargoTOML(content)
	default:
		return packages
	}

	// Set registry for all packages
	for i := range packages {
		packages[i].Registry = registry
	}

	return packages
}

// ExtractFromPackageJSON extracts packages from package.json content
func ExtractFromPackageJSON(content string) []PackageInfo {
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

	// Extract peerDependencies
	if peerDeps, ok := packageData["peerDependencies"].(map[string]interface{}); ok {
		for name, version := range peerDeps {
			if versionStr, ok := version.(string); ok {
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "package.json (peerDependencies)",
				})
			}
		}
	}

	// Extract optionalDependencies
	if optDeps, ok := packageData["optionalDependencies"].(map[string]interface{}); ok {
		for name, version := range optDeps {
			if versionStr, ok := version.(string); ok {
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "package.json (optionalDependencies)",
				})
			}
		}
	}

	return packages
}

// ExtractFromPackageLock extracts packages from package-lock.json content
func ExtractFromPackageLock(content string) []PackageInfo {
	var packages []PackageInfo
	var lockData map[string]interface{}

	if err := json.Unmarshal([]byte(content), &lockData); err != nil {
		return extractPackagesWithRegex(content, "package-lock.json")
	}

	// Extract from dependencies (npm v1/v2 format)
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

	// Extract from packages (npm v3 format)
	if pkgs, ok := lockData["packages"].(map[string]interface{}); ok {
		for path, pkgInfo := range pkgs {
			if path == "" {
				continue // Skip root package
			}
			if pkgMap, ok := pkgInfo.(map[string]interface{}); ok {
				// Extract package name from node_modules path
				name := path
				if strings.Contains(path, "node_modules/") {
					parts := strings.Split(path, "node_modules/")
					if len(parts) > 1 {
						name = parts[len(parts)-1]
					}
				}
				if version, ok := pkgMap["version"].(string); ok {
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

// ExtractFromYarnLock extracts packages from yarn.lock content
func ExtractFromYarnLock(content string) []PackageInfo {
	var packages []PackageInfo
	seen := make(map[string]bool)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := yarnPackageRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			name := matches[1]
			version := matches[2]
			key := name + "@" + version
			if !seen[key] {
				seen[key] = true
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  "yarn.lock",
				})
			}
		}
	}

	return packages
}

// ExtractFromComposerJSON extracts packages from composer.json content
func ExtractFromComposerJSON(content string) []PackageInfo {
	var packages []PackageInfo
	var composerData map[string]interface{}

	if err := json.Unmarshal([]byte(content), &composerData); err != nil {
		return packages
	}

	// Extract require
	if require, ok := composerData["require"].(map[string]interface{}); ok {
		for name, version := range require {
			if versionStr, ok := version.(string); ok {
				// Skip PHP version constraint
				if name == "php" || strings.HasPrefix(name, "ext-") {
					continue
				}
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "composer.json (require)",
				})
			}
		}
	}

	// Extract require-dev
	if requireDev, ok := composerData["require-dev"].(map[string]interface{}); ok {
		for name, version := range requireDev {
			if versionStr, ok := version.(string); ok {
				if name == "php" || strings.HasPrefix(name, "ext-") {
					continue
				}
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: versionStr,
					Source:  "composer.json (require-dev)",
				})
			}
		}
	}

	return packages
}

// ExtractFromComposerLock extracts packages from composer.lock content
func ExtractFromComposerLock(content string) []PackageInfo {
	var packages []PackageInfo
	var lockData map[string]interface{}

	if err := json.Unmarshal([]byte(content), &lockData); err != nil {
		return packages
	}

	extractPackagesFromArray := func(key string, source string) {
		if pkgs, ok := lockData[key].([]interface{}); ok {
			for _, pkg := range pkgs {
				if pkgMap, ok := pkg.(map[string]interface{}); ok {
					name, _ := pkgMap["name"].(string)
					version, _ := pkgMap["version"].(string)
					if name != "" {
						packages = append(packages, PackageInfo{
							Name:    name,
							Version: version,
							Source:  source,
						})
					}
				}
			}
		}
	}

	extractPackagesFromArray("packages", "composer.lock")
	extractPackagesFromArray("packages-dev", "composer.lock (dev)")

	return packages
}

// ExtractFromGemfile extracts gems from Gemfile content
func ExtractFromGemfile(content string) []PackageInfo {
	var packages []PackageInfo
	seen := make(map[string]bool)

	matches := gemRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			name := match[1]
			version := ""
			if len(match) >= 3 {
				version = match[2]
			}
			if !seen[name] {
				seen[name] = true
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  "Gemfile",
				})
			}
		}
	}

	return packages
}

// ExtractFromGemfileLock extracts gems from Gemfile.lock content
func ExtractFromGemfileLock(content string) []PackageInfo {
	var packages []PackageInfo
	seen := make(map[string]bool)

	matches := gemLockNameRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			name := match[1]
			if !seen[name] {
				seen[name] = true
				packages = append(packages, PackageInfo{
					Name:   name,
					Source: "Gemfile.lock",
				})
			}
		}
	}

	return packages
}

// ExtractFromRequirementsTxt extracts packages from requirements.txt content
func ExtractFromRequirementsTxt(content string) []PackageInfo {
	var packages []PackageInfo
	seen := make(map[string]bool)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip -r, -e, --index-url, etc.
		if strings.HasPrefix(line, "-") {
			continue
		}

		matches := requirementRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			name := strings.ToLower(matches[1]) // Python packages are case-insensitive
			version := ""
			if len(matches) >= 3 {
				version = matches[2]
			}
			if !seen[name] {
				seen[name] = true
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  "requirements.txt",
				})
			}
		}
	}

	return packages
}

// ExtractFromPipfile extracts packages from Pipfile content
func ExtractFromPipfile(content string) []PackageInfo {
	var packages []PackageInfo

	// Simple TOML-like parsing for Pipfile
	inPackages := false
	inDevPackages := false

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "[packages]" {
			inPackages = true
			inDevPackages = false
			continue
		}
		if line == "[dev-packages]" {
			inPackages = false
			inDevPackages = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inPackages = false
			inDevPackages = false
			continue
		}

		if (inPackages || inDevPackages) && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				version := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				source := "Pipfile"
				if inDevPackages {
					source = "Pipfile (dev)"
				}
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  source,
				})
			}
		}
	}

	return packages
}

// ExtractFromGoMod extracts modules from go.mod content
func ExtractFromGoMod(content string) []PackageInfo {
	var packages []PackageInfo
	seen := make(map[string]bool)

	// Find require blocks
	blockMatches := goModBlockRegex.FindAllStringSubmatch(content, -1)
	for _, blockMatch := range blockMatches {
		if len(blockMatch) >= 2 {
			blockContent := blockMatch[1]
			lineMatches := goModLineRegex.FindAllStringSubmatch(blockContent, -1)
			for _, lineMatch := range lineMatches {
				if len(lineMatch) >= 3 {
					name := lineMatch[1]
					version := lineMatch[2]
					if !seen[name] && !strings.HasPrefix(name, "//") {
						seen[name] = true
						packages = append(packages, PackageInfo{
							Name:    name,
							Version: version,
							Source:  "go.mod",
						})
					}
				}
			}
		}
	}

	// Find single-line require statements
	singleMatches := goModRequireRegex.FindAllStringSubmatch(content, -1)
	for _, match := range singleMatches {
		if len(match) >= 3 {
			name := match[1]
			version := match[2]
			if !seen[name] {
				seen[name] = true
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  "go.mod",
				})
			}
		}
	}

	return packages
}

// ExtractFromCargoTOML extracts crates from Cargo.toml content
func ExtractFromCargoTOML(content string) []PackageInfo {
	var packages []PackageInfo

	// Simple parsing for [dependencies] and [dev-dependencies] sections
	inDeps := false
	inDevDeps := false

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "[dependencies]" {
			inDeps = true
			inDevDeps = false
			continue
		}
		if trimmedLine == "[dev-dependencies]" {
			inDeps = false
			inDevDeps = true
			continue
		}
		if strings.HasPrefix(trimmedLine, "[") {
			inDeps = false
			inDevDeps = false
			continue
		}

		if inDeps || inDevDeps {
			matches := cargoDepRegex.FindStringSubmatch(line)
			if len(matches) >= 2 {
				name := matches[1]
				version := ""
				if matches[2] != "" {
					version = matches[2]
				} else if matches[3] != "" {
					version = matches[3]
				}
				source := "Cargo.toml"
				if inDevDeps {
					source = "Cargo.toml (dev)"
				}
				packages = append(packages, PackageInfo{
					Name:    name,
					Version: version,
					Source:  source,
				})
			}
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
