package update

import (
	"github.com/spf13/cobra"
)

// UpdateCmd groups commands that modify existing resources.
var UpdateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"u", "edit", "set"},
	Short:   "Used to modify existing resources",
}
