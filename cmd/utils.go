package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// utilsCmd represents the utils command
var utilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Utility commands",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("utils called")
	},
}

func init() {
	rootCmd.AddCommand(utilsCmd)
}
