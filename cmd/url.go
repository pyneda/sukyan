package cmd

import (
	"fmt"
	"net/url"
	"os"
	"sukyan/pkg/web"

	"github.com/spf13/cobra"
)

// urlCmd represents the url command
var urlCmd = &cobra.Command{
	Use:   "url [url]",
	Short: "Inspect and scan a single URL",
	Long:  `Opens the URL in the browser and does few checks`,
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) == 0 {
			fmt.Println("Error: You must specify a target URL.")
			os.Exit(1)
		}

		parsedURL, err := url.ParseRequestURI(args[0])
		if err != nil {
			fmt.Printf("Error Invalid URL: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("url called: %s\n", parsedURL)
		data := web.InspectURL(parsedURL.String())
		data.LogPageData()
	},
}

func init() {
	rootCmd.AddCommand(urlCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// urlCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// urlCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
