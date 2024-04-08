package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:     "describe",
	Aliases: []string{"d", "desc", "show"},
	Short:   "Describes a resource stored in the database",
	Long:    `Describes a resource.`,
}

func init() {
	rootCmd.AddCommand(describeCmd)
	describeCmd.PersistentFlags().StringVarP(&format, "format", "f", "json", "Output format (json, yaml, table, text, pretty)")

}
