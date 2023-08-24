package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Workspace is used to group projects
type Workspace struct {
	BaseModel
	Code        string `gorm:"index,unique" json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GetWorkspaceByID gets a workspace by ID
func (d *DatabaseConnection) GetWorkspaceByID(id uint) (*Workspace, error) {
	var workspace Workspace
	if err := d.db.Where("id = ?", id).First(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to fetch workspace by ID")
		return nil, err
	}
	return &workspace, nil
}

// GetWorkspaceByCode gets a workspace by code
func (d *DatabaseConnection) GetWorkspaceByCode(code string) (*Workspace, error) {
	var workspace Workspace
	if err := d.db.Where("code = ?", code).First(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("code", code).Msg("Unable to fetch workspace by code")
		return nil, err
	}
	return &workspace, nil
}

// WorkspaceExists checks if a workspace exists
func (d *DatabaseConnection) WorkspaceExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&Workspace{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type WorkspaceFilters struct {
	Query string `json:"query" validate:"omitempty,dive,ascii"`
}

// ListWorkspaces Lists workspaces
func (d *DatabaseConnection) ListWorkspaces(filters WorkspaceFilters) (items []*Workspace, count int64, err error) {
	query := d.db
	if filters.Query != "" {
		likeQuery := "%" + filters.Query + "%"
		query = query.Where("code LIKE ? OR title LIKE ? OR description LIKE ?", likeQuery, likeQuery, likeQuery)
	}

	result := query.Find(&items).Count(&count)
	if result.Error != nil {
		err = result.Error
	}
	return items, count, err
}

func (d *DatabaseConnection) CreateDefaultWorkspace() (*Workspace, error) {
	workspace := Workspace{
		Code:        "default",
		Title:       "Default workspace",
		Description: "Default workspace",
	}
	return d.GetOrCreateWorkspace(&workspace)
}

// CreateWorkspace saves a workspace to the database
func (d *DatabaseConnection) CreateWorkspace(workspace *Workspace) (*Workspace, error) {
	result := d.db.Create(&workspace)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("workspace", workspace).Msg("Workspace creation failed")
	}
	return workspace, result.Error
}

// GetOrCreateWorkspace gets a workspace with the given code, or creates it if it doesn't exist
func (d *DatabaseConnection) GetOrCreateWorkspace(workspace *Workspace) (*Workspace, error) {

	var existingWorkspace Workspace
	if err := d.db.Where("code = ?", workspace.Code).First(&existingWorkspace).Error; err == nil {
		return &existingWorkspace, nil
	} else if err != gorm.ErrRecordNotFound {
		log.Error().Err(err).Interface("workspace", workspace).Msg("Error checking workspace by code")
		return nil, err
	}

	if err := d.db.Create(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("workspace", workspace).Msg("Workspace creation failed")
		return nil, err
	}

	return workspace, nil
}

// DeleteWorkspace deletes a workspace by ID
func (d *DatabaseConnection) DeleteWorkspace(id uint) error {
	var workspace Workspace
	if err := d.db.Where("id = ?", id).Delete(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to delete workspace by ID")
		return err
	}
	return nil
}

// UpdateWorkspace updates a workspace by its ID with the provided fields
func (d *DatabaseConnection) UpdateWorkspace(id uint, updatedWorkspace *Workspace) error {
	var workspace Workspace

	// Fetch the workspace by ID
	if err := d.db.Where("id = ?", id).First(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to fetch workspace by ID for updating")
		return err
	}

	// Update the relevant fields
	if updatedWorkspace.Code != "" {
		workspace.Code = updatedWorkspace.Code
	}

	if updatedWorkspace.Title != "" {
		workspace.Title = updatedWorkspace.Title
	}

	if updatedWorkspace.Description != "" {
		workspace.Description = updatedWorkspace.Description
	}

	// Save the updated workspace
	if err := d.db.Save(&workspace).Error; err != nil {
		log.Error().Err(err).Interface("workspace", workspace).Msg("Unable to update workspace")
		return err
	}
	return nil
}
