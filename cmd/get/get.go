package get

import (
	"github.com/spf13/cobra"
)

var (
	pageSize int
	page     int
	format   string
)

// GetCmd represents the get command
var GetCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"g", "list", "ls"},
	Short:   "List resources",
	Long:    `Get is used to retrieve resources like workspaces, issues, etc.`,
}

func init() {
	GetCmd.PersistentFlags().IntVarP(&pageSize, "page-size", "s", 100, "Size of each page")
	GetCmd.PersistentFlags().IntVarP(&page, "page", "p", 1, "Page number")
	GetCmd.PersistentFlags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, table, text, pretty)")
}
