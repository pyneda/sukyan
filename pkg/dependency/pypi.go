package dependency

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

// PyPIChecker checks packages against the PyPI registry
type PyPIChecker struct {
	client *http.Client
}

// NewPyPIChecker creates a new PyPI registry checker
func NewPyPIChecker(client *http.Client) *PyPIChecker {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &PyPIChecker{client: client}
}

// GetRegistryType returns the registry type
func (c *PyPIChecker) GetRegistryType() RegistryType {
	return RegistryPyPI
}

// pypiResponse represents a simplified PyPI JSON API response
type pypiResponse struct {
	Info struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"info"`
}

// CheckPackage checks if a package exists in the PyPI registry
func (c *PyPIChecker) CheckPackage(packageName string) RegistryCheckResult {
	result := RegistryCheckResult{
		PackageName: packageName,
	}

	pypiURL := fmt.Sprintf("https://pypi.org/pypi/%s/json", packageName)

	resp, err := c.client.Get(pypiURL)
	if err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to check PyPI registry")
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		result.Exists = false
		return result
	}

	if resp.StatusCode != 200 {
		log.Debug().Int("status", resp.StatusCode).Str("package", packageName).Msg("Unexpected status from PyPI registry")
		return result
	}

	var pypiResp pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&pypiResp); err != nil {
		log.Debug().Err(err).Str("package", packageName).Msg("Failed to decode PyPI registry response")
		result.Error = err
		return result
	}

	result.Exists = true
	result.LatestVersion = pypiResp.Info.Version

	return result
}
