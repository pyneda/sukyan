package db

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type TokenExtractionSource string

const (
	TokenExtractionSourceBodyJSONPath   TokenExtractionSource = "body_jsonpath"
	TokenExtractionSourceResponseHeader TokenExtractionSource = "response_header"
)

type TokenRefreshConfig struct {
	BaseUUIDModel
	AuthConfigID       uuid.UUID             `gorm:"type:uuid;uniqueIndex;not null" json:"auth_config_id"`
	AuthConfig         APIAuthConfig         `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	RequestURL         string                `gorm:"type:text;not null" json:"request_url"`
	RequestMethod      string                `gorm:"size:10;not null" json:"request_method"`
	RequestHeaders     map[string]string     `gorm:"type:jsonb;serializer:json" json:"request_headers,omitempty"`
	RequestBody        string                `gorm:"type:text" json:"request_body,omitempty"`
	RequestContentType string                `gorm:"size:100" json:"request_content_type,omitempty"`
	IntervalSeconds    int                   `gorm:"type:integer;not null" json:"interval_seconds"`
	ExtractionSource   TokenExtractionSource `gorm:"size:50;not null" json:"extraction_source"`
	ExtractionValue    string                `gorm:"type:text;not null" json:"extraction_value"`
	CurrentToken       string                `gorm:"type:text" json:"-"`
	TokenFetchedAt     *time.Time            `json:"-"`
	LastError          string                `gorm:"type:text" json:"-"`
}

func (d *DatabaseConnection) CreateTokenRefreshConfig(config *TokenRefreshConfig) (*TokenRefreshConfig, error) {
	result := d.db.Create(config)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("config", config).Msg("TokenRefreshConfig creation failed")
	}
	return config, result.Error
}

func (d *DatabaseConnection) GetTokenRefreshConfigByAuthConfigID(authConfigID uuid.UUID) (*TokenRefreshConfig, error) {
	var config TokenRefreshConfig
	err := d.db.Where("auth_config_id = ?", authConfigID).First(&config).Error
	if err != nil {
		log.Error().Err(err).Str("auth_config_id", authConfigID.String()).Msg("Unable to fetch token refresh config by auth config ID")
		return nil, err
	}
	return &config, nil
}

func (d *DatabaseConnection) UpdateTokenRefreshConfig(config *TokenRefreshConfig) (*TokenRefreshConfig, error) {
	result := d.db.Save(config)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("config", config).Msg("TokenRefreshConfig update failed")
	}
	return config, result.Error
}

func (d *DatabaseConnection) DeleteTokenRefreshConfigByAuthConfigID(authConfigID uuid.UUID) error {
	if err := d.db.Delete(&TokenRefreshConfig{}, "auth_config_id = ?", authConfigID).Error; err != nil {
		log.Error().Err(err).Str("auth_config_id", authConfigID.String()).Msg("Error deleting token refresh config")
		return err
	}
	return nil
}

func (d *DatabaseConnection) UpdateTokenRefreshState(id uuid.UUID, token string, fetchedAt time.Time, lastError string) error {
	result := d.db.Model(&TokenRefreshConfig{}).Where("id = ?", id).Updates(map[string]any{
		"current_token":    token,
		"token_fetched_at": fetchedAt,
		"last_error":       lastError,
	})
	if result.Error != nil {
		log.Error().Err(result.Error).Str("id", id.String()).Msg("TokenRefreshConfig state update failed")
	}
	return result.Error
}
