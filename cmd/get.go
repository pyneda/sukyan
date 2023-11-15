package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var (
	pageSize int
	page     int
	format   string
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "List resources",
	Long:  `Get is used to retrieve resources like workspaces, issues, etc.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("get called")
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.PersistentFlags().IntVarP(&pageSize, "page-size", "s", 100, "Size of each page")
	getCmd.PersistentFlags().IntVarP(&page, "page", "p", 1, "Page number")
	getCmd.PersistentFlags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, text, pretty)")
}
