package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Workspace is used to group projects
type Workspace struct {
	gorm.Model
	Code        string
	Title       string
	Description string
}

// ListWorkspaces Lists workspaces
func (d *DatabaseConnection) ListWorkspaces() ([]*Workspace, error) {
	var workspaces []*Workspace
	err := d.db.Find(&workspaces).Error
	return workspaces, err
}

// CreateWorkspace saves a workspace to the database
func (d *DatabaseConnection) CreateWorkspace(workspace Workspace) (Workspace, error) {
	result := d.db.Create(&workspace)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("workspace", workspace).Msg("Workspace creation failed")
	}
	return workspace, result.Error
}
