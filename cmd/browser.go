package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/pkg/manual"

	"github.com/spf13/cobra"
)

// browserCmd represents the browser command
var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Launch a browser that records all traffic",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("browser called")
		browser := manual.UserBrowser{}
		browser.Launch()
	},
}

func init() {
	rootCmd.AddCommand(browserCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// browserCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// browserCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
