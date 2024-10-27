package cmd

import (
	"fmt"
	"io"
	"net/http"

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
		formatFlag, _ := cmd.Flags().GetString("format")

		format, err := openapi.ValidateFormat(formatFlag)
		if err != nil {
			return fmt.Errorf("invalid format specified: %w", err)
		}

		bodyBytes, detectedFormat, err := fetchOpenAPISpec(url)
		if err != nil {
			return fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
		}

		finalFormat := format
		if finalFormat == "" {
			finalFormat = detectedFormat
			if finalFormat != "" {
				fmt.Printf("Detected format: %s\n", finalFormat)
			}
		}

		_, err = openapi.GenerateRequests(openapi.OpenapiParseInput{
			BodyBytes:  bodyBytes,
			SwaggerURL: url,
			Format:     string(finalFormat),
		})
		if err != nil {
			return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
		}

		fmt.Println("Spec parsed successfully!")
		return nil
	},
}

func init() {
	openapiCmd.Flags().StringP("format", "f", "", "Specification format (json, yaml, or js)")
	rootCmd.AddCommand(openapiCmd)
}

func fetchOpenAPISpec(url string) ([]byte, openapi.Format, error) {
	client := http_utils.CreateHttpClient()
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	format, err := openapi.DetectFormat(url, resp.Header, bodyBytes)
	if err != nil && err != openapi.ErrFormatDetection {
		return nil, "", fmt.Errorf("format detection failed, try providing it manually: %w", err)
	}

	return bodyBytes, format, nil
}
