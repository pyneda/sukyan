package dependency

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

// RubyGemsChecker checks packages against the RubyGems registry
type RubyGemsChecker struct {
	client *http.Client
}

// NewRubyGemsChecker creates a new RubyGems registry checker
func NewRubyGemsChecker(client *http.Client) *RubyGemsChecker {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &RubyGemsChecker{client: client}
}

// GetRegistryType returns the registry type
func (c *RubyGemsChecker) GetRegistryType() RegistryType {
	return RegistryRubyGems
}

// rubygemsResponse represents a simplified RubyGems API response
type rubygemsResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CheckPackage checks if a gem exists in the RubyGems registry
func (c *RubyGemsChecker) CheckPackage(packageName string) RegistryCheckResult {
	result := RegistryCheckResult{
		PackageName: packageName,
	}

	gemURL := fmt.Sprintf("https://rubygems.org/api/v1/gems/%s.json", packageName)

	resp, err := c.client.Get(gemURL)
	if err != nil {
		log.Debug().Err(err).Str("gem", packageName).Msg("Failed to check RubyGems registry")
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		result.Exists = false
		return result
	}

	if resp.StatusCode != 200 {
		log.Debug().Int("status", resp.StatusCode).Str("gem", packageName).Msg("Unexpected status from RubyGems registry")
		return result
	}

	var gemResp rubygemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		log.Debug().Err(err).Str("gem", packageName).Msg("Failed to decode RubyGems registry response")
		result.Error = err
		return result
	}

	result.Exists = true
	result.LatestVersion = gemResp.Version

	return result
}
