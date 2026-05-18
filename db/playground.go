package db

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

// PlaygroundCollection represents a collection of playground sessions.
type PlaygroundCollection struct {
	BaseModel
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Sessions    []PlaygroundSession `gorm:"foreignKey:CollectionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	WorkspaceID uint                `json:"workspace_id" gorm:"index"`
	Workspace   Workspace           `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// PlaygroundSessionType represents the type of a playground session.
type PlaygroundSessionType string

const (
	ManualType   PlaygroundSessionType = "manual"
	FuzzType     PlaygroundSessionType = "fuzz"
	WsManualType PlaygroundSessionType = "ws_manual"
	WsFuzzType   PlaygroundSessionType = "ws_fuzz"
)

// PlaygroundSession represents a playground session.
type PlaygroundSession struct {
	BaseModel
	Name              string                `json:"name"`
	Type              PlaygroundSessionType `json:"type"`
	OriginalRequest   *History              `json:"-" gorm:"foreignKey:OriginalRequestID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OriginalRequestID *uint                 `json:"original_request_id"`
	InitialRawRequest string                `json:"initial_raw_request"`
	// Task              Task                 `json:"-" gorm:"foreignKey:TaskID"`
	// TaskID            *uint                `json:"task_id"`
	CollectionID uint                 `json:"collection_id"`
	Collection   PlaygroundCollection `json:"-" gorm:"foreignKey:CollectionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	WorkspaceID  uint                 `json:"workspace_id" gorm:"index"`
	Workspace    Workspace            `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Histories    []History            `gorm:"foreignKey:PlaygroundSessionID" json:"-"`
	// FuzzerConfig is the live, editable fuzzer config for sessions of type
	// "fuzz". Nullable: sessions of other types ignore it; new fuzz sessions
	// persist a config on first save. Snapshotted into PlaygroundFuzzRun on
	// each launch.
	FuzzerConfig json.RawMessage `json:"fuzzer_config,omitempty" gorm:"type:jsonb"`
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

// FindOrCreatePlaygroundCollection finds a PlaygroundCollection by workspace ID and name,
// creating it if it does not already exist. The lookup-and-insert is performed atomically
// by GORM's FirstOrCreate to avoid duplicate rows under concurrent callers.
func (d *DatabaseConnection) FindOrCreatePlaygroundCollection(workspaceID uint, name string) (*PlaygroundCollection, error) {
	coll := PlaygroundCollection{WorkspaceID: workspaceID, Name: name}
	err := d.db.Where(PlaygroundCollection{WorkspaceID: workspaceID, Name: name}).FirstOrCreate(&coll).Error
	return &coll, err
}

// UpdatePlaygroundCollection updates an existing PlaygroundCollection record.
func (d *DatabaseConnection) UpdatePlaygroundCollection(id uint, collection *PlaygroundCollection) error {
	return d.db.Model(&PlaygroundCollection{}).Where("id = ?", id).Updates(collection).Error
}

// UpdatePlaygroundSession updates an existing PlaygroundSession record.
func (d *DatabaseConnection) UpdatePlaygroundSession(id uint, session *PlaygroundSession) error {
	return d.db.Model(&PlaygroundSession{}).Where("id = ?", id).Updates(session).Error
}

// UpdatePlaygroundSessionFuzzerConfig persists the fuzzer config blob for a
// session. Returns ErrRecordNotFound if the session doesn't exist. Allows
// nil json.RawMessage to clear the field — pass an empty config explicitly
// to keep the column non-null.
func (d *DatabaseConnection) UpdatePlaygroundSessionFuzzerConfig(id uint, cfg json.RawMessage) error {
	res := d.db.Model(&PlaygroundSession{}).Where("id = ?", id).Update("fuzzer_config", cfg)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("session %d not found", id)
	}
	return nil
}

// CreatePlaygroundSession creates a new PlaygroundSession record.
func (d *DatabaseConnection) CreatePlaygroundSession(session *PlaygroundSession) error {
	return d.db.Create(session).Error
}

// DeletePlaygroundSession hard-deletes the row so the DB-level ON DELETE CASCADE
// fires on playground_ws_sessions and playground_ws_runs. Soft delete would not
// trigger the cascade and would leave child rows orphaned.
func (d *DatabaseConnection) DeletePlaygroundSession(id uint) error {
	return d.db.Unscoped().Delete(&PlaygroundSession{}, id).Error
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

// GetPlaygroundCollectionByID retrieves a PlaygroundCollection by its ID.
func (d *DatabaseConnection) GetPlaygroundCollectionByID(id uint) (*PlaygroundCollection, error) {
	var collection PlaygroundCollection
	err := d.db.First(&collection, id).Error
	if err != nil {
		log.Error().Err(err).Uint("id", id).Msg("Failed to get playground collection by ID")
		return nil, err
	}
	return &collection, nil
}

// GetPlaygroundSessionByID retrieves a PlaygroundSession by its ID.
func (d *DatabaseConnection) GetPlaygroundSessionByID(id uint) (*PlaygroundSession, error) {
	var session PlaygroundSession
	err := d.db.First(&session, id).Error
	if err != nil {
		log.Error().Err(err).Uint("id", id).Msg("Failed to get playground session by ID")
		return nil, err
	}
	return &session, nil
}
