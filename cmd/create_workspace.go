package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var newWorkspaceTitle string
var newWorkspaceCode string
var newWorkspaceDescription string

// createWorkspaceCmd represents the createWorkspace command
var createWorkspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Creates a new workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		if newWorkspaceCode == "" {
			return fmt.Errorf("workspace code cannot be empty")
		}
		if newWorkspaceTitle == "" {
			newWorkspaceTitle = newWorkspaceCode
		}

		if newWorkspaceDescription == "" {
			newWorkspaceDescription = fmt.Sprintf("Workspace for %s", newWorkspaceTitle)
		}

		workspace, err := db.Connection.GetWorkspaceByCode(newWorkspaceCode)
		if err != nil {
			workspace := db.Workspace{
				Title:       newWorkspaceTitle,
				Code:        newWorkspaceCode,
				Description: newWorkspaceDescription,
			}
			newWorkspace, err := db.Connection.CreateWorkspace(&workspace)
			if err != nil {
				return fmt.Errorf("error creating workspace: %v", err)
			}
			fmt.Println("Workspace created successfully!")
			fmt.Println("ID: ", newWorkspace.ID)
			return nil
		}
		fmt.Println("Workspace already exists!")
		fmt.Println("ID: ", workspace.ID)
		return nil
	},
}

func init() {
	createCmd.AddCommand(createWorkspaceCmd)

	createWorkspaceCmd.Flags().StringVarP(&newWorkspaceTitle, "title", "t", "", "Workspace title")
	createWorkspaceCmd.Flags().StringVarP(&newWorkspaceCode, "code", "c", "", "Workspace code")
	createWorkspaceCmd.Flags().StringVarP(&newWorkspaceDescription, "description", "d", "", "Workspace description")

}
