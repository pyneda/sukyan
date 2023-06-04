package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/lib"
	"os"

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
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	// consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout}

	// multi := zerolog.MultiLevelWriter(consoleWriter, os.Stdout)
	// log.Logger = zerolog.New(multi).With().Timestamp().Logger()

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sukyan.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.PersistentFlags().BoolVar(&debugLogging, "debug", false, "Use debug level logging")
	rootCmd.PersistentFlags().BoolVar(&prettyLogs, "pretty", true, "Use pretty logging instead JSON")

	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		lib.ZeroConsoleAndFileLog("logs.log")
		if debugLogging {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
		return nil
	}
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
		viper.SetConfigName(".sukyan")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
