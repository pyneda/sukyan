package cmd

import (
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/spf13/cobra"
)

// registerCrawlOptionFlags adds the per-scan crawl behaviour flags to a command.
// Defaults shown in the help text come from options.DefaultCrawlOptions, but the
// flag values are only sent when explicitly set, so an untouched flag leaves the
// option unset and the scan resolves it to the default.
func registerCrawlOptionFlags(cmd *cobra.Command) {
	defaults := options.DefaultCrawlOptions()

	cmd.Flags().Int("page-setup-timeout", int(defaults.PageSetupTimeout.Seconds()), "Seconds allowed to prepare a browser page before loading it")
	cmd.Flags().Int("navigation-timeout", int(defaults.NavigationTimeout.Seconds()), "Seconds allowed for a page to navigate and load")
	cmd.Flags().Int("interaction-timeout", int(defaults.InteractionTimeout.Seconds()), "Seconds allowed for form submission and clicking on each page")
	cmd.Flags().Bool("wait-for-stable-page", defaults.WaitForStablePage, "Wait for the page to stop changing before extracting links")
	cmd.Flags().Int("page-stable-duration", int(defaults.PageStableDuration.Seconds()), "Seconds a page must stay unchanged to count as stable")
	cmd.Flags().Int("page-stable-timeout", int(defaults.PageStableTimeout.Seconds()), "Seconds to wait for a page to become stable before giving up")
	cmd.Flags().Bool("submit-forms", defaults.SubmitForms, "Fill and submit forms found while crawling")
	cmd.Flags().Bool("submit-each-form-once", defaults.SubmitEachFormOnce, "Submit a form only the first time it is seen, instead of on every page it appears on")
	cmd.Flags().Bool("click-buttons", defaults.ClickButtons, "Click buttons found while crawling")
	cmd.Flags().Bool("capture-client-navigation", defaults.CaptureClientNavigation, "Capture client-side navigations that emit no request")
	cmd.Flags().Int("max-pages-with-same-parameters", defaults.MaxPagesWithSameParameters, "Max pages sharing a path and query parameter names, differing only in values (0 = unlimited)")
	cmd.Flags().StringArray("seed-path", defaults.SeedPaths, "Path crawled at each start URL's origin, repeatable")
	cmd.Flags().StringArray("exclude-extension", defaults.ExcludeExtensions, "URL extension to skip while crawling, repeatable")
	cmd.Flags().String("user-agent", defaults.UserAgent, "User agent for crawler browser pages")
}

// crawlOptionsFromFlags collects only the flags the user actually set. It returns
// nil when none were, so the scan stores no crawl options at all.
func crawlOptionsFromFlags(cmd *cobra.Command) *options.FullScanCrawlOptions {
	crawlOptions := &options.FullScanCrawlOptions{}
	changed := false

	intFlag := func(name string, target **int) {
		if !cmd.Flags().Changed(name) {
			return
		}
		value, err := cmd.Flags().GetInt(name)
		if err != nil {
			return
		}
		*target = &value
		changed = true
	}

	boolFlag := func(name string, target **bool) {
		if !cmd.Flags().Changed(name) {
			return
		}
		value, err := cmd.Flags().GetBool(name)
		if err != nil {
			return
		}
		*target = &value
		changed = true
	}

	stringArrayFlag := func(name string, target *[]string) {
		if !cmd.Flags().Changed(name) {
			return
		}
		value, err := cmd.Flags().GetStringArray(name)
		if err != nil {
			return
		}
		*target = value
		changed = true
	}

	intFlag("page-setup-timeout", &crawlOptions.PageSetupTimeoutSeconds)
	intFlag("navigation-timeout", &crawlOptions.NavigationTimeoutSeconds)
	intFlag("interaction-timeout", &crawlOptions.InteractionTimeoutSeconds)
	intFlag("page-stable-duration", &crawlOptions.PageStableDurationSeconds)
	intFlag("page-stable-timeout", &crawlOptions.PageStableTimeoutSeconds)
	intFlag("max-pages-with-same-parameters", &crawlOptions.MaxPagesWithSameParameters)
	boolFlag("wait-for-stable-page", &crawlOptions.WaitForStablePage)
	boolFlag("submit-forms", &crawlOptions.SubmitForms)
	boolFlag("submit-each-form-once", &crawlOptions.SubmitEachFormOnce)
	boolFlag("click-buttons", &crawlOptions.ClickButtons)
	boolFlag("capture-client-navigation", &crawlOptions.CaptureClientNavigation)
	stringArrayFlag("seed-path", &crawlOptions.SeedPaths)
	stringArrayFlag("exclude-extension", &crawlOptions.ExcludeExtensions)

	if cmd.Flags().Changed("user-agent") {
		if value, err := cmd.Flags().GetString("user-agent"); err == nil {
			crawlOptions.UserAgent = &value
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return crawlOptions
}
