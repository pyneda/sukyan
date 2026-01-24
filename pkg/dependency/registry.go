package dependency

import (
	"net/http"
	"time"
)

// RegistryChecker is the interface for checking package existence in registries
type RegistryChecker interface {
	// CheckPackage checks if a package exists in the registry
	CheckPackage(packageName string) RegistryCheckResult
	// GetRegistryType returns the type of registry this checker handles
	GetRegistryType() RegistryType
}

// DefaultHTTPClient returns an HTTP client configured for registry checks
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
	}
}

// CheckPackageInRegistry checks if a package exists using the appropriate registry checker
func CheckPackageInRegistry(pkg PackageInfo, client *http.Client) RegistryCheckResult {
	if client == nil {
		client = DefaultHTTPClient()
	}

	var checker RegistryChecker

	switch pkg.Registry {
	case RegistryNPM:
		checker = NewNPMChecker(client)
	case RegistryPyPI:
		checker = NewPyPIChecker(client)
	case RegistryRubyGems:
		checker = NewRubyGemsChecker(client)
	case RegistryPackagist:
		checker = NewPackagistChecker(client)
	default:
		return RegistryCheckResult{
			PackageName: pkg.Name,
			Exists:      true, // Assume exists if we can't check
		}
	}

	return checker.CheckPackage(pkg.Name)
}

// CheckPackages checks multiple packages and returns those that are missing from public registries
func CheckPackages(packages []PackageInfo, client *http.Client) []PackageInfo {
	if client == nil {
		client = DefaultHTTPClient()
	}

	var missing []PackageInfo

	for _, pkg := range packages {
		result := CheckPackageInRegistry(pkg, client)
		if !result.Exists && result.Error == nil {
			missing = append(missing, pkg)
		}
	}

	return missing
}
