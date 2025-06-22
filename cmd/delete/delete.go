package delete

import (
	"github.com/spf13/cobra"
)

var noConfirmDelete bool
var workspaceID uint

// DeleteCmd represents the describe command
var DeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"delete", "remove", "rm"},
	Short:   "Used to delete resources",
}

func init() {
	DeleteCmd.PersistentFlags().BoolVarP(&noConfirmDelete, "no-confirm", "y", false, "Do not ask for confirmation")
}
