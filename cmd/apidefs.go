package cmd

import (
	"github.com/spf13/cobra"
)

var apidefsCmd = &cobra.Command{
	Use:   "apidefs",
	Short: "API definition management and scanning",
	Long: `Commands for importing, managing, and scanning API definitions.

Supports:
  - OpenAPI (Swagger) specifications
  - GraphQL endpoints
  - WSDL (SOAP) services

Examples:
  # Scan an OpenAPI definition
  sukyan apidefs scan --url https://api.example.com/openapi.json -w 1

  # Parse and store without scanning
  sukyan apidefs parse --url https://api.example.com/openapi.json -w 1

  # List stored definitions
  sukyan apidefs list -w 1

  # Show definition details
  sukyan apidefs show <definition-id>`,
}

func init() {
	rootCmd.AddCommand(apidefsCmd)
}
