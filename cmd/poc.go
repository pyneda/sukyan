package cmd

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	pocType    string
	outputFile string
)

var availablePocTypes = map[string]bool{
	"cswh": true,
}

func getAvailablePocTypes() []string {
	keys := make([]string, 0, len(availablePocTypes))
	for k := range availablePocTypes {
		keys = append(keys, k)
	}
	return keys
}

// pocCmd represents the generate-poc command
var pocCmd = &cobra.Command{
	Use:   "poc",
	Short: "Generate proofs of concept (PoCs) for various vulnerabilities",
	Run: func(cmd *cobra.Command, args []string) {
		if pocType == "" {
			fmt.Println("Please provide a PoC type")
			return
		}

		if _, valid := availablePocTypes[pocType]; !valid {
			fmt.Printf("Invalid PoC type: %s. Valid types are: %v\n", pocType, getAvailablePocTypes())
			return
		}

		if filterWebSocketConnectionID == 0 {
			fmt.Println("Please provide a WebSocket connection ID")
			return
		}

		var buf bytes.Buffer
		var err error

		switch pocType {
		case "cswh":
			connection, err := db.Connection().GetWebSocketConnection(filterWebSocketConnectionID)
			if err != nil {
				fmt.Printf("Error fetching WebSocket connection: %v\n", err)
				return
			}

			interactManager := integrations.InteractionsManager{
				PollingInterval:       10 * time.Second,
				OnInteractionCallback: TestInteractionCallback,
			}

			interactManager.Start()
			defer interactManager.Stop()
			interactionURL := interactManager.GetURL().URL
			buf, err = manual.GenerateCrossSiteWebsocketHijackingPoC(*connection, interactionURL)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate CSWH PoC")
				fmt.Println("Failed to generate CSWH PoC")
				return
			}

		default:
			fmt.Printf("Unknown PoC type: %s\n", pocType)
			return
		}

		if outputFile == "" {
			outputFile = fmt.Sprintf("%s-poc.html", lib.Slugify(pocType))
		}

		err = os.WriteFile(outputFile, buf.Bytes(), os.ModePerm)
		if err != nil {
			fmt.Printf("Failed to write PoC to file: %v\n", err)
			return
		}

		fmt.Printf("PoC generated and saved to %s\n", outputFile)

		fmt.Println("Waiting for interactions. Press Ctrl+C to stop.")
		select {}
	},
}

func init() {
	rootCmd.AddCommand(pocCmd)

	pocCmd.Flags().UintVarP(&filterWebSocketConnectionID, "connection-id", "c", 0, "WebSocket connection ID")
	pocCmd.Flags().StringVarP(&pocType, "type", "t", "", fmt.Sprintf("Type (available: %v)", getAvailablePocTypes()))
	pocCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path")
}
