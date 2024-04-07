package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// describeWorkspaceCmd represents the workspace command
var describeWorkspaceCmd = &cobra.Command{
	Use:        "workspace [id]",
	Aliases:    []string{"w"},
	Short:      "Get details of a workspace",
	Long:       `List workspace details.`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		describeWorkspaceID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if describeWorkspaceID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		workspace, err := db.Connection.GetWorkspaceByID(uint(describeWorkspaceID))
		if err != nil {
			log.Panic().Err(err).Msg("Could not find a workspace with the provided ID")
		}
		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			fmt.Println("Error parsing format type")
			os.Exit(0)
		}
		formattedOutput, err := lib.FormatSingleOutput(workspace, formatType)
		if err != nil {
			fmt.Println("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	describeCmd.AddCommand(describeWorkspaceCmd)
}
