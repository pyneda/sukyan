package create

import (
	"github.com/spf13/cobra"
)

var workspaceID uint

// CreateCmd represents the describe command
var CreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c", "create", "add"},
	Short:   "Used to persist resources",
	// Run: func(cmd *cobra.Command, args []string) {
	// 	fmt.Println("describe called")
	// },
}
