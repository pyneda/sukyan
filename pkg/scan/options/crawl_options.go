package options

import "time"

// FullScanCrawlOptions configures how the crawler drives a target's pages.
//
// Every field is optional: a nil pointer (or nil slice) resolves to the default.
// Pointers are required rather than plain values because false and 0 are both
// meaningful settings here - MaxPagesWithSameParameters: 0 means unlimited - so a
// zero value cannot be used to mean "unset". This also makes scans stored before
// these options existed resolve to the shipped defaults.
type FullScanCrawlOptions struct {
	PageSetupTimeoutSeconds    *int     `json:"page_setup_timeout_seconds,omitempty" validate:"omitempty,min=1,max=300"`
	NavigationTimeoutSeconds   *int     `json:"navigation_timeout_seconds,omitempty" validate:"omitempty,min=1,max=300"`
	InteractionTimeoutSeconds  *int     `json:"interaction_timeout_seconds,omitempty" validate:"omitempty,min=1,max=120"`
	WaitForStablePage          *bool    `json:"wait_for_stable_page,omitempty"`
	PageStableDurationSeconds  *int     `json:"page_stable_duration_seconds,omitempty" validate:"omitempty,min=1,max=60"`
	PageStableTimeoutSeconds   *int     `json:"page_stable_timeout_seconds,omitempty" validate:"omitempty,min=1,max=120"`
	SubmitForms                *bool    `json:"submit_forms,omitempty"`
	SubmitEachFormOnce         *bool    `json:"submit_each_form_once,omitempty"`
	ClickButtons               *bool    `json:"click_buttons,omitempty"`
	CaptureClientNavigation    *bool    `json:"capture_client_navigation,omitempty"`
	MaxPagesWithSameParameters *int     `json:"max_pages_with_same_parameters,omitempty" validate:"omitempty,min=0"`
	SeedPaths                  []string `json:"seed_paths"`
	ExcludeExtensions          []string `json:"exclude_extensions"`
	UserAgent                  *string  `json:"user_agent,omitempty"`
}

// ResolvedCrawlOptions is FullScanCrawlOptions with every default applied, ready
// for the crawler to use without further lookups.
type ResolvedCrawlOptions struct {
	PageSetupTimeout           time.Duration
	NavigationTimeout          time.Duration
	InteractionTimeout         time.Duration
	WaitForStablePage          bool
	PageStableDuration         time.Duration
	PageStableTimeout          time.Duration
	SubmitForms                bool
	SubmitEachFormOnce         bool
	ClickButtons               bool
	CaptureClientNavigation    bool
	MaxPagesWithSameParameters int
	SeedPaths                  []string
	ExcludeExtensions          []string
	UserAgent                  string
}

// DefaultExcludeExtensions lists the URL suffixes the crawler skips by default:
// static assets and documents that cost a request but yield no new links.
func DefaultExcludeExtensions() []string {
	return []string{
		".jpg", ".woff2", ".png", ".gif", ".webp", ".ico", ".css", ".svg", ".tif",
		".tiff", ".bmp", ".raw", ".indd", ".ai", ".eps", ".pdf", ".exe", ".dll",
		".psd", ".fla", ".avi", ".flv", ".mov", ".mp4", ".mpg", ".mpeg", ".swf",
		".mkv", ".wav", ".mp3", ".flac", ".m4a", ".wma", ".aac", ".doc", ".docx",
		".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".zip", ".rar", ".7z", ".tar.gz",
		".iso", ".dmg",
	}
}

// DefaultSeedPaths lists the paths crawled at each start URL's origin in addition
// to the start URL itself.
func DefaultSeedPaths() []string {
	return []string{"/robots.txt", "/sitemap.xml"}
}

// DefaultCrawlOptions returns the crawl behaviour used when a scan specifies none.
func DefaultCrawlOptions() ResolvedCrawlOptions {
	return ResolvedCrawlOptions{
		PageSetupTimeout:           15 * time.Second,
		NavigationTimeout:          10 * time.Second,
		InteractionTimeout:         10 * time.Second,
		WaitForStablePage:          true,
		PageStableDuration:         2 * time.Second,
		PageStableTimeout:          10 * time.Second,
		SubmitForms:                true,
		SubmitEachFormOnce:         false,
		ClickButtons:               true,
		CaptureClientNavigation:    true,
		MaxPagesWithSameParameters: 20,
		SeedPaths:                  DefaultSeedPaths(),
		ExcludeExtensions:          DefaultExcludeExtensions(),
		UserAgent:                  "",
	}
}

// Resolve applies the configured overrides on top of the defaults. It is safe to
// call on a nil receiver, which yields the defaults unchanged.
func (o *FullScanCrawlOptions) Resolve() ResolvedCrawlOptions {
	resolved := DefaultCrawlOptions()
	if o == nil {
		return resolved
	}

	if o.PageSetupTimeoutSeconds != nil {
		resolved.PageSetupTimeout = time.Duration(*o.PageSetupTimeoutSeconds) * time.Second
	}
	if o.NavigationTimeoutSeconds != nil {
		resolved.NavigationTimeout = time.Duration(*o.NavigationTimeoutSeconds) * time.Second
	}
	if o.InteractionTimeoutSeconds != nil {
		resolved.InteractionTimeout = time.Duration(*o.InteractionTimeoutSeconds) * time.Second
	}
	if o.WaitForStablePage != nil {
		resolved.WaitForStablePage = *o.WaitForStablePage
	}
	if o.PageStableDurationSeconds != nil {
		resolved.PageStableDuration = time.Duration(*o.PageStableDurationSeconds) * time.Second
	}
	if o.PageStableTimeoutSeconds != nil {
		resolved.PageStableTimeout = time.Duration(*o.PageStableTimeoutSeconds) * time.Second
	}
	if o.SubmitForms != nil {
		resolved.SubmitForms = *o.SubmitForms
	}
	if o.SubmitEachFormOnce != nil {
		resolved.SubmitEachFormOnce = *o.SubmitEachFormOnce
	}
	if o.ClickButtons != nil {
		resolved.ClickButtons = *o.ClickButtons
	}
	if o.CaptureClientNavigation != nil {
		resolved.CaptureClientNavigation = *o.CaptureClientNavigation
	}
	if o.MaxPagesWithSameParameters != nil {
		resolved.MaxPagesWithSameParameters = *o.MaxPagesWithSameParameters
	}
	if o.SeedPaths != nil {
		resolved.SeedPaths = o.SeedPaths
	}
	if o.ExcludeExtensions != nil {
		resolved.ExcludeExtensions = o.ExcludeExtensions
	}
	if o.UserAgent != nil {
		resolved.UserAgent = *o.UserAgent
	}

	return resolved
}
