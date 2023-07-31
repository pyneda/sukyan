package cmd

import (
	"github.com/spf13/cobra"
)

// createCmd represents the describe command
var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c", "create", "add"},
	Short:   "Used to persist resources",
	// Run: func(cmd *cobra.Command, args []string) {
	// 	fmt.Println("describe called")
	// },
}

func init() {
	rootCmd.AddCommand(createCmd)
}
