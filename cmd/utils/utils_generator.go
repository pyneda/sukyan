package utils

import (
	"fmt"

	"github.com/pyneda/sukyan/lib/integrations"

	"time"

	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// generatorCmd represents the generator command
var generatorCmd = &cobra.Command{
	Use:   "payloads",
	Short: "Generate payloads using internal generator templates for testing purposes",
	Run: func(cmd *cobra.Command, args []string) {
		manager := integrations.InteractionsManager{
			GetAsnInfo:            false,
			PollingInterval:       time.Duration(5 * time.Second),
			OnInteractionCallback: TestInteractionCallback,
		}
		manager.Start()
		generators, _ := generation.LoadGenerators(viper.GetString("generators.directory"))
		log.Info().Msgf("Loaded %d payload generators", len(generators))
		for _, g := range generators {
			payloads, _ := g.BuildPayloads(manager)
			for _, p := range payloads {
				fmt.Println(p.Value)
			}
		}
	},
}

func init() {
	UtilsCmd.AddCommand(generatorCmd)
}
