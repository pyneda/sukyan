package db

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type ResponseFingerprint struct {
	StatusCode   int    `json:"status_code"`
	ResponseHash string `json:"response_hash"`
	ContentType  string `json:"content_type"`
	BodySize     int    `json:"body_size"`
}

type APIBehaviorResult struct {
	BaseUUIDModel

	ScanID      uint      `json:"scan_id" gorm:"uniqueIndex:idx_api_behavior_scan_def,priority:1;not null"`
	Scan        Scan      `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ScanJobID   *uint     `json:"scan_job_id,omitempty" gorm:"index"`
	ScanJob     *ScanJob  `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	WorkspaceID uint      `json:"workspace_id" gorm:"index;not null"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	DefinitionID   uuid.UUID         `json:"definition_id" gorm:"uniqueIndex:idx_api_behavior_scan_def,priority:2;type:uuid;not null"`
	Definition     APIDefinition     `json:"-" gorm:"foreignKey:DefinitionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	DefinitionType APIDefinitionType `json:"definition_type" gorm:"size:50;not null"`

	NotFoundFingerprints          []byte `json:"not_found_fingerprints,omitempty" gorm:"type:jsonb"`
	UnauthenticatedFingerprints   []byte `json:"unauthenticated_fingerprints,omitempty" gorm:"type:jsonb"`
	InvalidContentTypeFingerprints []byte `json:"invalid_content_type_fingerprints,omitempty" gorm:"type:jsonb"`
	MalformedBodyFingerprints     []byte `json:"malformed_body_fingerprints,omitempty" gorm:"type:jsonb"`
}

func (r *APIBehaviorResult) GetNotFoundFingerprints() []ResponseFingerprint {
	return unmarshalFingerprints(r.NotFoundFingerprints)
}

func (r *APIBehaviorResult) SetNotFoundFingerprints(fps []ResponseFingerprint) {
	r.NotFoundFingerprints = marshalFingerprints(fps)
}

func (r *APIBehaviorResult) GetUnauthenticatedFingerprints() []ResponseFingerprint {
	return unmarshalFingerprints(r.UnauthenticatedFingerprints)
}

func (r *APIBehaviorResult) SetUnauthenticatedFingerprints(fps []ResponseFingerprint) {
	r.UnauthenticatedFingerprints = marshalFingerprints(fps)
}

func (r *APIBehaviorResult) GetInvalidContentTypeFingerprints() []ResponseFingerprint {
	return unmarshalFingerprints(r.InvalidContentTypeFingerprints)
}

func (r *APIBehaviorResult) SetInvalidContentTypeFingerprints(fps []ResponseFingerprint) {
	r.InvalidContentTypeFingerprints = marshalFingerprints(fps)
}

func (r *APIBehaviorResult) GetMalformedBodyFingerprints() []ResponseFingerprint {
	return unmarshalFingerprints(r.MalformedBodyFingerprints)
}

func (r *APIBehaviorResult) SetMalformedBodyFingerprints(fps []ResponseFingerprint) {
	r.MalformedBodyFingerprints = marshalFingerprints(fps)
}

func (r *APIBehaviorResult) MatchesNotFound(statusCode int, responseHash string, bodySize int) bool {
	return matchesAnyFingerprint(r.GetNotFoundFingerprints(), statusCode, responseHash, bodySize)
}

func (r *APIBehaviorResult) MatchesUnauthenticated(statusCode int, responseHash string, bodySize int) bool {
	return matchesAnyFingerprint(r.GetUnauthenticatedFingerprints(), statusCode, responseHash, bodySize)
}

func marshalFingerprints(fps []ResponseFingerprint) []byte {
	if len(fps) == 0 {
		return nil
	}
	data, err := json.Marshal(fps)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal response fingerprints")
		return nil
	}
	return data
}

func unmarshalFingerprints(data []byte) []ResponseFingerprint {
	if len(data) == 0 {
		return nil
	}
	var fps []ResponseFingerprint
	if err := json.Unmarshal(data, &fps); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal response fingerprints")
		return nil
	}
	return fps
}

func matchesAnyFingerprint(fps []ResponseFingerprint, statusCode int, responseHash string, bodySize int) bool {
	for _, fp := range fps {
		if fp.ResponseHash != "" && fp.ResponseHash == responseHash {
			return true
		}
		if fp.StatusCode == statusCode && fp.BodySize == bodySize {
			return true
		}
	}
	return false
}

func DeduplicateFingerprints(fps []ResponseFingerprint) []ResponseFingerprint {
	seen := make(map[string]bool)
	result := make([]ResponseFingerprint, 0, len(fps))
	for _, fp := range fps {
		key := fp.ResponseHash
		if key == "" {
			key = fmt.Sprintf("%d|%s|%d", fp.StatusCode, fp.ContentType, fp.BodySize)
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, fp)
		}
	}
	return result
}

func (d *DatabaseConnection) CreateAPIBehaviorResult(result *APIBehaviorResult) (*APIBehaviorResult, error) {
	if result.ID == uuid.Nil {
		result.ID = uuid.New()
	}
	err := d.db.Create(result).Error
	if err != nil {
		log.Error().Err(err).Uint("scan_id", result.ScanID).Str("definition_id", result.DefinitionID.String()).Msg("APIBehaviorResult creation failed")
	}
	return result, err
}

func (d *DatabaseConnection) GetAPIBehaviorForDefinition(scanID uint, definitionID uuid.UUID) (*APIBehaviorResult, error) {
	var result APIBehaviorResult
	err := d.db.Where("scan_id = ? AND definition_id = ?", scanID, definitionID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *DatabaseConnection) GetAPIBehaviorResultsForScan(scanID uint) ([]*APIBehaviorResult, error) {
	var results []*APIBehaviorResult
	err := d.db.Where("scan_id = ?", scanID).Find(&results).Error
	return results, err
}

func (d *DatabaseConnection) APIBehaviorExistsForDefinition(scanID uint, definitionID uuid.UUID) (bool, error) {
	var count int64
	err := d.db.Model(&APIBehaviorResult{}).
		Where("scan_id = ? AND definition_id = ?", scanID, definitionID).
		Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) APIBehaviorJobExistsForDefinition(scanID uint, definitionID uuid.UUID) (bool, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND job_type = ? AND api_definition_id = ? AND status != ?",
			scanID, ScanJobTypeAPIBehavior, definitionID, ScanJobStatusCancelled).
		Count(&count).Error
	return count > 0, err
}
