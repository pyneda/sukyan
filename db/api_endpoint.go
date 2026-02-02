package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type APIEndpoint struct {
	BaseUUIDModel
	DefinitionID uuid.UUID     `gorm:"type:uuid;index;index:idx_endpoint_def_enabled,priority:1;not null" json:"definition_id"`
	Definition   APIDefinition `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	OperationID   string `gorm:"size:255;index" json:"operation_id"`
	Name          string `gorm:"size:255" json:"name"`
	Summary       string `gorm:"size:500" json:"summary"`
	Description   string `gorm:"type:text" json:"description"`
	Enabled       bool   `gorm:"default:true;index;index:idx_endpoint_def_enabled,priority:2" json:"enabled"`
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
	IssuesFound   int    `gorm:"default:0" json:"issues_found"`

	Method string `gorm:"size:10;index" json:"method"`
	Path   string `gorm:"type:text" json:"path"`

	OperationType string `gorm:"size:50" json:"operation_type"`
	ReturnType    string `gorm:"size:255" json:"return_type"`

	ServiceName  string `gorm:"size:255" json:"service_name"`
	PortName     string `gorm:"size:255" json:"port_name"`
	SOAPAction   string `gorm:"size:500" json:"soap_action"`
	BindingStyle string `gorm:"size:50" json:"binding_style"`

}

func (e APIEndpoint) TableHeaders() []string {
	return []string{"ID", "Method", "Path/Operation", "Name", "Enabled", "Issues", "Last Scanned"}
}

func (e APIEndpoint) TableRow() []string {
	path := e.Path
	if e.OperationType != "" {
		path = fmt.Sprintf("%s %s", e.OperationType, e.Name)
	}
	if len(path) > 50 {
		path = path[:50] + "..."
	}

	lastScanned := "never"
	if e.LastScannedAt != nil {
		lastScanned = e.LastScannedAt.Format(time.RFC3339)
	}

	enabled := "no"
	if e.Enabled {
		enabled = "yes"
	}

	return []string{
		e.ID.String()[:8],
		e.Method,
		path,
		e.Name,
		enabled,
		fmt.Sprintf("%d", e.IssuesFound),
		lastScanned,
	}
}

func (e APIEndpoint) String() string {
	return fmt.Sprintf("ID: %s, Method: %s, Path: %s, Name: %s",
		e.ID.String()[:8], e.Method, e.Path, e.Name)
}

func (e APIEndpoint) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %s\n%sMethod:%s %s\n%sPath:%s %s\n%sName:%s %s\n%sSummary:%s %s\n%sEnabled:%s %t\n%sIssues Found:%s %d\n",
		lib.Blue, lib.ResetColor, e.ID.String()[:8],
		lib.Blue, lib.ResetColor, e.Method,
		lib.Blue, lib.ResetColor, e.Path,
		lib.Blue, lib.ResetColor, e.Name,
		lib.Blue, lib.ResetColor, e.Summary,
		lib.Blue, lib.ResetColor, e.Enabled,
		lib.Blue, lib.ResetColor, e.IssuesFound,
	)
}

type APIEndpointFilter struct {
	Query         string     `json:"query" validate:"omitempty,ascii"`
	DefinitionID  *uuid.UUID `json:"definition_id" validate:"omitempty"`
	Methods       []string   `json:"methods" validate:"omitempty"`
	Enabled       *bool      `json:"enabled" validate:"omitempty"`
	OperationType string     `json:"operation_type" validate:"omitempty"`
	Pagination    Pagination `json:"pagination"`
	SortBy        string     `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at method path name issues_found last_scanned_at"`
	SortOrder     string     `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

func (d *DatabaseConnection) CreateAPIEndpoint(endpoint *APIEndpoint) (*APIEndpoint, error) {
	result := d.db.Create(endpoint)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("endpoint", endpoint).Msg("APIEndpoint creation failed")
	}
	return endpoint, result.Error
}

func (d *DatabaseConnection) CreateAPIEndpoints(endpoints []*APIEndpoint) error {
	if len(endpoints) == 0 {
		return nil
	}
	result := d.db.Create(endpoints)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(endpoints)).Msg("Batch APIEndpoint creation failed")
	}
	return result.Error
}

func (d *DatabaseConnection) GetAPIEndpointByID(id uuid.UUID) (*APIEndpoint, error) {
	var endpoint APIEndpoint
	err := d.db.Where("id = ?", id).First(&endpoint).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API endpoint by ID")
		return nil, err
	}
	return &endpoint, nil
}

func (d *DatabaseConnection) GetAPIEndpointByIDWithRelations(id uuid.UUID) (*APIEndpoint, error) {
	var endpoint APIEndpoint
	err := d.db.Where("id = ?", id).First(&endpoint).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API endpoint by ID with relations")
		return nil, err
	}
	return &endpoint, nil
}

func (d *DatabaseConnection) UpdateAPIEndpoint(endpoint *APIEndpoint) (*APIEndpoint, error) {
	result := d.db.Save(endpoint)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("endpoint", endpoint).Msg("APIEndpoint update failed")
	}
	return endpoint, result.Error
}

func (d *DatabaseConnection) DeleteAPIEndpoint(id uuid.UUID) error {
	if err := d.db.Delete(&APIEndpoint{}, "id = ?", id).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Error deleting API endpoint")
		return err
	}
	return nil
}

func (d *DatabaseConnection) ListAPIEndpoints(filter APIEndpointFilter) (items []*APIEndpoint, count int64, err error) {
	query := d.db.Model(&APIEndpoint{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("name ILIKE ? OR path ILIKE ? OR summary ILIKE ? OR operation_id ILIKE ?",
			likeQuery, likeQuery, likeQuery, likeQuery)
	}

	if filter.DefinitionID != nil {
		query = query.Where("definition_id = ?", *filter.DefinitionID)
	}

	if len(filter.Methods) > 0 {
		query = query.Where("method IN ?", filter.Methods)
	}

	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}

	if filter.OperationType != "" {
		query = query.Where("operation_type = ?", filter.OperationType)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	validSortBy := map[string]bool{
		"id":              true,
		"created_at":      true,
		"updated_at":      true,
		"method":          true,
		"path":            true,
		"name":            true,
		"issues_found":    true,
		"last_scanned_at": true,
	}

	order := "path asc, method asc"
	if validSortBy[filter.SortBy] {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		order = filter.SortBy + " " + sortOrder
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Order(order).Find(&items).Error
	return items, count, err
}

func (d *DatabaseConnection) GetAPIEndpointsByDefinitionID(definitionID uuid.UUID) ([]*APIEndpoint, error) {
	var endpoints []*APIEndpoint
	err := d.db.Where("definition_id = ?", definitionID).Find(&endpoints).Error
	return endpoints, err
}

func (d *DatabaseConnection) GetEnabledAPIEndpointsByDefinitionID(definitionID uuid.UUID) ([]*APIEndpoint, error) {
	var endpoints []*APIEndpoint
	err := d.db.Where("definition_id = ? AND enabled = true", definitionID).Find(&endpoints).Error
	return endpoints, err
}

func (d *DatabaseConnection) SetAPIEndpointEnabled(id uuid.UUID, enabled bool) error {
	return d.db.Model(&APIEndpoint{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (d *DatabaseConnection) SetAPIEndpointsEnabledByDefinition(definitionID uuid.UUID, enabled bool) error {
	return d.db.Model(&APIEndpoint{}).Where("definition_id = ?", definitionID).Update("enabled", enabled).Error
}

func (d *DatabaseConnection) MarkAPIEndpointScanned(id uuid.UUID, issuesFound int) error {
	now := time.Now()
	return d.db.Model(&APIEndpoint{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_scanned_at": now,
		"issues_found":    issuesFound,
	}).Error
}

func (d *DatabaseConnection) IncrementAPIEndpointIssuesFound(id uuid.UUID, count int) error {
	return d.db.Model(&APIEndpoint{}).Where("id = ?", id).
		Update("issues_found", gorm.Expr("issues_found + ?", count)).Error
}

func (d *DatabaseConnection) GetAPIEndpointByOperationID(definitionID uuid.UUID, operationID string) (*APIEndpoint, error) {
	var endpoint APIEndpoint
	err := d.db.Where("definition_id = ? AND operation_id = ?", definitionID, operationID).First(&endpoint).Error
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (d *DatabaseConnection) GetAPIEndpointByPathAndMethod(definitionID uuid.UUID, path, method string) (*APIEndpoint, error) {
	var endpoint APIEndpoint
	err := d.db.Where("definition_id = ? AND path = ? AND method = ?", definitionID, path, method).First(&endpoint).Error
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (d *DatabaseConnection) GetAPIEndpointsByIDs(definitionID uuid.UUID, endpointIDs []uuid.UUID) ([]*APIEndpoint, error) {
	if len(endpointIDs) == 0 {
		return nil, nil
	}
	var endpoints []*APIEndpoint
	err := d.db.Where("definition_id = ? AND id IN ?", definitionID, endpointIDs).Find(&endpoints).Error
	return endpoints, err
}
