package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/lib/integrations"

	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

// generatorCmd represents the generator command
var generatorCmd = &cobra.Command{
	Use:   "generator",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("generator called")
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
				// p.Print()
				fmt.Println(p.Value)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(generatorCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generatorCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// generatorCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
