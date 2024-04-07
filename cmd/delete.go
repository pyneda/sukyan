package cmd

import (
	"github.com/spf13/cobra"
)

// deleteCmd represents the describe command
var deleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"delete", "remove", "rm"},
	Short:   "Used to delete resources",
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
