package cmd

import (
	"github.com/spf13/cobra"
)

// utilsCmd represents the utils command
var utilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Utility commands",
}

func init() {
	rootCmd.AddCommand(utilsCmd)
}
