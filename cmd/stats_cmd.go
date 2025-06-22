package cmd

import (
	"github.com/pyneda/sukyan/cmd/stats"
)

func init() {
	rootCmd.AddCommand(stats.StatsCmd)
}
