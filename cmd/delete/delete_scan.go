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

// deleteScanCmd represents the delete scan command
var deleteScanCmd = &cobra.Command{
	Use:        "scan [id]",
	Aliases:    []string{"s"},
	Short:      "Delete a scan",
	Long:       `Deletes a scan and all its associated jobs`,
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"id"},
	Run: func(cmd *cobra.Command, args []string) {
		deleteScanID, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid ID provided")
			os.Exit(0)
		}
		if deleteScanID == 0 {
			fmt.Println("An ID needs to be provided")
			os.Exit(0)
		}
		scan, err := db.Connection().GetScanByID(uint(deleteScanID))
		if err != nil {
			fmt.Println("Could not find a scan with the provided ID")
			os.Exit(0)
		}

		fmt.Printf("Deleting the following scan:\n  - ID: %d\n  - Title: %s\n  - Status: %s\n  - Workspace ID: %d\n\n", scan.ID, scan.Title, scan.Status, scan.WorkspaceID)

		if !noConfirmDelete {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("WARNING: This will delete the scan and all associated jobs. This action cannot be undone.")
			fmt.Print("\nAre you sure you want to proceed with deletion? (yes/no): ")
			confirmation, _ := reader.ReadString('\n')
			confirmation = strings.TrimSpace(confirmation)

			if confirmation != "yes" {
				fmt.Println("Deletion aborted.")
				return
			}
		}

		err = db.Connection().DeleteScan(uint(deleteScanID))
		if err != nil {
			fmt.Printf("Error during deletion: %s\n", err)
		} else {
			fmt.Println("Scan has been successfully deleted!")
		}
	},
}

func init() {
	DeleteCmd.AddCommand(deleteScanCmd)
}
