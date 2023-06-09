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
