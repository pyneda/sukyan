package create

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/spf13/cobra"
)

var newBrowserActionsTitle string
var newBrowserActionsFile string
var newBrowserActionsScope string

var createBrowserActionsCmd = &cobra.Command{
	Use:     "browser-actions",
	Aliases: []string{"browser-action", "ba"},
	Short:   "Creates new browser actions",
	RunE: func(cmd *cobra.Command, args []string) error {
		var browserActions actions.BrowserActions
		var err error

		if newBrowserActionsFile != "" {
			browserActions, err = actions.LoadBrowserActions(newBrowserActionsFile)
			if err != nil {
				return fmt.Errorf("error loading browser actions from file: %v", err)
			}
		} else {
			if newBrowserActionsTitle == "" {
				return fmt.Errorf("title is required when not using a file")
			}
			browserActions = actions.BrowserActions{
				Title:   newBrowserActionsTitle,
				Actions: []actions.Action{},
			}
		}

		if newBrowserActionsTitle != "" && newBrowserActionsFile != "" {
			browserActions.Title = newBrowserActionsTitle
		}

		if newBrowserActionsScope == "" {
			newBrowserActionsScope = "global"
		}

		scope := db.BrowserActionScope(newBrowserActionsScope)
		if scope != db.BrowserActionScopeGlobal && scope != db.BrowserActionScopeWorkspace {
			return fmt.Errorf("scope must be either 'global' or 'workspace'")
		}

		var workspaceIDPtr *uint
		if scope == db.BrowserActionScopeWorkspace {
			if workspaceID == 0 {
				return fmt.Errorf("workspace ID is required when scope is 'workspace'")
			}
			workspaceExists, _ := db.Connection().WorkspaceExists(workspaceID)
			if !workspaceExists {
				return fmt.Errorf("workspace with ID %d does not exist", workspaceID)
			}
			workspaceIDPtr = &workspaceID
		}

		storedBrowserActions := db.StoredBrowserActions{
			Title:       browserActions.Title,
			Actions:     browserActions.Actions,
			Scope:       scope,
			WorkspaceID: workspaceIDPtr,
		}

		createdBrowserActions, err := db.Connection().CreateStoredBrowserActions(&storedBrowserActions)
		if err != nil {
			return fmt.Errorf("error creating browser actions: %v", err)
		}

		fmt.Println("Browser actions created successfully!")
		fmt.Printf("ID: %d\n", createdBrowserActions.ID)
		fmt.Printf("Title: %s\n", createdBrowserActions.Title)
		fmt.Printf("Scope: %s\n", createdBrowserActions.Scope)
		if createdBrowserActions.WorkspaceID != nil {
			fmt.Printf("Workspace ID: %d\n", *createdBrowserActions.WorkspaceID)
		}
		fmt.Printf("Actions Count: %d\n", len(createdBrowserActions.Actions))
		return nil
	},
}

func init() {
	CreateCmd.AddCommand(createBrowserActionsCmd)

	createBrowserActionsCmd.Flags().StringVarP(&newBrowserActionsTitle, "title", "t", "", "Browser actions title")
	createBrowserActionsCmd.Flags().StringVarP(&newBrowserActionsFile, "file", "f", "", "Load browser actions from YAML file")
	createBrowserActionsCmd.Flags().StringVarP(&newBrowserActionsScope, "scope", "s", "global", "Scope (global or workspace)")
	createBrowserActionsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID (required when scope is workspace)")
}
