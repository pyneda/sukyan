package cmd

import (
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	apidefsParseURL         string
	apidefsParseFile        string
	apidefsParseWorkspaceID uint
	apidefsParseName        string
	apidefsParseAuthType    string
	apidefsParseUsername    string
	apidefsParsePassword    string
	apidefsParseToken       string
	apidefsParseAPIKeyName  string
	apidefsParseAPIKeyValue string
	apidefsParseAPIKeyIn    string
	apidefsParseBaseURL     string
)

var apidefsParseCmd = &cobra.Command{
	Use:   "parse",
	Short: "Parse and store an API definition without scanning",
	Long: `Parse an API definition (OpenAPI, GraphQL, WSDL) and store it in the database.

This is useful for importing definitions for later scanning via the UI or API.
No scanning is performed - only parsing and storage.

Examples:
  # Parse OpenAPI definition from URL
  sukyan apidefs parse --url https://api.example.com/openapi.json -w 1

  # Parse with a custom name
  sukyan apidefs parse --url https://api.example.com/openapi.json -w 1 --name "Production API"

  # Parse from local file
  sukyan apidefs parse --file ./openapi.yaml -w 1`,
	Run: runAPIDefsParse,
}

func init() {
	apidefsCmd.AddCommand(apidefsParseCmd)

	apidefsParseCmd.Flags().StringVarP(&apidefsParseURL, "url", "u", "", "API definition URL")
	apidefsParseCmd.Flags().StringVarP(&apidefsParseFile, "file", "f", "", "Local file path")
	apidefsParseCmd.Flags().UintVarP(&apidefsParseWorkspaceID, "workspace", "w", 0, "Workspace ID")
	apidefsParseCmd.Flags().StringVarP(&apidefsParseName, "name", "n", "", "Name for the API definition")
	apidefsParseCmd.Flags().StringVar(&apidefsParseAuthType, "auth-type", "none", "Auth type: none, basic, bearer, api_key")
	apidefsParseCmd.Flags().StringVar(&apidefsParseUsername, "username", "", "Basic auth username")
	apidefsParseCmd.Flags().StringVar(&apidefsParsePassword, "password", "", "Basic auth password")
	apidefsParseCmd.Flags().StringVar(&apidefsParseToken, "token", "", "Bearer token")
	apidefsParseCmd.Flags().StringVar(&apidefsParseAPIKeyName, "api-key-name", "", "API key name")
	apidefsParseCmd.Flags().StringVar(&apidefsParseAPIKeyValue, "api-key-value", "", "API key value")
	apidefsParseCmd.Flags().StringVar(&apidefsParseAPIKeyIn, "api-key-in", "header", "API key location: header, query, cookie")
	apidefsParseCmd.Flags().StringVar(&apidefsParseBaseURL, "base-url", "", "Override base URL from definition")

	apidefsParseCmd.MarkFlagRequired("workspace")
}

func runAPIDefsParse(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "apidefs-parse").Logger()

	if apidefsParseURL == "" && apidefsParseFile == "" {
		logger.Error().Msg("Either --url or --file must be provided")
		os.Exit(1)
	}

	if apidefsParseURL != "" && apidefsParseFile != "" {
		logger.Error().Msg("Cannot provide both --url and --file")
		os.Exit(1)
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(apidefsParseWorkspaceID)
	if !workspaceExists {
		logger.Error().Uint("id", apidefsParseWorkspaceID).Msg("Workspace does not exist")
		os.Exit(1)
	}

	content, sourceURL, err := loadAPIDefinitionContent(apidefsParseURL, apidefsParseFile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load API definition")
		os.Exit(1)
	}

	apiType := detectAPIDefinitionType(content, sourceURL)
	logger.Info().Str("type", string(apiType)).Msg("Detected API type")

	var authConfig *db.APIAuthConfig
	if apidefsParseAuthType != "none" {
		authConfig, err = createAuthConfig(apidefsParseWorkspaceID, apiDefsAuthParams{
			AuthType:    apidefsParseAuthType,
			Username:    apidefsParseUsername,
			Password:    apidefsParsePassword,
			Token:       apidefsParseToken,
			APIKeyName:  apidefsParseAPIKeyName,
			APIKeyValue: apidefsParseAPIKeyValue,
			APIKeyIn:    apidefsParseAPIKeyIn,
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create auth config")
			os.Exit(1)
		}
		logger.Info().Str("auth_type", apidefsParseAuthType).Msg("Created auth configuration")
	}

	definition, err := parseAndPersistDefinition(content, sourceURL, apiType, apidefsParseWorkspaceID, authConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse and store API definition")
		os.Exit(1)
	}

	if apidefsParseName != "" {
		definition.Name = apidefsParseName
		db.Connection().UpdateAPIDefinition(definition)
	}

	if apidefsParseBaseURL != "" {
		definition.BaseURL = apidefsParseBaseURL
		db.Connection().UpdateAPIDefinition(definition)
	}

	logger.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Str("type", string(definition.Type)).
		Int("endpoints", definition.EndpointCount).
		Str("base_url", definition.BaseURL).
		Msg("API definition parsed and stored successfully")
}


