package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/openapi"
	"github.com/spf13/cobra"
)

// openapiCmd represents the OpenAPI fetch and parse command
var openapiCmd = &cobra.Command{
	Use:   "openapi [url]",
	Short: "Fetch and parse an OpenAPI specification from a given URL",
	Args:  cobra.ExactArgs(1), // Expect exactly one argument (the OpenAPI URL)
	Run: func(cmd *cobra.Command, args []string) {
		url := args[0]
		fmt.Printf("Fetching OpenAPI spec from: %s\n", url)

		// Fetch the OpenAPI spec
		bodyBytes, err := fetchOpenAPISpec(url)
		if err != nil {
			fmt.Printf("Error fetching OpenAPI spec: %v\n", err)
			os.Exit(1)
		}

		// Parse the OpenAPI spec
		_, err = openapi.GenerateRequests(bodyBytes, url)
		if err != nil {
			fmt.Printf("Error parsing OpenAPI spec: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Spec parsed successfully!")
		// for _, result := range results {
		// 	fmt.Println(result)
		// }
	},
}

// Initializes the Cobra command
func init() {
	rootCmd.AddCommand(openapiCmd)
}

// Fetches the OpenAPI spec from a URL and returns the response body
func fetchOpenAPISpec(url string) ([]byte, error) {
	client := http_utils.CreateHttpClient()
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenAPI spec from URL: %w", err)
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
