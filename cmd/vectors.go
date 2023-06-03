
package cmd

import (
	"fmt"
	"net/url"
	"os"
	"sukyan/pkg/fuzz"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var vectorsURL string

// vectorsCmd represents the vectors command
var vectorsCmd = &cobra.Command{
	Use:   "vectors",
	Short: "Command to test injection points",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if vectorsURL == "" {
			log.Error().Msg("An URL must be provided")
			os.Exit(1)
		}
		_, err := url.ParseRequestURI(vectorsURL)
		if err != nil {
			log.Error().Err(err).Str("url", vectorsURL).Msg("Invalid URL provided")
			os.Exit(1)
		}
		g := fuzz.InjectionPointGatherer{
			ParamsExtensive: true,
		}
		vectors := g.GetFromURL(vectorsURL)
		for _, vector := range vectors {
			// log.Info().Interface("vector", vector).Msg("Vector")
			fmt.Println(vector.URL)
		}

	},
}

func init() {
	rootCmd.AddCommand(vectorsCmd)
	vectorsCmd.Flags().StringVarP(&vectorsURL, "url", "u", "", "URL")
}
