package cmd

import (
	"fmt"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	apidefsListWorkspaceID uint
	apidefsListType        string
	apidefsListFormat      string
	apidefsListPageSize    int
)

var apidefsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored API definitions",
	Long: `List all API definitions stored in the database.

Results can be filtered by workspace and type.

Examples:
  # List all definitions in workspace 1
  sukyan apidefs list -w 1

  # List only GraphQL definitions
  sukyan apidefs list -w 1 --type graphql

  # Output as JSON
  sukyan apidefs list -w 1 --format json`,
	Run: runAPIDefsList,
}

func init() {
	apidefsCmd.AddCommand(apidefsListCmd)

	apidefsListCmd.Flags().UintVarP(&apidefsListWorkspaceID, "workspace", "w", 0, "Filter by workspace ID")
	apidefsListCmd.Flags().StringVar(&apidefsListType, "type", "", "Filter by type: openapi, graphql, wsdl")
	apidefsListCmd.Flags().StringVarP(&apidefsListFormat, "format", "f", "table", "Output format: table, json, pretty")
	apidefsListCmd.Flags().IntVarP(&apidefsListPageSize, "page-size", "s", 50, "Number of results to show")
}

func runAPIDefsList(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "apidefs-list").Logger()

	filter := db.APIDefinitionFilter{
		WorkspaceID: apidefsListWorkspaceID,
		Pagination: db.Pagination{
			PageSize: apidefsListPageSize,
			Page:     1,
		},
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	if apidefsListType != "" {
		switch apidefsListType {
		case "openapi":
			filter.Types = []db.APIDefinitionType{db.APIDefinitionTypeOpenAPI}
		case "graphql":
			filter.Types = []db.APIDefinitionType{db.APIDefinitionTypeGraphQL}
		case "wsdl":
			filter.Types = []db.APIDefinitionType{db.APIDefinitionTypeWSDL}
		default:
			logger.Error().Str("type", apidefsListType).Msg("Invalid type filter")
			os.Exit(1)
		}
	}

	definitions, count, err := db.Connection().ListAPIDefinitions(filter)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list API definitions")
		os.Exit(1)
	}

	if count == 0 {
		logger.Info().Msg("No API definitions found")
		return
	}

	formatType, err := lib.ParseFormatType(apidefsListFormat)
	if err != nil {
		logger.Error().Err(err).Msg("Invalid format type")
		os.Exit(1)
	}

	switch formatType {
	case lib.Table:
		printDefinitionsTable(definitions)
	case lib.JSON:
		output, _ := lib.FormatOutput(definitions, lib.JSON)
		fmt.Println(output)
	case lib.Pretty:
		for _, def := range definitions {
			fmt.Println(def.Pretty())
			fmt.Println("---")
		}
	default:
		printDefinitionsTable(definitions)
	}

	logger.Info().Int64("total", count).Int("shown", len(definitions)).Msg("API definitions listed")
}

func printDefinitionsTable(definitions []*db.APIDefinition) {
	if len(definitions) == 0 {
		return
	}

	headers := definitions[0].TableHeaders()

	fmt.Printf("%-10s %-30s %-10s %-12s %-10s %-40s %-10s\n",
		headers[0], headers[1], headers[2], headers[3], headers[4], headers[5], headers[6])
	fmt.Println("------------------------------------------------------------------------------------------------------------------")

	for _, def := range definitions {
		row := def.TableRow()
		fmt.Printf("%-10s %-30s %-10s %-12s %-10s %-40s %-10s\n",
			row[0], truncateString(row[1], 30), row[2], row[3], row[4], truncateString(row[5], 40), row[6])
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
