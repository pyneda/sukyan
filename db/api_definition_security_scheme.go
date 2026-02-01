package db

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
