package scan

type ScanMode string

const (
	ScanModeFast  ScanMode = "fast"
	ScanModeSmart ScanMode = "smart"
	ScanModeFuzz  ScanMode = "fuzz"
)

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

type HistoryItemScanOptions struct {
	WorkspaceID        uint     `json:"workspace_id" validate:"required,min=0"`
	TaskID             uint     `json:"task_id" validate:"required,min=0"`
	TaskJobID          uint     `json:"task_job_id" validate:"required,min=0"`
	Mode               ScanMode `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	InsertionPoints    []string `json:"insertion_points" validate:"omitempty,dive,oneof=parameters urlpath body headers cookies json xml"`
	FingerprintTags    []string `json:"fingerprint_tags" validate:"omitempty,dive"`
	ExperimentalAudits bool     `json:"experimental_audits"`
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
	Title              string              `json:"title" validate:"omitempty,min=1,max=255"`
	StartURLs          []string            `json:"start_urls" validate:"required,dive,url"`
	MaxDepth           int                 `json:"max_depth" validate:"min=0"`
	MaxPagesToCrawl    int                 `json:"max_pages_to_crawl" validate:"min=0"`
	ExcludePatterns    []string            `json:"exclude_patterns"`
	WorkspaceID        uint                `json:"workspace_id" validate:"required,min=0"`
	PagesPoolSize      int                 `json:"pages_pool_size" validate:"min=1,max=100"`
	Headers            map[string][]string `json:"headers" validate:"omitempty"`
	InsertionPoints    []string            `json:"insertion_points" validate:"omitempty,dive,oneof=parameters urlpath body headers cookies json xml"`
	Mode               ScanMode            `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
	ExperimentalAudits bool                `json:"experimental_audits"`
}

func GetValidInsertionPoints() []string {
	return []string{"parameters", "urlpath", "body", "headers", "cookies", "json", "xml"}
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
