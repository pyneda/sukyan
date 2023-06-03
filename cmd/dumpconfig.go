
package cmd

import (
	"github.com/pyneda/sukyan/pkg/config"

	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dumpconfigCmd represents the dumpconfig command
var dumpconfigCmd = &cobra.Command{
	Use:   "dumpconfig",
	Short: "Dumps default configuration file",
	Long:  `Dumps default configuration file`,
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadConfig()
		err := viper.SafeWriteConfigAs("config.yml")
		if err != nil {
			log.Fatal().Err(err).Msg("Could not write config file")
		}
		log.Info().Msg("Config file written")
	},
}

func init() {
	rootCmd.AddCommand(dumpconfigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dumpconfigCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dumpconfigCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
