package delete

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

// deleteWorkspaceCmd representa el comando workspace
var deleteWorkspaceCmd = &cobra.Command{
	Use:        "workspace [id]",
	Aliases:    []string{"w"},
	Short:      "Delete a workspace",
	Long:       `Deletes a workspace and all associated data`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		deleteWorkspaceID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if deleteWorkspaceID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		workspace, err := db.Connection().GetWorkspaceByID(uint(deleteWorkspaceID))
		if err != nil {
			fmt.Println("Could not find a workspace with the provided ID")
			os.Exit(0)
		}

		fmt.Printf("Deleting the following workspace:\n  - ID: %d\n  - Code: %s\n  - Title: %s\n\n", workspace.ID, workspace.Code, workspace.Title)

		if !noConfirmDelete {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("WARNING: This will delete the workspace and all associated data such as detected issues, tasks, history, etc. This action cannot be undone.")
			fmt.Print("\nAre you sure you want to proceed with deletion? (yes/no): ")
			confirmation, _ := reader.ReadString('\n')
			confirmation = strings.TrimSpace(confirmation)

			if confirmation != "yes" {
				fmt.Println("Deletion aborted.")
				return
			}
		}

		err = db.Connection().DeleteWorkspace(uint(deleteWorkspaceID))
		if err != nil {
			fmt.Printf("Error during deletion: %s\n", err)
		} else {
			fmt.Println("Workspace has been successfully deleted!")
		}
	},
}

func init() {
	DeleteCmd.AddCommand(deleteWorkspaceCmd)
}
