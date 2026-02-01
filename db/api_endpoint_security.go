package db

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type APIEndpointSecurity struct {
	BaseUUIDModel
	EndpointID uuid.UUID   `gorm:"type:uuid;index;not null" json:"endpoint_id"`
	Endpoint   APIEndpoint `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	SchemeName string `gorm:"size:255;not null" json:"scheme_name"`
	Scopes     string `gorm:"type:text" json:"scopes"`
}

func (d *DatabaseConnection) CreateAPIEndpointSecurity(sec *APIEndpointSecurity) (*APIEndpointSecurity, error) {
	result := d.db.Create(sec)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("security", sec).Msg("APIEndpointSecurity creation failed")
	}
	return sec, result.Error
}

func (d *DatabaseConnection) CreateAPIEndpointSecurities(secs []*APIEndpointSecurity) error {
	if len(secs) == 0 {
		return nil
	}
	result := d.db.Create(secs)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(secs)).Msg("Batch APIEndpointSecurity creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) GetAPIEndpointSecuritiesByEndpointID(endpointID uuid.UUID) ([]*APIEndpointSecurity, error) {
	var secs []*APIEndpointSecurity
	err := d.db.Where("endpoint_id = ?", endpointID).Find(&secs).Error
	return secs, err
}

func (d *DatabaseConnection) DeleteAPIEndpointSecuritiesByEndpointID(endpointID uuid.UUID) error {
	return d.db.Where("endpoint_id = ?", endpointID).Delete(&APIEndpointSecurity{}).Error
}
