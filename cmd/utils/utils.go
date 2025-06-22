package utils

import (
	"github.com/spf13/cobra"
)

var workspaceID uint

// UtilsCmd represents the utils command
var UtilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Utility commands",
}
