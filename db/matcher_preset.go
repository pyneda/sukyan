package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// isUniqueViolation returns true when the underlying driver error came from a
// Postgres unique-constraint failure. We string-match instead of importing
// pgx directly so this helper stays usable from db/ without pulling driver
// internals into a layer that should remain ORM-agnostic.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// pgx wraps the SQLSTATE in the message as "SQLSTATE 23505" or
	// "(SQLSTATE 23505)". gorm doesn't unwrap to a typed error, so a string
	// check is the most stable signal.
	if strings.Contains(msg, "SQLSTATE 23505") {
		return true
	}
	// Older drivers / sqlite (used by some tests) surface a different wording.
	return strings.Contains(strings.ToLower(msg), "unique constraint")
}

// MatcherPresetDomain narrows which fuzzer surface a preset applies to.
// Presets are deliberately scoped per domain — the field/operator taxonomies
// diverge enough between HTTP and WS that mixing them in a single picker
// would mislead users.
type MatcherPresetDomain string

const (
	MatcherPresetDomainHTTPFuzz MatcherPresetDomain = "http_fuzz"
	MatcherPresetDomainWsFuzz   MatcherPresetDomain = "ws_fuzz"
)

// MatcherPreset persists a named matcher set per workspace + domain so
// researchers can switch between common views (e.g. "auth failures",
// "5xx + body contains stack") without rebuilding rules each time. The
// matcher_set blob is the same shape the /match endpoint already accepts;
// applying a preset is a pure UI operation (no server-side execution).
type MatcherPreset struct {
	BaseModel
	WorkspaceID uint                `json:"workspace_id" gorm:"index:idx_matcher_preset_ws_domain_name,priority:1;not null"`
	Workspace   Workspace           `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Domain      MatcherPresetDomain `json:"domain" gorm:"type:varchar(32);index:idx_matcher_preset_ws_domain_name,priority:2;not null"`
	Name        string              `json:"name" gorm:"type:varchar(128);index:idx_matcher_preset_ws_domain_name,priority:3,unique;not null"`
	MatcherSet  json.RawMessage     `json:"matcher_set" gorm:"type:jsonb;not null"`
}

// MatcherPresetFilters narrows ListMatcherPresets to a single workspace and
// (optionally) a single domain. Presets are workspace-scoped, never global.
type MatcherPresetFilters struct {
	WorkspaceID uint                `json:"workspace_id" validate:"required,min=1"`
	Domain      MatcherPresetDomain `json:"domain" validate:"omitempty,oneof=http_fuzz ws_fuzz"`
}

// ErrMatcherPresetNameTaken indicates a name collision in the (workspace,
// domain) scope. The handler converts this into a 409 so the UI can show a
// targeted message instead of a generic 500.
var ErrMatcherPresetNameTaken = errors.New("matcher preset name already exists in this workspace + domain")

func (c *DatabaseConnection) ListMatcherPresets(filters MatcherPresetFilters) ([]MatcherPreset, error) {
	var presets []MatcherPreset
	q := c.db.Where("workspace_id = ?", filters.WorkspaceID)
	if filters.Domain != "" {
		q = q.Where("domain = ?", filters.Domain)
	}
	if err := q.Order("name ASC").Find(&presets).Error; err != nil {
		return nil, fmt.Errorf("list matcher presets: %w", err)
	}
	return presets, nil
}

func (c *DatabaseConnection) GetMatcherPreset(id uint) (*MatcherPreset, error) {
	var p MatcherPreset
	if err := c.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateMatcherPreset inserts the preset, mapping unique-constraint
// violations to ErrMatcherPresetNameTaken so the API layer can return a
// stable 409 sentinel.
func (c *DatabaseConnection) CreateMatcherPreset(p *MatcherPreset) error {
	if err := c.db.Create(p).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrMatcherPresetNameTaken
		}
		return err
	}
	return nil
}

// UpdateMatcherPreset rewrites the name and/or matcher_set on an existing
// row. Workspace and domain are immutable — moving a preset between
// workspaces is out of scope and creating a fresh one is easier than
// re-keying.
func (c *DatabaseConnection) UpdateMatcherPreset(id uint, name string, matcherSet json.RawMessage) error {
	res := c.db.Model(&MatcherPreset{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"name":        name,
			"matcher_set": matcherSet,
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return ErrMatcherPresetNameTaken
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (c *DatabaseConnection) DeleteMatcherPreset(id uint) error {
	res := c.db.Delete(&MatcherPreset{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
