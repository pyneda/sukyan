package db

import (
	"github.com/rs/zerolog/log"
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

// ListWorkspaces Lists workspaces
func (d *DatabaseConnection) ListWorkspaces() (items []*Workspace, count int64, err error) {
	result := d.db.Find(&items).Count(&count)
	if result.Error != nil {
		err = result.Error
	}
	if count == 0 {
		log.Info().Msg("No workspaces found, creating default")

		workspace, err := d.CreateDefaultWorkspace()
		if err != nil {
			log.Error().Err(err).Msg("Error creating default workspace")
		} else {
			items = append(items, workspace)
			count = 1
		}
	}
	return items, count, err
}

func (d *DatabaseConnection) CreateDefaultWorkspace() (*Workspace, error) {
	workspace := Workspace{
		Code:        "default",
		Title:       "Default workspace",
		Description: "Default workspace",
	}
	return d.CreateWorkspace(&workspace)
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
	result := d.db.FirstOrCreate(workspace, Workspace{Code: workspace.Code})
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("workspace", workspace).Msg("Workspace get or create failed")
	}
	return workspace, result.Error
}
