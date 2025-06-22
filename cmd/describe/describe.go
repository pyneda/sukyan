package describe

import (
	"github.com/spf13/cobra"
)

var format string

// DescribeCmd represents the describe command
var DescribeCmd = &cobra.Command{
	Use:     "describe",
	Aliases: []string{"d", "desc", "show"},
	Short:   "Describes a resource stored in the database",
	Long:    `Describes a resource.`,
}

func init() {
	DescribeCmd.PersistentFlags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, table, text, pretty)")

}
