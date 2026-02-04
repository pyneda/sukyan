package db

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type APIAuthType string

const (
	APIAuthTypeNone   APIAuthType = "none"
	APIAuthTypeBasic  APIAuthType = "basic"
	APIAuthTypeBearer APIAuthType = "bearer"
	APIAuthTypeAPIKey APIAuthType = "api_key"
	APIAuthTypeOAuth2 APIAuthType = "oauth2"
)

type APIKeyLocation string

const (
	APIKeyLocationHeader APIKeyLocation = "header"
	APIKeyLocationQuery  APIKeyLocation = "query"
	APIKeyLocationCookie APIKeyLocation = "cookie"
)

type APIAuthConfig struct {
	BaseUUIDModel
	WorkspaceID uint      `gorm:"index;not null" json:"workspace_id"`
	Workspace   Workspace `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	Name        string    `gorm:"size:255" json:"name"`
	Type        APIAuthType `gorm:"size:50;not null" json:"type"`

	Username string `gorm:"size:255" json:"username,omitempty"`
	Password string `gorm:"size:500" json:"password,omitempty"`

	Token       string `gorm:"type:text" json:"token,omitempty"`
	TokenPrefix string `gorm:"size:50;default:'Bearer'" json:"token_prefix"`

	APIKeyName     string         `gorm:"size:255" json:"api_key_name,omitempty"`
	APIKeyValue    string         `gorm:"type:text" json:"api_key_value,omitempty"`
	APIKeyLocation APIKeyLocation `gorm:"size:50" json:"api_key_location,omitempty"`

	CustomHeaders      []APIAuthHeader     `gorm:"foreignKey:AuthConfigID;constraint:OnDelete:CASCADE" json:"custom_headers,omitempty"`
	TokenRefreshConfig *TokenRefreshConfig `gorm:"foreignKey:AuthConfigID;constraint:OnDelete:CASCADE" json:"token_refresh_config,omitempty"`
}

func (c APIAuthConfig) TableHeaders() []string {
	return []string{"ID", "Name", "Type", "Workspace"}
}

func (c APIAuthConfig) TableRow() []string {
	return []string{
		c.ID.String()[:8],
		c.Name,
		string(c.Type),
		fmt.Sprintf("%d", c.WorkspaceID),
	}
}

func (c APIAuthConfig) String() string {
	return fmt.Sprintf("ID: %s, Name: %s, Type: %s", c.ID.String()[:8], c.Name, c.Type)
}

func (c APIAuthConfig) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %s\n%sName:%s %s\n%sType:%s %s\n%sWorkspace:%s %d\n",
		lib.Blue, lib.ResetColor, c.ID.String()[:8],
		lib.Blue, lib.ResetColor, c.Name,
		lib.Blue, lib.ResetColor, c.Type,
		lib.Blue, lib.ResetColor, c.WorkspaceID,
	)
}

type APIAuthHeader struct {
	BaseUUIDModel
	AuthConfigID uuid.UUID     `gorm:"type:uuid;index;not null" json:"auth_config_id"`
	AuthConfig   APIAuthConfig `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	HeaderName   string        `gorm:"size:255;not null" json:"header_name"`
	HeaderValue  string        `gorm:"type:text;not null" json:"header_value"`
}

type APIAuthConfigFilter struct {
	Query       string      `json:"query" validate:"omitempty,ascii"`
	WorkspaceID uint        `json:"workspace_id" validate:"omitempty,numeric"`
	Types       []APIAuthType `json:"types" validate:"omitempty"`
	Pagination  Pagination  `json:"pagination"`
	SortBy      string      `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at name type"`
	SortOrder   string      `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

func (d *DatabaseConnection) CreateAPIAuthConfig(config *APIAuthConfig) (*APIAuthConfig, error) {
	result := d.db.Create(config)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("config", config).Msg("APIAuthConfig creation failed")
	}
	return config, result.Error
}

func (d *DatabaseConnection) GetAPIAuthConfigByID(id uuid.UUID) (*APIAuthConfig, error) {
	var config APIAuthConfig
	err := d.db.Where("id = ?", id).First(&config).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API auth config by ID")
		return nil, err
	}
	return &config, nil
}

func (d *DatabaseConnection) GetAPIAuthConfigByIDWithRelations(id uuid.UUID) (*APIAuthConfig, error) {
	var config APIAuthConfig
	err := d.db.Preload("CustomHeaders").Preload("TokenRefreshConfig").Where("id = ?", id).First(&config).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API auth config by ID with relations")
		return nil, err
	}
	return &config, nil
}

func (d *DatabaseConnection) UpdateAPIAuthConfig(config *APIAuthConfig) (*APIAuthConfig, error) {
	result := d.db.Save(config)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("config", config).Msg("APIAuthConfig update failed")
	}
	return config, result.Error
}

func (d *DatabaseConnection) DeleteAPIAuthConfig(id uuid.UUID) error {
	if err := d.db.Delete(&APIAuthConfig{}, "id = ?", id).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Error deleting API auth config")
		return err
	}
	return nil
}

func (d *DatabaseConnection) ListAPIAuthConfigs(filter APIAuthConfigFilter) (items []*APIAuthConfig, count int64, err error) {
	query := d.db.Model(&APIAuthConfig{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("name ILIKE ?", likeQuery)
	}

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	if len(filter.Types) > 0 {
		query = query.Where("type IN ?", filter.Types)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	validSortBy := map[string]bool{
		"id":         true,
		"created_at": true,
		"updated_at": true,
		"name":       true,
		"type":       true,
	}

	order := "name asc"
	if validSortBy[filter.SortBy] {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		order = filter.SortBy + " " + sortOrder
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Preload("TokenRefreshConfig").Order(order).Find(&items).Error
	return items, count, err
}

func (d *DatabaseConnection) GetAPIAuthConfigsByWorkspace(workspaceID uint) ([]*APIAuthConfig, error) {
	var configs []*APIAuthConfig
	err := d.db.Where("workspace_id = ?", workspaceID).Find(&configs).Error
	return configs, err
}

func (d *DatabaseConnection) CreateAPIAuthHeader(header *APIAuthHeader) (*APIAuthHeader, error) {
	result := d.db.Create(header)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("header", header).Msg("APIAuthHeader creation failed")
	}
	return header, result.Error
}

func (d *DatabaseConnection) CreateAPIAuthHeaders(headers []*APIAuthHeader) error {
	if len(headers) == 0 {
		return nil
	}
	result := d.db.Create(headers)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(headers)).Msg("Batch APIAuthHeader creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) DeleteAPIAuthHeadersByConfigID(configID uuid.UUID) error {
	return d.db.Where("auth_config_id = ?", configID).Delete(&APIAuthHeader{}).Error
}

func (d *DatabaseConnection) GetAPIAuthHeadersByConfigID(configID uuid.UUID) ([]*APIAuthHeader, error) {
	var headers []*APIAuthHeader
	err := d.db.Where("auth_config_id = ?", configID).Find(&headers).Error
	return headers, err
}
