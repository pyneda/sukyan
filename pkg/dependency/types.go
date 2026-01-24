package dependency

// RegistryType represents the type of package registry
type RegistryType string

const (
	RegistryNPM       RegistryType = "npm"
	RegistryPyPI      RegistryType = "pypi"
	RegistryRubyGems  RegistryType = "rubygems"
	RegistryPackagist RegistryType = "packagist"
	RegistryGo        RegistryType = "go"
	RegistryCrates    RegistryType = "crates"
	RegistryMaven     RegistryType = "maven"
)

// FileType represents the type of dependency file
type FileType string

const (
	FileTypePackageJSON     FileType = "package.json"
	FileTypePackageLock     FileType = "package-lock.json"
	FileTypeYarnLock        FileType = "yarn.lock"
	FileTypePnpmLock        FileType = "pnpm-lock.yaml"
	FileTypeComposerJSON    FileType = "composer.json"
	FileTypeComposerLock    FileType = "composer.lock"
	FileTypeGemfile         FileType = "Gemfile"
	FileTypeGemfileLock     FileType = "Gemfile.lock"
	FileTypeRequirementsTxt FileType = "requirements.txt"
	FileTypePipfile         FileType = "Pipfile"
	FileTypePipfileLock     FileType = "Pipfile.lock"
	FileTypePyProjectTOML   FileType = "pyproject.toml"
	FileTypeGoMod           FileType = "go.mod"
	FileTypeGoSum           FileType = "go.sum"
	FileTypeCargoTOML       FileType = "Cargo.toml"
	FileTypeCargoLock       FileType = "Cargo.lock"
	FileTypePomXML          FileType = "pom.xml"
	FileTypeBuildGradle     FileType = "build.gradle"
	FileTypeBuildGradleKts  FileType = "build.gradle.kts"
)

// PackageInfo represents information about a package dependency
type PackageInfo struct {
	Name     string       `json:"name"`
	Version  string       `json:"version"`
	Source   string       `json:"source"`   // Where it was found (e.g., "package.json (dependencies)")
	Registry RegistryType `json:"registry"` // Which registry to check
}

// RegistryCheckResult represents the result of checking a package in a registry
type RegistryCheckResult struct {
	PackageName   string `json:"package_name"`
	Exists        bool   `json:"exists"`
	LatestVersion string `json:"latest_version,omitempty"`
	Error         error  `json:"error,omitempty"`
}

// FileTypeToRegistry maps file types to their corresponding registries
var FileTypeToRegistry = map[FileType]RegistryType{
	FileTypePackageJSON:     RegistryNPM,
	FileTypePackageLock:     RegistryNPM,
	FileTypeYarnLock:        RegistryNPM,
	FileTypePnpmLock:        RegistryNPM,
	FileTypeComposerJSON:    RegistryPackagist,
	FileTypeComposerLock:    RegistryPackagist,
	FileTypeGemfile:         RegistryRubyGems,
	FileTypeGemfileLock:     RegistryRubyGems,
	FileTypeRequirementsTxt: RegistryPyPI,
	FileTypePipfile:         RegistryPyPI,
	FileTypePipfileLock:     RegistryPyPI,
	FileTypePyProjectTOML:   RegistryPyPI,
	FileTypeGoMod:           RegistryGo,
	FileTypeGoSum:           RegistryGo,
	FileTypeCargoTOML:       RegistryCrates,
	FileTypeCargoLock:       RegistryCrates,
	FileTypePomXML:          RegistryMaven,
	FileTypeBuildGradle:     RegistryMaven,
	FileTypeBuildGradleKts:  RegistryMaven,
}

// GetFileType determines the file type from a path or filename
func GetFileType(path string) (FileType, bool) {
	// Check for exact matches
	fileTypes := []FileType{
		FileTypePackageJSON,
		FileTypePackageLock,
		FileTypeYarnLock,
		FileTypePnpmLock,
		FileTypeComposerJSON,
		FileTypeComposerLock,
		FileTypeGemfile,
		FileTypeGemfileLock,
		FileTypeRequirementsTxt,
		FileTypePipfile,
		FileTypePipfileLock,
		FileTypePyProjectTOML,
		FileTypeGoMod,
		FileTypeGoSum,
		FileTypeCargoTOML,
		FileTypeCargoLock,
		FileTypePomXML,
		FileTypeBuildGradle,
		FileTypeBuildGradleKts,
	}

	for _, ft := range fileTypes {
		if pathEndsWith(path, string(ft)) {
			return ft, true
		}
	}

	return "", false
}

// pathEndsWith checks if a path ends with a specific filename (case-insensitive for some files)
// It ensures the match is at a directory boundary (preceded by / or is the whole string)
func pathEndsWith(path, suffix string) bool {
	if len(path) < len(suffix) {
		return false
	}

	// Check if there's a character before the suffix
	if len(path) > len(suffix) {
		// The character before the suffix must be a path separator
		charBefore := path[len(path)-len(suffix)-1]
		if charBefore != '/' && charBefore != '\\' {
			return false
		}
	}

	pathSuffix := path[len(path)-len(suffix):]

	// Case-insensitive for JSON/lock files
	switch suffix {
	case "package.json", "package-lock.json", "composer.json", "composer.lock":
		return eqIgnoreCase(pathSuffix, suffix)
	default:
		return pathSuffix == suffix
	}
}

func eqIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
