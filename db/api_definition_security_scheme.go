package db

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type APIDefinitionSecurityScheme struct {
	BaseUUIDModel
	DefinitionID     uuid.UUID     `gorm:"type:uuid;index;not null" json:"definition_id"`
	Definition       APIDefinition `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	Name             string        `gorm:"size:255;not null" json:"name"`
	Type             string        `gorm:"size:50;not null" json:"type"`
	Scheme           string        `gorm:"size:50" json:"scheme"`
	In               string        `gorm:"size:50" json:"in"`
	ParameterName    string        `gorm:"size:255" json:"parameter_name"`
	BearerFormat     string        `gorm:"size:50" json:"bearer_format"`
	Description      string        `gorm:"type:text" json:"description"`
	OpenIDConnectURL string        `gorm:"type:text" json:"openid_connect_url"`
}

// BeforeSave truncates the fields backed by bounded columns. These values come
// straight out of a scan target's OpenAPI document, and an over-long one would fail
// the insert and roll back the surrounding transaction, discarding the endpoints
// persisted alongside it.
func (s *APIDefinitionSecurityScheme) BeforeSave(*gorm.DB) error {
	s.Name = truncateRunes(s.Name, 255)
	s.Type = truncateRunes(s.Type, 50)
	s.Scheme = truncateRunes(s.Scheme, 50)
	s.In = truncateRunes(s.In, 50)
	s.ParameterName = truncateRunes(s.ParameterName, 255)
	s.BearerFormat = truncateRunes(s.BearerFormat, 50)
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (d *DatabaseConnection) CreateAPIDefinitionSecuritySchemes(schemes []*APIDefinitionSecurityScheme) error {
	if len(schemes) == 0 {
		return nil
	}
	result := d.db.Create(schemes)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(schemes)).Msg("Batch APIDefinitionSecurityScheme creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) GetAPIDefinitionSecuritySchemes(definitionID uuid.UUID) ([]*APIDefinitionSecurityScheme, error) {
	var schemes []*APIDefinitionSecurityScheme
	err := d.db.Where("definition_id = ?", definitionID).Find(&schemes).Error
	return schemes, err
}

func (d *DatabaseConnection) DeleteAPIDefinitionSecuritySchemesByDefinitionID(definitionID uuid.UUID) error {
	return d.db.Where("definition_id = ?", definitionID).Delete(&APIDefinitionSecurityScheme{}).Error
}
