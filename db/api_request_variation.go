package db

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type APIRequestVariation struct {
	BaseUUIDModel
	EndpointID uuid.UUID   `gorm:"type:uuid;index;not null" json:"endpoint_id"`
	Endpoint   APIEndpoint `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	Label       string `gorm:"size:255;not null" json:"label"`
	Description string `gorm:"type:text" json:"description"`
	URL         string `gorm:"type:text" json:"url"`
	Method      string `gorm:"size:10" json:"method"`
	Headers     []byte `gorm:"type:bytea" json:"headers,omitempty"`
	Body        []byte `gorm:"type:bytea" json:"body,omitempty"`
	ContentType string `gorm:"size:100" json:"content_type"`

	Query         string `gorm:"type:text" json:"query"`
	Variables     []byte `gorm:"type:bytea" json:"variables,omitempty"`
	OperationName string `gorm:"size:255" json:"operation_name"`
}

func (v APIRequestVariation) String() string {
	return fmt.Sprintf("[%s] %s - %s %s", v.Label, v.Description, v.Method, v.URL)
}

func (d *DatabaseConnection) CreateAPIRequestVariation(variation *APIRequestVariation) (*APIRequestVariation, error) {
	result := d.db.Create(variation)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("variation", variation).Msg("APIRequestVariation creation failed")
	}
	return variation, result.Error
}

func (d *DatabaseConnection) CreateAPIRequestVariations(variations []*APIRequestVariation) error {
	if len(variations) == 0 {
		return nil
	}
	result := d.db.Create(variations)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(variations)).Msg("Batch APIRequestVariation creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) GetAPIRequestVariationsByEndpointID(endpointID uuid.UUID) ([]*APIRequestVariation, error) {
	var variations []*APIRequestVariation
	err := d.db.Where("endpoint_id = ?", endpointID).Find(&variations).Error
	return variations, err
}

func (d *DatabaseConnection) GetAPIRequestVariationByID(id uuid.UUID) (*APIRequestVariation, error) {
	var variation APIRequestVariation
	err := d.db.Where("id = ?", id).First(&variation).Error
	if err != nil {
		return nil, err
	}
	return &variation, nil
}

func (d *DatabaseConnection) DeleteAPIRequestVariationsByEndpointID(endpointID uuid.UUID) error {
	return d.db.Where("endpoint_id = ?", endpointID).Delete(&APIRequestVariation{}).Error
}
