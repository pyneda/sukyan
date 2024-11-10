package db

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// WorkspaceCookie represents a single cookie stored for a workspace
type WorkspaceCookie struct {
	BaseUUIDModel
	WorkspaceID *uint     `json:"workspace_id" gorm:"index"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Name        string    `json:"name" gorm:"index"`
	Value       string    `json:"value"`
	Domain      string    `json:"domain" gorm:"index"`
	Path        string    `json:"path"`
	Expires     time.Time `json:"expires"`
	MaxAge      int       `json:"max_age"`
	Secure      bool      `json:"secure"`
	HttpOnly    bool      `json:"http_only"`
	SameSite    string    `json:"same_site"`
}

func (c WorkspaceCookie) TableHeaders() []string {
	return []string{"ID", "WorkspaceID", "Name", "Domain", "Path", "Expires"}
}

func (c WorkspaceCookie) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", c.ID),
		formatUintPointer(c.WorkspaceID),
		c.Name,
		c.Domain,
		c.Path,
		c.Expires.Format(time.RFC3339),
	}
}

func (c WorkspaceCookie) ToHTTPCookie() *http.Cookie {
	cookie := &http.Cookie{
		Name:     c.Name,
		Value:    c.Value,
		Path:     c.Path,
		Domain:   c.Domain,
		Expires:  c.Expires,
		MaxAge:   c.MaxAge,
		Secure:   c.Secure,
		HttpOnly: c.HttpOnly,
	}

	switch c.SameSite {
	case "Strict":
		cookie.SameSite = http.SameSiteStrictMode
	case "Lax":
		cookie.SameSite = http.SameSiteLaxMode
	case "None":
		cookie.SameSite = http.SameSiteNoneMode
	}

	return cookie
}

func (d *DatabaseConnection) CreateWorkspaceCookie(cookie *WorkspaceCookie) error {
	result := d.db.Create(cookie)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("cookie", cookie).Msg("Failed to create workspace cookie")
	}
	return result.Error
}

func (d *DatabaseConnection) GetWorkspaceCookie(id uuid.UUID) (*WorkspaceCookie, error) {
	var cookie WorkspaceCookie
	err := d.db.First(&cookie, id).Error
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to get workspace cookie")
		return nil, err
	}
	return &cookie, nil
}

func (d *DatabaseConnection) UpdateWorkspaceCookie(cookie *WorkspaceCookie) error {
	result := d.db.Save(cookie)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("cookie", cookie).Msg("Failed to update workspace cookie")
	}
	return result.Error
}

func (d *DatabaseConnection) DeleteWorkspaceCookie(id uuid.UUID) error {
	result := d.db.Delete(&WorkspaceCookie{}, id)
	if result.Error != nil {
		log.Error().Err(result.Error).Str("id", id.String()).Msg("Failed to delete workspace cookie")
	}
	return result.Error
}

type WorkspaceCookieFilter struct {
	Pagination
	WorkspaceID uint   `json:"workspace_id" validate:"required"`
	Domain      string `json:"domain"`
	Name        string `json:"name"`
}

func (d *DatabaseConnection) ListWorkspaceCookies(filter WorkspaceCookieFilter) ([]WorkspaceCookie, int64, error) {
	query := d.db.Model(&WorkspaceCookie{})

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.Domain != "" {
		query = query.Where("domain = ?", filter.Domain)
	}
	if filter.Name != "" {
		query = query.Where("name = ?", filter.Name)
	}

	var cookies []WorkspaceCookie
	var count int64

	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count workspace cookies")
		return nil, 0, err
	}

	if filter.PageSize > 0 && filter.Page > 0 {
		query = query.Scopes(Paginate(&filter.Pagination))
	}

	if err := query.Find(&cookies).Error; err != nil {
		log.Error().Err(err).Msg("Failed to list workspace cookies")
		return nil, 0, err
	}

	return cookies, count, nil
}

// Helper functions to work with http.CookieJar interface
func (d *DatabaseConnection) GetCookiesForURL(workspaceID uint, u *url.URL) []*http.Cookie {
	var cookies []WorkspaceCookie
	err := d.db.Where("workspace_id = ? AND domain = ?", workspaceID, u.Host).Find(&cookies).Error
	if err != nil {
		log.Error().Err(err).Uint("workspace_id", workspaceID).Str("url", u.String()).Msg("Failed to get cookies for URL")
		return nil
	}

	httpCookies := make([]*http.Cookie, len(cookies))
	for i, cookie := range cookies {
		httpCookies[i] = cookie.ToHTTPCookie()
	}
	return httpCookies
}

func (d *DatabaseConnection) SetCookiesForURL(workspaceID uint, u *url.URL, cookies []*http.Cookie) error {
	for _, cookie := range cookies {
		workspaceCookie := &WorkspaceCookie{
			WorkspaceID: &workspaceID,
			Name:        cookie.Name,
			Value:       cookie.Value,
			Domain:      cookie.Domain,
			Path:        cookie.Path,
			Expires:     cookie.Expires,
			MaxAge:      cookie.MaxAge,
			Secure:      cookie.Secure,
			HttpOnly:    cookie.HttpOnly,
		}

		switch cookie.SameSite {
		case http.SameSiteDefaultMode:
			workspaceCookie.SameSite = "Default"
		case http.SameSiteStrictMode:
			workspaceCookie.SameSite = "Strict"
		case http.SameSiteLaxMode:
			workspaceCookie.SameSite = "Lax"
		case http.SameSiteNoneMode:
			workspaceCookie.SameSite = "None"
		}

		if workspaceCookie.Domain == "" {
			workspaceCookie.Domain = u.Host
		}

		result := d.db.Where("workspace_id = ? AND domain = ? AND name = ?",
			workspaceID, workspaceCookie.Domain, workspaceCookie.Name).
			Assign(workspaceCookie).
			FirstOrCreate(workspaceCookie)

		if result.Error != nil {
			log.Error().Err(result.Error).
				Interface("cookie", workspaceCookie).
				Msg("Failed to upsert workspace cookie")
			return result.Error
		}
	}
	return nil
}

// CookieJar type that implements http.CookieJar interface
type WorkspaceCookieJar struct {
	workspaceID uint
	mu          sync.RWMutex
}

func NewWorkspaceCookieJar(workspaceID uint) *WorkspaceCookieJar {
	return &WorkspaceCookieJar{
		workspaceID: workspaceID,
	}
}

func (j *WorkspaceCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()

	err := Connection.SetCookiesForURL(j.workspaceID, u, cookies)
	if err != nil {
		log.Error().Err(err).
			Uint("workspace_id", j.workspaceID).
			Str("url", u.String()).
			Msg("Failed to set cookies for URL")
	}
}

func (j *WorkspaceCookieJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return Connection.GetCookiesForURL(j.workspaceID, u)
}
