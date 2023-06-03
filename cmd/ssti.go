
package cmd

import (
	"fmt"
	"net/url"
	"os"
	"github.com/pyneda/sukyan/pkg/active"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var sstiAuditor active.SSTIAudit

// sstiCmd represents the ssti command
var sstiCmd = &cobra.Command{
	Use:   "ssti",
	Short: "Perform SSTI audit",
	Long:  `Performs a SSTI audit against the provided URL parameters.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ssti called")
		if sstiAuditor.URL == "" {
			log.Error().Msg("An URL must be provided")
			os.Exit(1)
		}
		_, err := url.ParseRequestURI(sstiAuditor.URL)
		if err != nil {
			log.Error().Str("url", sstiAuditor.URL).Msg("Invalid URL provided")
			os.Exit(1)
		}
		sstiAuditor.Run()
	},
}

func init() {
	rootCmd.AddCommand(sstiCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// sstiCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// sstiCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	sstiCmd.Flags().StringVar(&sstiAuditor.URL, "url", "", "URL to test")
	sstiCmd.Flags().IntVar(&sstiAuditor.Concurrency, "concurrency", 20, "Max concurrency")
	sstiCmd.Flags().StringArrayVar(&sstiAuditor.Params, "params", nil, "Parameters to test, will be used the ones from the url if not provided")
	sstiCmd.Flags().BoolVar(&sstiAuditor.StopAfterSuccess, "stop-after-success", false, "Not implemented stop after success switch")
}
