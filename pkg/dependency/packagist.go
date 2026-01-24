package dependency

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// PackagistChecker checks packages against the Packagist registry (PHP/Composer)
type PackagistChecker struct {
	client *http.Client
}

// NewPackagistChecker creates a new Packagist registry checker
func NewPackagistChecker(client *http.Client) *PackagistChecker {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &PackagistChecker{client: client}
}

// GetRegistryType returns the registry type
func (c *PackagistChecker) GetRegistryType() RegistryType {
	return RegistryPackagist
}

// CheckPackage checks if a package exists in the Packagist registry
// Package names in Composer are in the format "vendor/package"
func (c *PackagistChecker) CheckPackage(packageName string) RegistryCheckResult {
	result := RegistryCheckResult{
		PackageName: packageName,
	}

	// Packagist requires vendor/package format
	if !strings.Contains(packageName, "/") {
		// Without vendor, we can't check Packagist
		log.Debug().Str("package", packageName).Msg("Packagist package without vendor prefix, cannot check")
		result.Exists = true // Assume exists since we can't verify
		return result
	}

	// Use the Packagist repo API endpoint
	packagistURL := fmt.Sprintf("https://repo.packagist.org/p2/%s.json", packageName)

	resp, err := c.client.Get(packagistURL)
	if err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to check Packagist registry")
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		result.Exists = false
		return result
	}

	if resp.StatusCode == 200 {
		result.Exists = true
		result.LatestVersion = "available on Packagist"
		return result
	}

	log.Debug().Int("status", resp.StatusCode).Str("package", packageName).Msg("Unexpected status from Packagist registry")
	return result
}
