package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/openapi"
	"github.com/spf13/cobra"
)

var openapiCmd = &cobra.Command{
	Use:   "openapi [url]",
	Short: "Fetch and parse an OpenAPI specification from a given URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		fuzz, _ := cmd.Flags().GetBool("fuzz")
		optional, _ := cmd.Flags().GetBool("optional")
		outputFile, _ := cmd.Flags().GetString("output")
		formatStr, _ := cmd.Flags().GetString("format")
		baseURL, _ := cmd.Flags().GetString("base-url")

		bodyBytes, err := fetchOpenAPISpec(url)
		if err != nil {
			return fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
		}

		doc, err := openapi.Parse(bodyBytes)
		if err != nil {
			return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
		}

		config := openapi.GenerationConfig{
			BaseURL:               doc.BaseURL(),
			IncludeOptionalParams: optional,
			FuzzingEnabled:        fuzz,
		}

		if baseURL != "" {
			config.BaseURL = baseURL
		}

		endpoints, err := openapi.GenerateRequests(doc, config)
		if err != nil {
			return fmt.Errorf("failed to generate requests: %w", err)
		}

		if outputFile != "" {
			// Determine format
			var format openapi.ReportFormat
			if formatStr == "json" {
				format = openapi.ReportFormatJSON
			} else if formatStr == "html" {
				format = openapi.ReportFormatHTML
			} else {
				// Default or auto-detect could go here, for now default to HTML if not specified or fallback
				format = openapi.ReportFormatHTML
			}

			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()

			if err := openapi.GenerateReport(endpoints, format, f); err != nil {
				return fmt.Errorf("failed to generate report: %w", err)
			}
			fmt.Printf("Report saved to %s\n", outputFile)
		} else {
			// Output as JSON to stdout
			output, err := json.MarshalIndent(endpoints, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal output: %w", err)
			}
			fmt.Println(string(output))
		}
		return nil
	},
}

func init() {
	openapiCmd.Flags().Bool("fuzz", false, "Enable fuzzing generation")
	openapiCmd.Flags().Bool("optional", false, "Include optional parameters")
	openapiCmd.Flags().StringP("output", "o", "", "Output file path")
	openapiCmd.Flags().StringP("format", "f", "html", "Report format (html or json)")
	openapiCmd.Flags().String("base-url", "", "Override the API base URL")
	rootCmd.AddCommand(openapiCmd)
}

func fetchOpenAPISpec(url string) ([]byte, error) {
	client := http_utils.CreateHttpClient()
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}
