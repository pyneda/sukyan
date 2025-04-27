package cmd

import (
	"fmt"
	"os"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string
var debugLogging bool
var prettyLogs bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sukyan",
	Short: `A web application vulnerability scanner`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		lib.ZeroConsoleAndFileLog()
		if debugLogging {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		db.Cleanup()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sukyan.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugLogging, "debug", false, "Use debug level logging")
	rootCmd.PersistentFlags().BoolVar(&prettyLogs, "pretty", true, "Use pretty logging instead JSON")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".sukyan" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".sukyan")

	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
