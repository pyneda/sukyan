package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type APIScan struct {
	BaseUUIDModel
	ScanID       uint          `gorm:"index;index:idx_api_scan_def_scan,priority:2;not null" json:"scan_id"`
	Scan         Scan          `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	DefinitionID uuid.UUID     `gorm:"type:uuid;index;index:idx_api_scan_def_scan,priority:1;not null" json:"definition_id"`
	Definition   APIDefinition `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	RunAPISpecificTests bool `gorm:"default:true" json:"run_api_specific_tests"`
	RunStandardTests    bool `gorm:"default:true" json:"run_standard_tests"`
	RunSchemaTests      bool `gorm:"default:false" json:"run_schema_tests"`
	TotalEndpoints      int  `gorm:"default:0" json:"total_endpoints"`
	CompletedEndpoints  int  `gorm:"default:0" json:"completed_endpoints"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	SelectedEndpoints []APIEndpoint `gorm:"many2many:api_scan_endpoints;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"selected_endpoints,omitempty"`
}

func (s APIScan) Progress() float64 {
	if s.TotalEndpoints == 0 {
		return 0
	}
	return float64(s.CompletedEndpoints) / float64(s.TotalEndpoints) * 100
}

func (s APIScan) TableHeaders() []string {
	return []string{"ID", "Scan ID", "Definition ID", "Progress", "Started", "Completed"}
}

func (s APIScan) TableRow() []string {
	started := "pending"
	if s.StartedAt != nil {
		started = s.StartedAt.Format(time.RFC3339)
	}

	completed := "in progress"
	if s.CompletedAt != nil {
		completed = s.CompletedAt.Format(time.RFC3339)
	}

	progress := fmt.Sprintf("%.1f%% (%d/%d)", s.Progress(), s.CompletedEndpoints, s.TotalEndpoints)

	return []string{
		s.ID.String()[:8],
		fmt.Sprintf("%d", s.ScanID),
		s.DefinitionID.String()[:8],
		progress,
		started,
		completed,
	}
}

func (s APIScan) String() string {
	return fmt.Sprintf("ID: %s, ScanID: %d, Progress: %.1f%%",
		s.ID.String()[:8], s.ScanID, s.Progress())
}

func (s APIScan) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %s\n%sScan ID:%s %d\n%sDefinition ID:%s %s\n%sProgress:%s %.1f%% (%d/%d)\n%sAPI Specific Tests:%s %t\n%sStandard Tests:%s %t\n",
		lib.Blue, lib.ResetColor, s.ID.String()[:8],
		lib.Blue, lib.ResetColor, s.ScanID,
		lib.Blue, lib.ResetColor, s.DefinitionID.String()[:8],
		lib.Blue, lib.ResetColor, s.Progress(), s.CompletedEndpoints, s.TotalEndpoints,
		lib.Blue, lib.ResetColor, s.RunAPISpecificTests,
		lib.Blue, lib.ResetColor, s.RunStandardTests,
	)
}

type APIScanFilter struct {
	ScanID       *uint      `json:"scan_id" validate:"omitempty"`
	DefinitionID *uuid.UUID `json:"definition_id" validate:"omitempty"`
	Pagination   Pagination `json:"pagination"`
	SortBy       string     `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at started_at completed_at"`
	SortOrder    string     `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

func (d *DatabaseConnection) CreateAPIScan(scan *APIScan) (*APIScan, error) {
	result := d.db.Create(scan)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("scan", scan).Msg("APIScan creation failed")
	}
	return scan, result.Error
}

func (d *DatabaseConnection) GetAPIScanByID(id uuid.UUID) (*APIScan, error) {
	var scan APIScan
	err := d.db.Where("id = ?", id).First(&scan).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API scan by ID")
		return nil, err
	}
	return &scan, nil
}

func (d *DatabaseConnection) GetAPIScanByIDWithEndpoints(id uuid.UUID) (*APIScan, error) {
	var scan APIScan
	err := d.db.Preload("SelectedEndpoints").Where("id = ?", id).First(&scan).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch API scan by ID with endpoints")
		return nil, err
	}
	return &scan, nil
}

func (d *DatabaseConnection) UpdateAPIScan(scan *APIScan) (*APIScan, error) {
	result := d.db.Save(scan)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("scan", scan).Msg("APIScan update failed")
	}
	return scan, result.Error
}

func (d *DatabaseConnection) DeleteAPIScan(id uuid.UUID) error {
	if err := d.db.Delete(&APIScan{}, "id = ?", id).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Error deleting API scan")
		return err
	}
	return nil
}

func (d *DatabaseConnection) ListAPIScans(filter APIScanFilter) (items []*APIScan, count int64, err error) {
	query := d.db.Model(&APIScan{})

	if filter.ScanID != nil {
		query = query.Where("scan_id = ?", *filter.ScanID)
	}

	if filter.DefinitionID != nil {
		query = query.Where("definition_id = ?", *filter.DefinitionID)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	validSortBy := map[string]bool{
		"id":           true,
		"created_at":   true,
		"updated_at":   true,
		"started_at":   true,
		"completed_at": true,
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

func (d *DatabaseConnection) GetAPIScansByScanID(scanID uint) ([]*APIScan, error) {
	var scans []*APIScan
	err := d.db.Where("scan_id = ?", scanID).Find(&scans).Error
	return scans, err
}

func (d *DatabaseConnection) GetAPIScanByDefinitionAndScan(definitionID uuid.UUID, scanID uint) (*APIScan, error) {
	var scan APIScan
	err := d.db.Where("definition_id = ? AND scan_id = ?", definitionID, scanID).First(&scan).Error
	if err != nil {
		return nil, err
	}
	return &scan, nil
}

func (d *DatabaseConnection) MarkAPIScanStarted(id uuid.UUID) error {
	now := time.Now()
	return d.db.Model(&APIScan{}).Where("id = ?", id).Update("started_at", now).Error
}

func (d *DatabaseConnection) MarkAPIScanCompleted(id uuid.UUID) error {
	now := time.Now()
	return d.db.Model(&APIScan{}).Where("id = ?", id).Update("completed_at", now).Error
}

func (d *DatabaseConnection) IncrementAPIScanCompletedEndpoints(id uuid.UUID) error {
	return d.db.Model(&APIScan{}).Where("id = ?", id).
		Update("completed_endpoints", gorm.Expr("completed_endpoints + 1")).Error
}

func (d *DatabaseConnection) SetAPIScanSelectedEndpoints(id uuid.UUID, endpointIDs []uuid.UUID) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		var scan APIScan
		if err := tx.Where("id = ?", id).First(&scan).Error; err != nil {
			return err
		}

		var endpoints []APIEndpoint
		if len(endpointIDs) > 0 {
			if err := tx.Where("id IN ? AND definition_id = ?", endpointIDs, scan.DefinitionID).Find(&endpoints).Error; err != nil {
				return err
			}
			if len(endpoints) != len(endpointIDs) {
				return fmt.Errorf("some endpoint IDs do not belong to definition %s", scan.DefinitionID)
			}
		}

		if err := tx.Model(&scan).Association("SelectedEndpoints").Replace(endpoints); err != nil {
			return err
		}

		return tx.Model(&APIScan{}).Where("id = ?", id).Update("total_endpoints", len(endpoints)).Error
	})
}

func (d *DatabaseConnection) GetAPIScanSelectedEndpointIDs(id uuid.UUID) ([]uuid.UUID, error) {
	var endpointIDs []uuid.UUID
	err := d.db.Table("api_scan_endpoints").
		Select("api_endpoint_id").
		Where("api_scan_id = ?", id).
		Pluck("api_endpoint_id", &endpointIDs).Error
	return endpointIDs, err
}
