package cmd

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	apidefsShowFormat         string
	apidefsShowIncludeEndpoints bool
)

var apidefsShowCmd = &cobra.Command{
	Use:   "show [definition-id]",
	Short: "Show API definition details",
	Long: `Show detailed information about an API definition including its endpoints.

Examples:
  # Show definition details
  sukyan apidefs show abc12345-uuid-here

  # Show with endpoints
  sukyan apidefs show abc12345-uuid-here --endpoints

  # Output as JSON
  sukyan apidefs show abc12345-uuid-here --format json`,
	Args: cobra.ExactArgs(1),
	Run:  runAPIDefsShow,
}

func init() {
	apidefsCmd.AddCommand(apidefsShowCmd)

	apidefsShowCmd.Flags().StringVarP(&apidefsShowFormat, "format", "f", "pretty", "Output format: pretty, json")
	apidefsShowCmd.Flags().BoolVar(&apidefsShowIncludeEndpoints, "endpoints", true, "Include endpoint details")
}

func runAPIDefsShow(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "apidefs-show").Logger()

	definitionID, err := uuid.Parse(args[0])
	if err != nil {
		logger.Error().Err(err).Str("id", args[0]).Msg("Invalid definition ID format")
		os.Exit(1)
	}

	var definition *db.APIDefinition
	if apidefsShowIncludeEndpoints {
		definition, err = db.Connection().GetAPIDefinitionByIDWithEndpoints(definitionID)
	} else {
		definition, err = db.Connection().GetAPIDefinitionByID(definitionID)
	}

	if err != nil {
		logger.Error().Err(err).Str("id", args[0]).Msg("Failed to get API definition")
		os.Exit(1)
	}

	formatType, err := lib.ParseFormatType(apidefsShowFormat)
	if err != nil {
		formatType = lib.Pretty
	}

	switch formatType {
	case lib.JSON:
		output, _ := lib.FormatSingleOutput(definition, lib.JSON)
		fmt.Println(output)
	default:
		printDefinitionDetails(definition)
	}
}

func printDefinitionDetails(definition *db.APIDefinition) {
	fmt.Println("=== API Definition ===")
	fmt.Printf("ID:           %s\n", definition.ID.String())
	fmt.Printf("Name:         %s\n", definition.Name)
	fmt.Printf("Type:         %s\n", definition.Type)
	fmt.Printf("Status:       %s\n", definition.Status)
	fmt.Printf("Base URL:     %s\n", definition.BaseURL)
	fmt.Printf("Source URL:   %s\n", definition.SourceURL)
	fmt.Printf("Workspace:    %d\n", definition.WorkspaceID)
	fmt.Printf("Endpoints:    %d\n", definition.EndpointCount)
	fmt.Printf("Created:      %s\n", definition.CreatedAt.Format("2006-01-02 15:04:05"))

	if definition.AutoDiscovered {
		fmt.Printf("Auto-discovered: yes\n")
	}

	if definition.ScanID != nil {
		fmt.Printf("Scan ID:      %d\n", *definition.ScanID)
	}

	switch definition.Type {
	case db.APIDefinitionTypeOpenAPI:
		if definition.OpenAPIVersion != nil {
			fmt.Printf("\n--- OpenAPI Details ---\n")
			fmt.Printf("Version:      %s\n", *definition.OpenAPIVersion)
			if definition.OpenAPITitle != nil {
				fmt.Printf("Title:        %s\n", *definition.OpenAPITitle)
			}
			fmt.Printf("Servers:      %d\n", definition.OpenAPIServers)
		}

	case db.APIDefinitionTypeGraphQL:
		fmt.Printf("\n--- GraphQL Details ---\n")
		fmt.Printf("Queries:       %d\n", definition.GraphQLQueryCount)
		fmt.Printf("Mutations:     %d\n", definition.GraphQLMutationCount)
		fmt.Printf("Subscriptions: %d\n", definition.GraphQLSubscriptionCount)
		fmt.Printf("Types:         %d\n", definition.GraphQLTypeCount)

	case db.APIDefinitionTypeWSDL:
		fmt.Printf("\n--- WSDL Details ---\n")
		if definition.WSDLTargetNamespace != nil {
			fmt.Printf("Namespace:    %s\n", *definition.WSDLTargetNamespace)
		}
		fmt.Printf("Services:     %d\n", definition.WSDLServiceCount)
		fmt.Printf("Ports:        %d\n", definition.WSDLPortCount)
		if definition.WSDLSOAPVersion != nil {
			fmt.Printf("SOAP Version: %s\n", *definition.WSDLSOAPVersion)
		}
	}

	if len(definition.Endpoints) > 0 {
		fmt.Printf("\n=== Endpoints (%d) ===\n", len(definition.Endpoints))
		fmt.Printf("%-10s %-8s %-40s %-25s %-8s %-8s\n",
			"ID", "Method", "Path", "Name", "Enabled", "Issues")
		fmt.Println(string(make([]byte, 100)))

		for _, endpoint := range definition.Endpoints {
			path := endpoint.Path
			if endpoint.OperationType != "" {
				path = fmt.Sprintf("[%s] %s", endpoint.OperationType, endpoint.Name)
			}
			if len(path) > 40 {
				path = path[:37] + "..."
			}

			name := endpoint.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}

			enabled := "yes"
			if !endpoint.Enabled {
				enabled = "no"
			}

			fmt.Printf("%-10s %-8s %-40s %-25s %-8s %-8d\n",
				endpoint.ID.String()[:8],
				endpoint.Method,
				path,
				name,
				enabled,
				endpoint.IssuesFound,
			)
		}
	}
}
