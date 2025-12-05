package scanctl

import (
	"github.com/spf13/cobra"
)

// ScanCtlCmd represents the scan control command group
var ScanCtlCmd = &cobra.Command{
	Use:     "scanctl",
	Aliases: []string{"sc"},
	Short:   "Scan control commands",
	Long:    `Commands for controlling running scans: pause, resume, cancel.`,
}
