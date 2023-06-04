package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var fuzzParams []string
var fuzzURL string
var fuzzPayloadsWordlist string
var fuzzConcurrency int
var fuzzAllParams bool

// fuzzCmd represents the fuzz command
var fuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "Fuzz url parameters",
	Long:  `Given a payloads file, it test them against the url parameters.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("fuzz called")
		if len(targets) == 0 {
			fmt.Println("At least one target needs to be provided")
			os.Exit(1)
		}
		if _, err := os.Stat(wordlist); os.IsNotExist(err) {
			fmt.Printf("Wordlist does not exist: %s\n", wordlist)
			os.Exit(1)
		}

	},
}

func init() {
	rootCmd.AddCommand(fuzzCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// fuzzCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// fuzzCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	fuzzCmd.Flags().StringVar(&fuzzURL, "url", "", "Target start url(s)")
	fuzzCmd.Flags().StringVar(&fuzzPayloadsWordlist, "payloads", "default.txt", "Payloads wordlist")
	fuzzCmd.Flags().IntVar(&fuzzConcurrency, "concurrency", 20, "Fuzz workers concurrency")
	fuzzCmd.Flags().StringArrayVar(&fuzzParams, "params", nil, "Force testing this parameters, if not provided, existing url parameters will be tested")
	fuzzCmd.Flags().BoolVar(&fuzzAllParams, "all-params", false, "Force testing all parameters (provided and existing in the path)")
}
