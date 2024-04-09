package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

// describeTaskCmd represents the task command
var describeTaskCmd = &cobra.Command{
	Use:        "task [id]",
	Aliases:    []string{"t"},
	Short:      "Get details of a task",
	Long:       `List task details.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		describeTaskID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if describeTaskID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		task, err := db.Connection.GetTaskByID(uint(describeTaskID))
		if err != nil {
			fmt.Println("Could not find a task with the provided ID")
			os.Exit(0)
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(task, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeTaskCmd)
}
