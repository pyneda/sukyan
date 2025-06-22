package cmd

import (
	"github.com/pyneda/sukyan/cmd/create"
	"github.com/pyneda/sukyan/cmd/delete"
	"github.com/pyneda/sukyan/cmd/describe"
	"github.com/pyneda/sukyan/cmd/get"
	"github.com/pyneda/sukyan/cmd/stats"
)

func init() {
	rootCmd.AddCommand(get.GetCmd)
	rootCmd.AddCommand(describe.DescribeCmd)
	rootCmd.AddCommand(stats.StatsCmd)
	rootCmd.AddCommand(delete.DeleteCmd)
	rootCmd.AddCommand(create.CreateCmd)
}
