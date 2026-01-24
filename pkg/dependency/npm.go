package dependency

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
)

// NPMChecker checks packages against the npm registry
type NPMChecker struct {
	client *http.Client
}

// NewNPMChecker creates a new npm registry checker
func NewNPMChecker(client *http.Client) *NPMChecker {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &NPMChecker{client: client}
}

// GetRegistryType returns the registry type
func (c *NPMChecker) GetRegistryType() RegistryType {
	return RegistryNPM
}

// npmRegistryResponse represents a simplified npm registry response
type npmRegistryResponse struct {
	Name     string                 `json:"name"`
	Versions map[string]interface{} `json:"versions"`
	Error    string                 `json:"error,omitempty"`
}

// CheckPackage checks if a package exists in the npm registry
func (c *NPMChecker) CheckPackage(packageName string) RegistryCheckResult {
	result := RegistryCheckResult{
		PackageName: packageName,
	}

	// First try the official npm registry
	exists, version := c.checkOfficialRegistry(packageName)
	if exists {
		result.Exists = true
		result.LatestVersion = version
		return result
	}

	// Fallback to unpkg.com
	exists, version = c.checkUnpkg(packageName)
	result.Exists = exists
	result.LatestVersion = version

	return result
}

// checkOfficialRegistry checks the official npm registry
func (c *NPMChecker) checkOfficialRegistry(packageName string) (bool, string) {
	// URL encode the package name to handle scoped packages like @org/package
	encodedName := url.PathEscape(packageName)
	registryURL := fmt.Sprintf("https://registry.npmjs.org/%s", encodedName)

	resp, err := c.client.Get(registryURL)
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

	var npmResp npmRegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&npmResp); err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to decode npm registry response")
		return false, ""
	}

	if npmResp.Error != "" {
		return false, ""
	}

	// Get version count
	latestVersion := "unknown"
	if len(npmResp.Versions) > 0 {
		latestVersion = fmt.Sprintf("%d versions available", len(npmResp.Versions))
	}

	return true, latestVersion
}

// checkUnpkg checks unpkg.com as a fallback
func (c *NPMChecker) checkUnpkg(packageName string) (bool, string) {
	unpkgURL := fmt.Sprintf("https://unpkg.com/%s", packageName)

	resp, err := c.client.Get(unpkgURL)
	if err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to check unpkg registry")
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, ""
	}

	if resp.StatusCode == 200 || resp.StatusCode == 302 {
		return true, "available on unpkg"
	}

	log.Debug().Int("status", resp.StatusCode).Str("package", packageName).Msg("Unexpected status from unpkg registry")
	return false, ""
}
