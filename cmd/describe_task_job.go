package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/cobra"
)

var describeTaskJobCmd = &cobra.Command{
	Use:        "task-job [id]",
	Aliases:    []string{"tj", "job"},
	Short:      "Get details of a task job",
	Long:       "Get details of a task job by its ID",
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		taskJobID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if taskJobID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}

		taskJob, err := db.Connection().GetTaskJobByID(uint(taskJobID))
		if err != nil {
			fmt.Println("Could not find a task job with the provided ID")
			os.Exit(0)
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}

		formattedOutput, err := lib.FormatSingleOutput(taskJob, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeTaskJobCmd)
}
