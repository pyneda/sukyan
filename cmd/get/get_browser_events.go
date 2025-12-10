package get

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var browserEventFilterScanID uint
var browserEventFilterEventTypes []string
var browserEventFilterCategories []string
var browserEventFilterURL string
var browserEventFilterSources []string
var browserEventFilterSortBy string
var browserEventFilterSortOrder string

// getBrowserEventsCmd represents the browser-events command
var getBrowserEventsCmd = &cobra.Command{
	Use:     "browser-events",
	Aliases: []string{"be", "events", "browser-event"},
	Short:   "List browser events captured during scanning",
	Long:    `List browser events (console messages, dialogs, storage events, etc.) captured during scanning`,
	Run: func(cmd *cobra.Command, args []string) {
		// Convert string event types to BrowserEventType
		var eventTypes []db.BrowserEventType
		for _, et := range browserEventFilterEventTypes {
			eventTypes = append(eventTypes, db.BrowserEventType(et))
		}

		// Convert string categories to BrowserEventCategory
		var categories []db.BrowserEventCategory
		for _, cat := range browserEventFilterCategories {
			categories = append(categories, db.BrowserEventCategory(cat))
		}

		// Build filter
		filter := db.BrowserEventFilter{
			WorkspaceID: uint(workspaceID),
			EventTypes:  eventTypes,
			Categories:  categories,
			URL:         browserEventFilterURL,
			Sources:     browserEventFilterSources,
			SortBy:      browserEventFilterSortBy,
			SortOrder:   browserEventFilterSortOrder,
			Pagination: db.Pagination{
				Page:     page,
				PageSize: pageSize,
			},
		}

		// Set scan ID if provided
		if browserEventFilterScanID > 0 {
			filter.ScanID = &browserEventFilterScanID
		}

		items, count, err := db.Connection().ListBrowserEvents(filter)
		if err != nil {
			log.Error().Err(err).Msg("Error listing browser events from database")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(items, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
		fmt.Printf("\nTotal: %d events\n", count)
	},
}

func init() {
	GetCmd.AddCommand(getBrowserEventsCmd)

	getBrowserEventsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getBrowserEventsCmd.Flags().UintVar(&browserEventFilterScanID, "scan", 0, "Filter by scan ID")
	getBrowserEventsCmd.Flags().StringSliceVarP(&browserEventFilterEventTypes, "type", "t", []string{}, "Filter by event type (console, dialog, dom_storage, security, certificate, audit, indexeddb, cache_storage, background_service, database, network_auth). Can be added multiple times.")
	getBrowserEventsCmd.Flags().StringSliceVarP(&browserEventFilterCategories, "category", "c", []string{}, "Filter by category (runtime, storage, security, network, audit). Can be added multiple times.")
	getBrowserEventsCmd.Flags().StringVarP(&browserEventFilterURL, "url", "u", "", "Filter by URL (partial match)")
	getBrowserEventsCmd.Flags().StringSliceVar(&browserEventFilterSources, "source", []string{}, "Filter by source (crawler, replay, audit, etc.). Can be added multiple times.")
	getBrowserEventsCmd.Flags().StringVar(&browserEventFilterSortBy, "sort-by", "last_seen_at", "Sort by field (id, created_at, event_type, category, occurrence_count, last_seen_at, first_seen_at)")
	getBrowserEventsCmd.Flags().StringVar(&browserEventFilterSortOrder, "sort-order", "desc", "Sort order (asc, desc)")
}
