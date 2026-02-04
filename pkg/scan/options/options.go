package options

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
)

// JobTimeouts configures maximum durations for different job types.
// Jobs exceeding these durations are considered stale and will be reset.
type JobTimeouts struct {
	Crawl       time.Duration `json:"crawl" swaggertype:"integer" example:"3600000000000"`
	Discovery   time.Duration `json:"discovery" swaggertype:"integer" example:"300000000000"`
	ActiveScan  time.Duration `json:"active_scan" swaggertype:"integer" example:"1800000000000"`
	Nuclei      time.Duration `json:"nuclei" swaggertype:"integer" example:"1200000000000"`
	Fingerprint time.Duration `json:"fingerprint" swaggertype:"integer" example:"300000000000"`
	WebSocket   time.Duration `json:"websocket" swaggertype:"integer" example:"900000000000"`
	APIScan     time.Duration `json:"api_scan" swaggertype:"integer" example:"1800000000000"`
}

// DefaultJobTimeouts returns the default job timeouts.
func DefaultJobTimeouts() JobTimeouts {
	return JobTimeouts{
		Crawl:       1 * time.Hour,
		Discovery:   5 * time.Minute,
		ActiveScan:  30 * time.Minute,
		Nuclei:      20 * time.Minute,
		Fingerprint: 5 * time.Minute,
		WebSocket:   15 * time.Minute,
		APIScan:     30 * time.Minute,
	}
}

// GetTimeout returns the timeout for a specific job type.
func (jt JobTimeouts) GetTimeout(jobType string) time.Duration {
	switch jobType {
	case "crawl":
		return jt.Crawl
	case "discovery":
		return jt.Discovery
	case "active_scan":
		return jt.ActiveScan
	case "nuclei":
		return jt.Nuclei
	case "fingerprint":
		return jt.Fingerprint
	case "websocket_scan":
		return jt.WebSocket
	case "api_scan":
		return jt.APIScan
	default:
		return 30 * time.Minute // Default fallback
	}
}

type ScanMode string

const (
	ScanModeFast  ScanMode = "fast"
	ScanModeSmart ScanMode = "smart"
	ScanModeFuzz  ScanMode = "fuzz"
)

func NewScanMode(mode string) ScanMode {
	switch mode {
	case ScanModeFast.String():
		return ScanModeFast
	case ScanModeSmart.String():
		return ScanModeSmart
	case ScanModeFuzz.String():
		return ScanModeFuzz
	default:
		return ScanModeSmart
	}
}

func (sm ScanMode) String() string {
	return string(sm)
}

func (sm ScanMode) IsHigherOrEqual(other ScanMode) bool {
	order := map[ScanMode]int{
		ScanModeFast:  1,
		ScanModeSmart: 2,
		ScanModeFuzz:  3,
	}
	return order[sm] >= order[other]
}

func (sm ScanMode) IsLowerOrEqual(other ScanMode) bool {
	order := map[ScanMode]int{
		ScanModeFast:  1,
		ScanModeSmart: 2,
		ScanModeFuzz:  3,
	}
	return order[sm] <= order[other]
}

func (sm ScanMode) MaxDiscoveryPathsPerModule() int {
	switch sm {
	case ScanModeFast:
		return 20
	case ScanModeSmart:
		return -1
	default:
		return -1
	}
}

type AuditCategories struct {
	Discovery  bool `json:"discovery"`
	ServerSide bool `json:"server_side"`
	ClientSide bool `json:"client_side"`
	Passive    bool `json:"passive"`
	WebSocket  bool `json:"websocket"`
}

type HistoryItemScanOptions struct {
	Ctx                context.Context   `json:"-"` // Context for cancellation propagation
	WorkspaceID        uint              `json:"workspace_id" validate:"required,min=0"`
	TaskID             uint              `json:"task_id" validate:"required,min=0"`
	TaskJobID          uint              `json:"task_job_id" validate:"required,min=0"`
	ScanID             uint              `json:"scan_id"`
	ScanJobID          uint              `json:"scan_job_id"`
	Mode               ScanMode          `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	InsertionPoints    []string          `json:"insertion_points" validate:"omitempty,dive,oneof=parameters urlpath body headers cookies json xml graphql"`
	FingerprintTags    []string          `json:"fingerprint_tags" validate:"omitempty,dive"`
	Fingerprints       []lib.Fingerprint `json:"fingerprints" validate:"omitempty,dive"`
	ExperimentalAudits bool              `json:"experimental_audits"`
	AuditCategories    AuditCategories   `json:"audit_categories" validate:"required"`
	MaxRetries         int               `json:"max_retries" validate:"min=0"`
	AuditSampler       *AuditSampler     `json:"-"`
	HTTPClient         *http.Client      `json:"-"`
}

func (o HistoryItemScanOptions) IsScopedInsertionPoint(insertionPoint string) bool {
	if len(o.InsertionPoints) == 0 {
		return true
	}

	for _, ip := range o.InsertionPoints {
		if ip == insertionPoint {
			return true
		}
	}
	return false
}

type FullScanOptions struct {
	Title                   string                   `json:"title" validate:"omitempty,min=1,max=255"`
	StartURLs               []string                 `json:"start_urls" validate:"omitempty,dive,url"`
	MaxDepth                int                      `json:"max_depth" validate:"min=0"`
	MaxPagesToCrawl         int                      `json:"max_pages_to_crawl" validate:"min=0"`
	MaxPagesPerSite         int                      `json:"max_pages_per_site" validate:"min=0"`
	ExcludePatterns         []string                 `json:"exclude_patterns"`
	WorkspaceID             uint                     `json:"workspace_id" validate:"required,min=0"`
	PagesPoolSize           int                      `json:"pages_pool_size" validate:"min=1,max=100"`
	Headers                 map[string][]string      `json:"headers" validate:"omitempty"`
	InsertionPoints         []string                 `json:"insertion_points" validate:"omitempty,dive,oneof=parameters urlpath body headers cookies json xml graphql"`
	Mode                    ScanMode                 `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	ExperimentalAudits      bool                     `json:"experimental_audits"`
	AuditCategories         AuditCategories          `json:"audit_categories" validate:"required"`
	WebSocketOptions        FullScanWebSocketOptions `json:"websocket_options" validate:"omitempty"`
	APIScanOptions          FullScanAPIScanOptions   `json:"api_scan_options" validate:"omitempty"`
	MaxRetries              int                      `json:"max_retries" validate:"min=0"`
	MaxConcurrentJobs       *int                     `json:"max_concurrent_jobs,omitempty"`
	MaxRPS                  *int                     `json:"max_rps,omitempty"`
	JobTimeouts             *JobTimeouts             `json:"job_timeouts,omitempty"`
	CaptureBrowserEvents    bool                     `json:"capture_browser_events"`
	HTTPTimeout             *int                     `json:"http_timeout,omitempty"`
	HTTPMaxIdleConns        *int                     `json:"http_max_idle_conns,omitempty"`
	HTTPMaxIdleConnsPerHost *int                     `json:"http_max_idle_conns_per_host,omitempty"`
	HTTPMaxConnsPerHost     *int                     `json:"http_max_conns_per_host,omitempty"`
	HTTPDisableKeepAlives   *bool                    `json:"http_disable_keep_alives,omitempty"`
	PauseOnAuthFailure      bool                     `json:"pause_on_auth_failure"`
}

type APIDefinitionScanConfig struct {
	DefinitionID  uuid.UUID            `json:"definition_id" validate:"required"`
	EndpointIDs   []uuid.UUID          `json:"endpoint_ids,omitempty"`
	AuthConfigID  *uuid.UUID           `json:"auth_config_id,omitempty"`
	SchemeAuthMap map[string]uuid.UUID  `json:"scheme_auth_map,omitempty"`
}

type FullScanAPIScanOptions struct {
	Enabled             bool `json:"enabled"`
	RunAPISpecificTests bool `json:"run_api_specific_tests"`
	RunStandardTests    bool `json:"run_standard_tests"`
	RunSchemaTests      bool `json:"run_schema_tests"`

	DefinitionConfigs []APIDefinitionScanConfig `json:"definition_configs,omitempty"`
}

type APIContext struct {
	DefinitionType string         `json:"definition_type"`
	DefinitionID   *uuid.UUID     `json:"definition_id,omitempty"`
	EndpointID     *uuid.UUID     `json:"endpoint_id,omitempty"`
	Parameters     []APIParameter `json:"parameters,omitempty"`
}

type APIParameter struct {
	Name      string   `json:"name"`
	In        string   `json:"in"`
	Type      string   `json:"type"`
	Format    string   `json:"format,omitempty"`
	Required  bool     `json:"required"`
	Enum      []string `json:"enum,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
}

type FullScanWebSocketOptions struct {
	Concurrency       int  `json:"concurrency" validate:"min=1,max=100"`
	ReplayMessages    bool `json:"replay_messages"`
	ObservationWindow int  `json:"observation_window" validate:"min=1,max=100"`
}

func GetValidInsertionPoints() []string {
	return []string{"parameters", "urlpath", "body", "headers", "cookies", "json", "xml", "graphql"}
}

func GetValidScanModes() []string {
	return []string{ScanModeFast.String(), ScanModeSmart.String(), ScanModeFuzz.String()}
}

func IsValidScanMode(mode string) bool {
	for _, validMode := range GetValidScanModes() {
		if mode == validMode {
			return true
		}
	}
	return false
}

func GetScanMode(mode string) ScanMode {
	switch mode {
	case ScanModeFast.String():
		return ScanModeFast
	case ScanModeSmart.String():
		return ScanModeSmart
	case ScanModeFuzz.String():
		return ScanModeFuzz
	default:
		return ScanModeSmart
	}
}

// WebSocketScanOptions defines the options for a WebSocket scan
type WebSocketScanOptions struct {
	WorkspaceID       uint
	TaskID            uint
	TaskJobID         uint
	ScanID            uint
	ScanJobID         uint
	Mode              ScanMode
	FingerprintTags   []string
	ReplayMessages    bool          // Whether to replay previous messages to establish context
	ObservationWindow time.Duration // How long to wait for responses
	Concurrency       int
}
