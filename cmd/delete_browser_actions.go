package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var deleteBrowserActionsCmd = &cobra.Command{
	Use:        "browser-actions [id]",
	Aliases:    []string{"browser-action", "ba"},
	Short:      "Delete browser actions",
	Long:       `Deletes stored browser actions`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		deleteBrowserActionsID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if deleteBrowserActionsID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		browserActions, err := db.Connection().GetStoredBrowserActionsByID(uint(deleteBrowserActionsID))
		if err != nil {
			fmt.Println("Could not find browser actions with the provided ID")
			os.Exit(0)
		}

		fmt.Printf("Deleting the following browser actions:\n  - ID: %d\n  - Title: %s\n  - Scope: %s\n  - Actions Count: %d\n\n",
			browserActions.ID, browserActions.Title, browserActions.Scope, len(browserActions.Actions))

		if !noConfirmDelete {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("WARNING: This will delete the browser actions. This action cannot be undone.")
			fmt.Print("\nAre you sure you want to proceed with deletion? (yes/no): ")
			confirmation, _ := reader.ReadString('\n')
			confirmation = strings.TrimSpace(confirmation)

			if confirmation != "yes" {
				fmt.Println("Deletion aborted.")
				return
			}
		}

		err = db.Connection().DeleteStoredBrowserActions(uint(deleteBrowserActionsID))
		if err != nil {
			fmt.Printf("Error during deletion: %s\n", err)
		} else {
			fmt.Println("Browser actions have been successfully deleted!")
		}
	},
}

func init() {
	deleteCmd.AddCommand(deleteBrowserActionsCmd)
}
