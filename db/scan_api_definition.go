package db

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type ScanAPIDefinition struct {
	ScanID          uint      `gorm:"primaryKey;not null"`
	APIDefinitionID uuid.UUID `gorm:"primaryKey;type:uuid;not null"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
}

func (ScanAPIDefinition) TableName() string {
	return "scan_api_definitions"
}

func (d *DatabaseConnection) LinkAPIDefinitionToScan(scanID uint, definitionID uuid.UUID) error {
	link := &ScanAPIDefinition{
		ScanID:          scanID,
		APIDefinitionID: definitionID,
	}
	result := d.db.Create(link)
	if result.Error != nil {
		log.Error().Err(result.Error).
			Uint("scan_id", scanID).
			Str("definition_id", definitionID.String()).
			Msg("Failed to link API definition to scan")
	}
	return result.Error
}

func (d *DatabaseConnection) LinkAPIDefinitionsToScan(scanID uint, definitionIDs []uuid.UUID) error {
	if len(definitionIDs) == 0 {
		return nil
	}

	links := make([]ScanAPIDefinition, len(definitionIDs))
	for i, defID := range definitionIDs {
		links[i] = ScanAPIDefinition{
			ScanID:          scanID,
			APIDefinitionID: defID,
		}
	}

	result := d.db.Create(&links)
	if result.Error != nil {
		log.Error().Err(result.Error).
			Uint("scan_id", scanID).
			Int("count", len(definitionIDs)).
			Msg("Failed to link API definitions to scan")
	}
	return result.Error
}

func (d *DatabaseConnection) UnlinkAPIDefinitionFromScan(scanID uint, definitionID uuid.UUID) error {
	result := d.db.Where("scan_id = ? AND api_definition_id = ?", scanID, definitionID).
		Delete(&ScanAPIDefinition{})
	return result.Error
}

func (d *DatabaseConnection) GetAPIDefinitionsForScan(scanID uint) ([]*APIDefinition, error) {
	var definitions []*APIDefinition
	err := d.db.Joins("JOIN scan_api_definitions ON scan_api_definitions.api_definition_id = api_definitions.id").
		Where("scan_api_definitions.scan_id = ?", scanID).
		Find(&definitions).Error
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to get API definitions for scan")
		return nil, err
	}
	return definitions, nil
}

func (d *DatabaseConnection) GetLinkedAPIDefinitionIDs(scanID uint) ([]uuid.UUID, error) {
	var links []ScanAPIDefinition
	err := d.db.Where("scan_id = ?", scanID).Find(&links).Error
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(links))
	for i, link := range links {
		ids[i] = link.APIDefinitionID
	}
	return ids, nil
}

func (d *DatabaseConnection) HasLinkedAPIDefinitions(scanID uint) (bool, error) {
	var count int64
	err := d.db.Model(&ScanAPIDefinition{}).Where("scan_id = ?", scanID).Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	err = d.db.Model(&APIDefinition{}).Where("scan_id = ? AND auto_discovered = true", scanID).Count(&count).Error
	return count > 0, err
}

func (d *DatabaseConnection) GetAllAPIDefinitionsForScan(scanID uint) ([]*APIDefinition, error) {
	userDefs, err := d.GetAPIDefinitionsForScan(scanID)
	if err != nil {
		return nil, err
	}

	autoDefs, err := d.GetAPIDefinitionsByScanID(scanID)
	if err != nil {
		return nil, err
	}

	seen := make(map[uuid.UUID]bool)
	allDefs := make([]*APIDefinition, 0, len(userDefs)+len(autoDefs))

	for _, def := range userDefs {
		if !seen[def.ID] {
			seen[def.ID] = true
			allDefs = append(allDefs, def)
		}
	}

	for _, def := range autoDefs {
		if !seen[def.ID] {
			seen[def.ID] = true
			allDefs = append(allDefs, def)
		}
	}

	return allDefs, nil
}
