package db

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type APIParameterLocation string

const (
	APIParamLocationPath   APIParameterLocation = "path"
	APIParamLocationQuery  APIParameterLocation = "query"
	APIParamLocationHeader APIParameterLocation = "header"
	APIParamLocationCookie APIParameterLocation = "cookie"
	APIParamLocationBody   APIParameterLocation = "body"
)

type APIEndpointParameter struct {
	BaseUUIDModel
	EndpointID uuid.UUID   `gorm:"type:uuid;index;not null" json:"endpoint_id"`
	Endpoint   APIEndpoint `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	Name     string               `gorm:"size:255;not null" json:"name"`
	Location APIParameterLocation `gorm:"size:50;not null" json:"location"`
	Required bool                 `gorm:"default:false" json:"required"`

	DataType string `gorm:"size:50" json:"data_type"`
	Format   string `gorm:"size:50" json:"format"`

	Pattern   string   `gorm:"size:500" json:"pattern"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`

	EnumValues string `gorm:"type:text" json:"enum_values"`

	DefaultValue string `gorm:"type:text" json:"default_value"`
	Example      string `gorm:"type:text" json:"example"`
}

func (p APIEndpointParameter) String() string {
	required := ""
	if p.Required {
		required = " (required)"
	}
	return fmt.Sprintf("%s [%s] %s%s", p.Name, p.Location, p.DataType, required)
}

func (d *DatabaseConnection) CreateAPIEndpointParameter(param *APIEndpointParameter) (*APIEndpointParameter, error) {
	result := d.db.Create(param)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("parameter", param).Msg("APIEndpointParameter creation failed")
	}
	return param, result.Error
}

func (d *DatabaseConnection) CreateAPIEndpointParameters(params []*APIEndpointParameter) error {
	if len(params) == 0 {
		return nil
	}
	result := d.db.Create(params)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(params)).Msg("Batch APIEndpointParameter creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) GetAPIEndpointParametersByEndpointID(endpointID uuid.UUID) ([]*APIEndpointParameter, error) {
	var params []*APIEndpointParameter
	err := d.db.Where("endpoint_id = ?", endpointID).Find(&params).Error
	return params, err
}

func (d *DatabaseConnection) DeleteAPIEndpointParametersByEndpointID(endpointID uuid.UUID) error {
	return d.db.Where("endpoint_id = ?", endpointID).Delete(&APIEndpointParameter{}).Error
}
