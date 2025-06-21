package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/spf13/cobra"
)

var describeBrowserActionsCmd = &cobra.Command{
	Use:        "browser-actions [id]",
	Aliases:    []string{"browser-action", "ba"},
	Short:      "Get details of browser actions",
	Long:       `Get details of stored browser actions.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		describeBrowserActionsID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if describeBrowserActionsID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		browserActions, err := db.Connection().GetStoredBrowserActionsByID(uint(describeBrowserActionsID))
		if err != nil {
			fmt.Println("Could not find browser actions with the provided ID")
			os.Exit(0)
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(browserActions, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeBrowserActionsCmd)
}
