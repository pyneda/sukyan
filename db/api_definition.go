package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type APIDefinitionType string

const (
	APIDefinitionTypeOpenAPI APIDefinitionType = "openapi"
	APIDefinitionTypeGraphQL APIDefinitionType = "graphql"
	APIDefinitionTypeWSDL    APIDefinitionType = "wsdl"
)

type APIDefinitionStatus string

const (
	APIDefinitionStatusParsed    APIDefinitionStatus = "parsed"
	APIDefinitionStatusScanning  APIDefinitionStatus = "scanning"
	APIDefinitionStatusCompleted APIDefinitionStatus = "completed"
	APIDefinitionStatusFailed    APIDefinitionStatus = "failed"
)

type APIDefinition struct {
	BaseUUIDModel
	WorkspaceID     uint                `gorm:"index;not null" json:"workspace_id"`
	Workspace       Workspace           `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	Name            string              `gorm:"size:255" json:"name"`
	Type            APIDefinitionType   `gorm:"size:50;index;not null" json:"type"`
	Status          APIDefinitionStatus `gorm:"size:50;index;default:'parsed'" json:"status"`
	SourceURL       string              `gorm:"type:text" json:"source_url"`
	BaseURL         string              `gorm:"type:text" json:"base_url"`
	SourceHistoryID *uint    `gorm:"index" json:"source_history_id,omitempty"`
	SourceHistory   *History `gorm:"foreignKey:SourceHistoryID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	RawDefinition   []byte              `gorm:"type:bytea" json:"-"`
	AutoDiscovered  bool                `gorm:"default:false" json:"auto_discovered"`
	ScanID          *uint               `gorm:"index" json:"scan_id,omitempty"`
	Scan            *Scan               `gorm:"foreignKey:ScanID;constraint:OnDelete:SET NULL" json:"-"`
	AuthConfigID    *uuid.UUID          `gorm:"type:uuid;index" json:"auth_config_id,omitempty"`
	EndpointCount   int                 `gorm:"default:0" json:"endpoint_count"`

	OpenAPIVersion *string `gorm:"size:20" json:"openapi_version,omitempty"`
	OpenAPITitle   *string `gorm:"size:255" json:"openapi_title,omitempty"`
	OpenAPIServers int     `gorm:"default:0" json:"openapi_servers"`

	GraphQLQueryCount        int `gorm:"default:0" json:"graphql_query_count"`
	GraphQLMutationCount     int `gorm:"default:0" json:"graphql_mutation_count"`
	GraphQLSubscriptionCount int `gorm:"default:0" json:"graphql_subscription_count"`
	GraphQLTypeCount         int `gorm:"default:0" json:"graphql_type_count"`

	WSDLTargetNamespace *string `gorm:"type:text" json:"wsdl_target_namespace,omitempty"`
	WSDLServiceCount    int     `gorm:"default:0" json:"wsdl_service_count"`
	WSDLPortCount       int     `gorm:"default:0" json:"wsdl_port_count"`
	WSDLSOAPVersion     *string `gorm:"size:10" json:"wsdl_soap_version,omitempty"`

	GlobalSecurityJSON []byte `gorm:"type:jsonb" json:"-"`

	Endpoints       []APIEndpoint                 `gorm:"foreignKey:DefinitionID;constraint:OnDelete:CASCADE" json:"endpoints,omitempty"`
	SecuritySchemes []APIDefinitionSecurityScheme  `gorm:"foreignKey:DefinitionID;constraint:OnDelete:CASCADE" json:"security_schemes,omitempty"`
	AuthConfig      *APIAuthConfig                 `gorm:"foreignKey:AuthConfigID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"auth_config,omitempty"`
}

func (d APIDefinition) TableHeaders() []string {
	return []string{"ID", "Name", "Type", "Status", "Endpoints", "Base URL", "Workspace"}
}

func (d APIDefinition) TableRow() []string {
	baseURL := d.BaseURL
	if len(baseURL) > PrintMaxURLLength {
		baseURL = baseURL[:PrintMaxURLLength] + "..."
	}
	return []string{
		d.ID.String()[:8],
		d.Name,
		string(d.Type),
		string(d.Status),
		fmt.Sprintf("%d", d.EndpointCount),
		baseURL,
		fmt.Sprintf("%d", d.WorkspaceID),
	}
}

func (d APIDefinition) String() string {
	return fmt.Sprintf("ID: %s, Name: %s, Type: %s, Status: %s, Endpoints: %d",
		d.ID.String()[:8], d.Name, d.Type, d.Status, d.EndpointCount)
}

func (d APIDefinition) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %s\n%sName:%s %s\n%sType:%s %s\n%sStatus:%s %s\n%sEndpoints:%s %d\n%sBase URL:%s %s\n%sWorkspace:%s %d\n",
		lib.Blue, lib.ResetColor, d.ID.String()[:8],
		lib.Blue, lib.ResetColor, d.Name,
		lib.Blue, lib.ResetColor, d.Type,
		lib.Blue, lib.ResetColor, d.Status,
		lib.Blue, lib.ResetColor, d.EndpointCount,
		lib.Blue, lib.ResetColor, d.BaseURL,
		lib.Blue, lib.ResetColor, d.WorkspaceID,
	)
}

type APIDefinitionFilter struct {
	Query          string              `json:"query" validate:"omitempty,ascii"`
	WorkspaceID    uint                `json:"workspace_id" validate:"omitempty,numeric"`
	ScanID         *uint               `json:"scan_id" validate:"omitempty,numeric"`
	Types          []APIDefinitionType `json:"types" validate:"omitempty"`
	Statuses       []APIDefinitionStatus `json:"statuses" validate:"omitempty"`
	AutoDiscovered *bool               `json:"auto_discovered" validate:"omitempty"`
	Pagination     Pagination          `json:"pagination"`
	SortBy         string              `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at name type status endpoint_count"`
	SortOrder      string              `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

func (d *DatabaseConnection) CreateAPIDefinition(definition *APIDefinition) (*APIDefinition, error) {
	if definition.SourceHistoryID != nil && *definition.SourceHistoryID == 0 {
		definition.SourceHistoryID = nil
	}
	if definition.ScanID != nil && *definition.ScanID == 0 {
		definition.ScanID = nil
	}
	if definition.AuthConfigID != nil && *definition.AuthConfigID == uuid.Nil {
		definition.AuthConfigID = nil
	}

	result := d.db.Create(definition)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("definition", definition).Msg("APIDefinition creation failed")
	}
	return definition, result.Error
}

func (d *DatabaseConnection) GetAPIDefinitionByID(id uuid.UUID) (*APIDefinition, error) {
	var definition APIDefinition
	err := d.db.Where("id = ?", id).First(&definition).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API definition by ID")
		return nil, err
	}
	return &definition, nil
}

func (d *DatabaseConnection) GetAPIDefinitionByIDWithEndpoints(id uuid.UUID) (*APIDefinition, error) {
	var definition APIDefinition
	err := d.db.Preload("Endpoints").Preload("Endpoints.SecuritySchemes").Preload("SecuritySchemes").
		Where("id = ?", id).First(&definition).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API definition by ID with endpoints")
		return nil, err
	}
	return &definition, nil
}

func (d *DatabaseConnection) UpdateAPIDefinition(definition *APIDefinition) (*APIDefinition, error) {
	result := d.db.Save(definition)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("definition", definition).Msg("APIDefinition update failed")
	}
	return definition, result.Error
}

func (d *DatabaseConnection) DeleteAPIDefinition(id uuid.UUID) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ScanJob{}).
			Where("api_definition_id = ? AND status IN ?", id,
				[]ScanJobStatus{ScanJobStatusPending, ScanJobStatusClaimed}).
			Updates(map[string]any{
				"status":        ScanJobStatusCancelled,
				"error_message": "API definition was deleted",
			}).Error; err != nil {
			return fmt.Errorf("cancelling pending jobs: %w", err)
		}

		if err := tx.Delete(&APIDefinition{}, "id = ?", id).Error; err != nil {
			log.Error().Err(err).Str("id", id.String()).Msg("Error deleting API definition")
			return err
		}

		return nil
	})
}

func (d *DatabaseConnection) ListAPIDefinitions(filter APIDefinitionFilter) (items []*APIDefinition, count int64, err error) {
	query := d.db.Model(&APIDefinition{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("name ILIKE ? OR base_url ILIKE ? OR source_url ILIKE ?", likeQuery, likeQuery, likeQuery)
	}

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	if filter.ScanID != nil {
		query = query.Where("scan_id = ?", *filter.ScanID)
	}

	if len(filter.Types) > 0 {
		query = query.Where("type IN ?", filter.Types)
	}

	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}

	if filter.AutoDiscovered != nil {
		query = query.Where("auto_discovered = ?", *filter.AutoDiscovered)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	validSortBy := map[string]bool{
		"id":             true,
		"created_at":     true,
		"updated_at":     true,
		"name":           true,
		"type":           true,
		"status":         true,
		"endpoint_count": true,
	}

	order := "created_at desc"
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

func (d *DatabaseConnection) GetAPIDefinitionsByScanID(scanID uint) ([]*APIDefinition, error) {
	var definitions []*APIDefinition
	err := d.db.Where("scan_id = ? AND auto_discovered = true", scanID).Find(&definitions).Error
	return definitions, err
}

func (d *DatabaseConnection) SetAPIDefinitionStatus(id uuid.UUID, status APIDefinitionStatus) error {
	return d.db.Model(&APIDefinition{}).Where("id = ?", id).Update("status", status).Error
}

func (d *DatabaseConnection) UpdateAPIDefinitionEndpointCount(id uuid.UUID) error {
	var count int64
	if err := d.db.Model(&APIEndpoint{}).Where("definition_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	return d.db.Model(&APIDefinition{}).Where("id = ?", id).Update("endpoint_count", count).Error
}

func (d *DatabaseConnection) APIDefinitionExists(id uuid.UUID) (bool, error) {
	var count int64
	err := d.db.Model(&APIDefinition{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) APIDefinitionExistsBySourceURL(workspaceID uint, sourceURL string) (bool, error) {
	var count int64
	err := d.db.Model(&APIDefinition{}).
		Where("workspace_id = ? AND source_url = ?", workspaceID, sourceURL).
		Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) GetAPIDefinitionBySourceURL(workspaceID uint, sourceURL string) (*APIDefinition, error) {
	var definition APIDefinition
	err := d.db.Where("workspace_id = ? AND source_url = ?", workspaceID, sourceURL).First(&definition).Error
	if err != nil {
		return nil, err
	}
	return &definition, nil
}

type APIDefinitionStats struct {
	TotalEndpoints     int       `json:"total_endpoints"`
	EnabledEndpoints   int       `json:"enabled_endpoints"`
	ScannedEndpoints   int       `json:"scanned_endpoints"`
	TotalIssues        int       `json:"total_issues"`
	LastScannedAt      *time.Time `json:"last_scanned_at,omitempty"`
}

func (d *DatabaseConnection) GetAPIDefinitionStats(id uuid.UUID) (*APIDefinitionStats, error) {
	type endpointAgg struct {
		Total    int64      `gorm:"column:total"`
		Enabled  int64      `gorm:"column:enabled"`
		Scanned  int64      `gorm:"column:scanned"`
		LastScan *time.Time `gorm:"column:last_scan"`
	}

	var agg endpointAgg
	if err := d.db.Model(&APIEndpoint{}).
		Select(`COUNT(*) as total,
			SUM(CASE WHEN enabled = true THEN 1 ELSE 0 END) as enabled,
			SUM(CASE WHEN last_scanned_at IS NOT NULL THEN 1 ELSE 0 END) as scanned,
			MAX(last_scanned_at) as last_scan`).
		Where("definition_id = ?", id).
		Scan(&agg).Error; err != nil {
		return nil, fmt.Errorf("aggregating endpoint stats: %w", err)
	}

	var totalIssues int64
	if err := d.db.Model(&Issue{}).Where("api_definition_id = ?", id).Count(&totalIssues).Error; err != nil {
		return nil, fmt.Errorf("counting total issues: %w", err)
	}

	return &APIDefinitionStats{
		TotalEndpoints:   int(agg.Total),
		EnabledEndpoints: int(agg.Enabled),
		ScannedEndpoints: int(agg.Scanned),
		TotalIssues:      int(totalIssues),
		LastScannedAt:    agg.LastScan,
	}, nil
}
