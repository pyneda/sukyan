package db

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SiteBehaviorResult struct {
	BaseUUIDModel

	ScanID      uint      `json:"scan_id" gorm:"uniqueIndex:idx_site_behavior_scan_url;not null"`
	Scan        Scan      `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanJobID   *uint     `json:"scan_job_id,omitempty" gorm:"index"`
	ScanJob     *ScanJob  `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	WorkspaceID uint      `json:"workspace_id" gorm:"index;not null"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	BaseURL             string `json:"base_url" gorm:"uniqueIndex:idx_site_behavior_scan_url;type:text;not null"`
	NotFoundReturns404  bool   `json:"not_found_returns_404"`
	NotFoundChanges     bool   `json:"not_found_changes"`
	NotFoundCommonHash  string `json:"not_found_common_hash,omitempty" gorm:"size:255"`
	NotFoundStatusCode  int    `json:"not_found_status_code,omitempty"`

	BaseURLSampleID *uint    `json:"base_url_sample_id,omitempty" gorm:"index"`
	BaseURLSample   *History `json:"-" gorm:"foreignKey:BaseURLSampleID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	NotFoundSamples []SiteBehaviorNotFoundSample `json:"-" gorm:"foreignKey:SiteBehaviorResultID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type SiteBehaviorNotFoundSample struct {
	BaseModel
	SiteBehaviorResultID uuid.UUID          `json:"site_behavior_result_id" gorm:"type:uuid;not null;index"`
	SiteBehaviorResult   SiteBehaviorResult `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	HistoryID            uint               `json:"history_id" gorm:"not null;index"`
	History              History            `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (d *DatabaseConnection) CreateSiteBehaviorResult(result *SiteBehaviorResult) (*SiteBehaviorResult, error) {
	if result.ID == uuid.Nil {
		result.ID = uuid.New()
	}
	err := d.db.Create(result).Error
	if err != nil {
		log.Error().Err(err).Interface("result", result).Msg("SiteBehaviorResult creation failed")
	}
	return result, err
}

func (d *DatabaseConnection) GetSiteBehaviorResultsForScan(scanID uint) ([]*SiteBehaviorResult, error) {
	var results []*SiteBehaviorResult
	err := d.db.Where("scan_id = ?", scanID).Find(&results).Error
	return results, err
}

func (d *DatabaseConnection) GetSiteBehaviorForBaseURL(scanID uint, baseURL string) (*SiteBehaviorResult, error) {
	var result SiteBehaviorResult
	err := d.db.Where("scan_id = ? AND base_url = ?", scanID, baseURL).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *DatabaseConnection) SiteBehaviorExistsForURL(scanID uint, baseURL string) (bool, error) {
	var count int64
	err := d.db.Model(&SiteBehaviorResult{}).
		Where("scan_id = ? AND base_url = ?", scanID, baseURL).
		Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) SiteBehaviorJobExistsForURL(scanID uint, url string) (bool, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND job_type = ? AND url = ? AND status != ?",
			scanID, ScanJobTypeSiteBehavior, url, ScanJobStatusCancelled).
		Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) GetSiteBehaviorWithSamples(scanID uint, baseURL string) (*SiteBehaviorResult, error) {
	var result SiteBehaviorResult
	err := d.db.
		Where("scan_id = ? AND base_url = ?", scanID, baseURL).
		Preload("BaseURLSample").
		Preload("NotFoundSamples.History").
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *DatabaseConnection) CreateSiteBehaviorNotFoundSample(sample *SiteBehaviorNotFoundSample) error {
	return d.db.Create(sample).Error
}
