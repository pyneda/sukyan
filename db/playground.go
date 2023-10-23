package db

import (
	"fmt"
	"github.com/rs/zerolog/log"
)

// PlaygroundCollection represents a collection of playground sessions.
type PlaygroundCollection struct {
	BaseModel
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Sessions    []PlaygroundSession `json:"-" gorm:"foreignKey:CollectionID"`
	WorkspaceID uint                `json:"workspace_id" gorm:"index"`
	Workspace   Workspace           `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// PlaygroundSessionType represents the type of a playground session.
type PlaygroundSessionType string

const (
	ManualType PlaygroundSessionType = "manual"
	FuzzType   PlaygroundSessionType = "fuzz"
)

// PlaygroundSession represents a playground session.
type PlaygroundSession struct {
	BaseModel
	Name string                `json:"name"`
	Type PlaygroundSessionType `json:"type"`
	// OriginalRequest   History               `json:"-" gorm:"foreignKey:OriginalRequestID"`
	OriginalRequestID *uint `json:"original_request_id"`
	// Task              Task                 `json:"-" gorm:"foreignKey:TaskID"`
	// TaskID            *uint                `json:"task_id"`
	CollectionID uint                 `json:"collection_id"`
	Collection   PlaygroundCollection `json:"-" gorm:"foreignKey:CollectionID"`
	WorkspaceID  uint                 `json:"workspace_id" gorm:"index"`
	Workspace    Workspace            `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Histories    []History            `gorm:"foreignKey:PlaygroundSessionID" json:"-"`
}

// PlaygroundCollectionFilters contains filters for listing PlaygroundCollections.
type PlaygroundCollectionFilters struct {
	Query       string `json:"query"`
	SortBy      string `json:"sort_by" validate:"omitempty,oneof=id name description created_at updated_at"`
	SortOrder   string `json:"sort_order" validate:"omitempty,oneof=asc desc"`
	WorkspaceID uint   `json:"workspace_id" validate:"omitempty,numeric"`
	Pagination
}

// ListPlaygroundCollections retrieves a list of PlaygroundCollections with filters, sorting, and pagination.
func (d *DatabaseConnection) ListPlaygroundCollections(filters PlaygroundCollectionFilters) ([]*PlaygroundCollection, int64, error) {
	query := d.db.Model(&PlaygroundCollection{})

	if filters.Query != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?", "%"+filters.Query+"%", "%"+filters.Query+"%")
	}

	sortColumn := "id"
	sortOrder := "asc"

	if filters.SortBy != "" {
		sortColumn = filters.SortBy
	}

	if filters.SortOrder != "" {
		sortOrder = filters.SortOrder
	}

	if filters.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filters.WorkspaceID)
	}

	query = query.Order(fmt.Sprintf("%s %s", sortColumn, sortOrder))
	if filters.Page > 0 && filters.PageSize > 0 {
		query = query.Scopes(Paginate(&filters.Pagination))
	}

	var collections []*PlaygroundCollection
	var count int64
	err := query.Find(&collections).Count(&count).Error
	return collections, count, err
}

// PlaygroundSessionFilters contains filters for listing PlaygroundSessions.
type PlaygroundSessionFilters struct {
	Query             string                `json:"query"`
	Type              PlaygroundSessionType `json:"type"`
	OriginalRequestID uint                  `json:"original_request_id"`
	// TaskID            uint                  `json:"task_id"`
	CollectionID uint   `json:"collection_id"`
	WorkspaceID  uint   `json:"workspace_id"`
	SortBy       string `json:"sort_by" validate:"omitempty,oneof=id name type workspace_id collection_id created_at updated_at"`
	SortOrder    string `json:"sort_order" validate:"omitempty,oneof=asc desc"`
	Pagination
}

// ListPlaygroundSessions retrieves a list of PlaygroundSessions with filters, sorting, and pagination.
func (d *DatabaseConnection) ListPlaygroundSessions(filters PlaygroundSessionFilters) ([]*PlaygroundSession, int64, error) {
	query := d.db.Model(&PlaygroundSession{})

	if filters.Type != "" {
		query = query.Where("type = ?", filters.Type)
	}
	if filters.OriginalRequestID > 0 {
		query = query.Where("original_request_id = ?", filters.OriginalRequestID)
	}
	// if filters.TaskID > 0 {
	// 	query = query.Where("task_id = ?", filters.TaskID)
	// }
	if filters.CollectionID > 0 {
		query = query.Where("collection_id = ?", filters.CollectionID)
	}
	if filters.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filters.WorkspaceID)
	}

	if filters.Query != "" {
		query = query.Where("name ILIKE ?", "%"+filters.Query+"%")
	}

	sortColumn := "id"
	sortOrder := "asc"

	if filters.SortBy != "" {
		sortColumn = filters.SortBy
	}
	if filters.SortOrder != "" {
		sortOrder = filters.SortOrder
	}

	query = query.Order(fmt.Sprintf("%s %s", sortColumn, sortOrder))

	if filters.Page > 0 && filters.PageSize > 0 {
		query = query.Scopes(Paginate(&filters.Pagination))
	}

	var sessions []*PlaygroundSession
	var count int64
	err := query.Find(&sessions).Count(&count).Error
	return sessions, count, err
}

// GetPlaygroundCollection retrieves a single PlaygroundCollection by its ID.
func (d *DatabaseConnection) GetPlaygroundCollection(id uint) (*PlaygroundCollection, error) {
	var collection PlaygroundCollection
	err := d.db.First(&collection, id).Error
	return &collection, err
}

// GetPlaygroundSession retrieves a single PlaygroundSession by its ID.
func (d *DatabaseConnection) GetPlaygroundSession(id uint) (*PlaygroundSession, error) {
	var session PlaygroundSession
	err := d.db.First(&session, id).Error
	return &session, err
}

// CreatePlaygroundCollection creates a new PlaygroundCollection record.
func (d *DatabaseConnection) CreatePlaygroundCollection(collection *PlaygroundCollection) error {
	return d.db.Create(collection).Error
}

// UpdatePlaygroundCollection updates an existing PlaygroundCollection record.
func (d *DatabaseConnection) UpdatePlaygroundCollection(id uint, collection *PlaygroundCollection) error {
	return d.db.Model(&PlaygroundCollection{}).Where("id = ?", id).Updates(collection).Error
}

// UpdatePlaygroundSession updates an existing PlaygroundSession record.
func (d *DatabaseConnection) UpdatePlaygroundSession(id uint, session *PlaygroundSession) error {
	return d.db.Model(&PlaygroundSession{}).Where("id = ?", id).Updates(session).Error
}

// CreatePlaygroundSession creates a new PlaygroundSession record.
func (d *DatabaseConnection) CreatePlaygroundSession(session *PlaygroundSession) error {
	return d.db.Create(session).Error
}

func (d *DatabaseConnection) InitializeWorkspacePlayground(workspaceID uint) error {
	collection := PlaygroundCollection{
		Name:        "Default collection",
		Description: "Default playground collection",
		WorkspaceID: workspaceID,
	}
	err := d.CreatePlaygroundCollection(&collection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create playground collection")
		return err
	}

	session := PlaygroundSession{
		Name:         "Default session",
		Type:         ManualType,
		WorkspaceID:  workspaceID,
		CollectionID: collection.ID,
	}
	err = d.CreatePlaygroundSession(&session)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create playground session")
		return err
	}
	log.Info().Uint("workspace", workspaceID).Msg("Initialized workspace playground")
	return nil
}
