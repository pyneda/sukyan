package db

import (
	"fmt"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/rs/zerolog/log"
)

type BrowserActionScope string

const (
	BrowserActionScopeGlobal    BrowserActionScope = "global"
	BrowserActionScopeWorkspace BrowserActionScope = "workspace"
)

type StoredBrowserActions struct {
	BaseModel
	Title       string             `json:"title" gorm:"index"`
	Actions     []actions.Action   `json:"actions" gorm:"serializer:json"`
	Scope       BrowserActionScope `json:"scope" gorm:"index"`
	Workspace   Workspace          `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID *uint              `json:"workspace_id" gorm:"index"`
}

// CreateStoredBrowserActions creates a new StoredBrowserActions record
func (d *DatabaseConnection) CreateStoredBrowserActions(sba *StoredBrowserActions) (*StoredBrowserActions, error) {
	result := d.db.Create(sba)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("stored_browser_actions", sba).Msg("StoredBrowserActions creation failed")
	}
	return sba, result.Error
}

// GetStoredBrowserActionsByID retrieves a StoredBrowserActions by its ID
func (d *DatabaseConnection) GetStoredBrowserActionsByID(id uint) (*StoredBrowserActions, error) {
	var sba StoredBrowserActions
	if err := d.db.Where("id = ?", id).First(&sba).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to fetch StoredBrowserActions by ID")
		return nil, err
	}
	return &sba, nil
}

// UpdateStoredBrowserActions updates an existing StoredBrowserActions record
func (d *DatabaseConnection) UpdateStoredBrowserActions(id uint, sba *StoredBrowserActions) (*StoredBrowserActions, error) {
	result := d.db.Model(&StoredBrowserActions{}).Where("id = ?", id).Updates(sba)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("stored_browser_actions", sba).Msg("StoredBrowserActions update failed")
	}
	return sba, result.Error
}

// DeleteStoredBrowserActions deletes a StoredBrowserActions record
func (d *DatabaseConnection) DeleteStoredBrowserActions(id uint) error {
	if err := d.db.Delete(&StoredBrowserActions{}, id).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Error deleting StoredBrowserActions")
		return err
	}
	return nil
}

// ListStoredBrowserActions retrieves a list of StoredBrowserActions based on the provided filter
func (d *DatabaseConnection) ListStoredBrowserActions(filter StoredBrowserActionsFilter) (items []*StoredBrowserActions, count int64, err error) {
	query := d.db.Model(&StoredBrowserActions{}).Scopes(Paginate(&filter.Pagination))

	if filter.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *filter.WorkspaceID)
	}

	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}

	if filter.Query != "" {
		query = query.Where("title LIKE ?", "%"+filter.Query+"%")
	}

	err = query.Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	query.Count(&count)

	return items, count, nil
}

// StoredBrowserActionsFilter defines the filter for listing StoredBrowserActions
type StoredBrowserActionsFilter struct {
	Query       string             `json:"query" validate:"omitempty,dive,ascii"`
	Scope       BrowserActionScope `json:"scope" validate:"omitempty,oneof=global workspace"`
	WorkspaceID *uint              `json:"workspace_id" validate:"omitempty,numeric"`
	Pagination  Pagination         `json:"pagination"`
}

// TableHeaders returns the headers for the StoredBrowserActions table
func (sba StoredBrowserActions) TableHeaders() []string {
	return []string{"ID", "Title", "Scope", "WorkspaceID", "Actions Count", "Created At", "Updated At"}
}

// TableRow returns a row representation of StoredBrowserActions for display in a table
func (sba StoredBrowserActions) TableRow() []string {
	workspaceID := "N/A"
	if sba.WorkspaceID != nil {
		workspaceID = fmt.Sprintf("%d", *sba.WorkspaceID)
	}
	return []string{
		fmt.Sprintf("%d", sba.ID),
		sba.Title,
		string(sba.Scope),
		workspaceID,
		fmt.Sprintf("%d", len(sba.Actions)),
		sba.CreatedAt.Format(time.RFC3339),
		sba.UpdatedAt.Format(time.RFC3339),
	}
}

// String provides a basic textual representation of the StoredBrowserActions
func (sba StoredBrowserActions) String() string {
	workspaceID := "N/A"
	if sba.WorkspaceID != nil {
		workspaceID = fmt.Sprintf("%d", *sba.WorkspaceID)
	}
	return fmt.Sprintf("ID: %d, Title: %s, Scope: %s, WorkspaceID: %s, Actions Count: %d",
		sba.ID, sba.Title, sba.Scope, workspaceID, len(sba.Actions))
}

// Pretty provides a more formatted, user-friendly representation of the StoredBrowserActions
func (sba StoredBrowserActions) Pretty() string {
	workspaceID := "N/A"
	if sba.WorkspaceID != nil {
		workspaceID = fmt.Sprintf("%d", *sba.WorkspaceID)
	}
	return fmt.Sprintf(
		"%sID:%s %d\n%sTitle:%s %s\n%sScope:%s %s\n%sWorkspaceID:%s %s\n%sActions Count:%s %d\n%sCreated At:%s %s\n%sUpdated At:%s %s\n",
		lib.Blue, lib.ResetColor, sba.ID,
		lib.Blue, lib.ResetColor, sba.Title,
		lib.Blue, lib.ResetColor, sba.Scope,
		lib.Blue, lib.ResetColor, workspaceID,
		lib.Blue, lib.ResetColor, len(sba.Actions),
		lib.Blue, lib.ResetColor, sba.CreatedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, sba.UpdatedAt.Format(time.RFC3339))
}
