package cmd

import (
	"github.com/spf13/cobra"
)

var (
	pageSize int
	page     int
	format   string
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"g", "list", "ls"},
	Short:   "List resources",
	Long:    `Get is used to retrieve resources like workspaces, issues, etc.`,
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.PersistentFlags().IntVarP(&pageSize, "page-size", "s", 100, "Size of each page")
	getCmd.PersistentFlags().IntVarP(&page, "page", "p", 1, "Page number")
	getCmd.PersistentFlags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, table, text, pretty)")
}
