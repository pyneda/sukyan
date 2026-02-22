package db

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ProxyService represents a managed proxy instance
type ProxyService struct {
	BaseUUIDModel

	// Workspace scoping
	WorkspaceID *uint     `json:"workspace_id" gorm:"index;not null"`
	Workspace   Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	// Basic config
	Name string `json:"name" gorm:"not null"`
	Host string `json:"host" gorm:"default:localhost"`
	Port int    `json:"port" gorm:"not null;uniqueIndex"`

	// Current proxy settings
	Verbose               bool `json:"verbose" gorm:"default:true"`
	LogOutOfScopeRequests bool `json:"log_out_of_scope_requests" gorm:"default:true"`

	// State management
	Enabled bool `json:"enabled" gorm:"default:false;index"`
}

// TableHeaders returns the table headers for ProxyService
func (p ProxyService) TableHeaders() []string {
	return []string{"ID", "Name", "Host", "Port", "Enabled"}
}

// TableRow returns the table row for ProxyService
func (p ProxyService) TableRow() []string {
	return []string{
		p.ID.String(),
		p.Name,
		p.Host,
		fmt.Sprintf("%d", p.Port),
		fmt.Sprintf("%t", p.Enabled),
	}
}

// CreateProxyService creates a new proxy service
func (conn *DatabaseConnection) CreateProxyService(proxyService *ProxyService) (*ProxyService, error) {
	result := conn.db.Create(proxyService)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("proxy_service", proxyService).Msg("ProxyService creation failed")
		return nil, result.Error
	}
	return proxyService, nil
}

// GetProxyServiceByID retrieves a proxy service by ID
func (conn *DatabaseConnection) GetProxyServiceByID(id uuid.UUID) (*ProxyService, error) {
	var proxyService ProxyService
	if err := conn.db.Where("id = ?", id).First(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch proxy service by ID")
		return nil, err
	}
	return &proxyService, nil
}

// GetProxyServiceByPort retrieves a proxy service by port
func (conn *DatabaseConnection) GetProxyServiceByPort(port int) (*ProxyService, error) {
	var proxyService ProxyService
	if err := conn.db.Where("port = ?", port).First(&proxyService).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		log.Error().Err(err).Int("port", port).Msg("Unable to fetch proxy service by port")
		return nil, err
	}
	return &proxyService, nil
}

// ListProxyServicesByWorkspace lists all proxy services for a workspace
func (conn *DatabaseConnection) ListProxyServicesByWorkspace(workspaceID uint) ([]*ProxyService, error) {
	var proxyServices []*ProxyService
	if err := conn.db.Where("workspace_id = ?", workspaceID).Find(&proxyServices).Error; err != nil {
		log.Error().Err(err).Uint("workspace_id", workspaceID).Msg("Unable to list proxy services")
		return nil, err
	}
	return proxyServices, nil
}

// ListEnabledProxyServices lists all enabled proxy services
func (conn *DatabaseConnection) ListEnabledProxyServices() ([]*ProxyService, error) {
	var proxyServices []*ProxyService
	if err := conn.db.Where("enabled = ?", true).Find(&proxyServices).Error; err != nil {
		log.Error().Err(err).Msg("Unable to list enabled proxy services")
		return nil, err
	}
	return proxyServices, nil
}

// UpdateProxyService updates a proxy service
func (conn *DatabaseConnection) UpdateProxyService(id uuid.UUID, updates *ProxyService) error {
	var proxyService ProxyService
	if err := conn.db.Where("id = ?", id).First(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to fetch proxy service for update")
		return err
	}

	// Update fields
	if updates.Name != "" {
		proxyService.Name = updates.Name
	}
	if updates.Host != "" {
		proxyService.Host = updates.Host
	}
	if updates.Port != 0 {
		proxyService.Port = updates.Port
	}
	proxyService.Verbose = updates.Verbose
	proxyService.LogOutOfScopeRequests = updates.LogOutOfScopeRequests
	proxyService.Enabled = updates.Enabled

	if err := conn.db.Save(&proxyService).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to update proxy service")
		return err
	}
	return nil
}

// DeleteProxyService deletes a proxy service
func (conn *DatabaseConnection) DeleteProxyService(id uuid.UUID) error {
	if err := conn.db.Where("id = ?", id).Delete(&ProxyService{}).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Unable to delete proxy service")
		return err
	}
	return nil
}

// SetProxyServiceEnabled sets the enabled status of a proxy service
func (conn *DatabaseConnection) SetProxyServiceEnabled(id uuid.UUID, enabled bool) error {
	if err := conn.db.Model(&ProxyService{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		log.Error().Err(err).Str("id", id.String()).Bool("enabled", enabled).Msg("Unable to update proxy service enabled status")
		return err
	}
	return nil
}
