package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	oobTestNames          []string
	oobTargets            []string
	oobInteractionDomains []string
	oobInteractionFullIDs []string
	oobPayloads           []string
	oobInsertionPoints    []string
	oobCodes              []string
)

// getOOBTestsCmd represents the command for getting OOB tests
var getOOBTestsCmd = &cobra.Command{
	Use:     "oob-tests",
	Aliases: []string{"oob-test", "oobt", "oob"},
	Short:   "List out-of-band tests",
	Run: func(cmd *cobra.Command, args []string) {
		filters := db.OOBTestsFilter{
			TestNames:          oobTestNames,
			Targets:            oobTargets,
			InteractionDomains: oobInteractionDomains,
			InteractionFullIDs: oobInteractionFullIDs,
			Payloads:           oobPayloads,
			InsertionPoints:    oobInsertionPoints,
			Codes:              oobCodes,
			Pagination:         db.Pagination{PageSize: pageSize, Page: page},
			WorkspaceID:        workspaceID,
			TaskID:             filterTaskID,
			TaskJobID:          filterTaskJobID,
		}

		oobTests, count, err := db.Connection().ListOOBTests(filters)
		if err != nil {
			log.Error().Err(err).Msg("Error listing OOB tests")
			return
		}

		if count == 0 {
			fmt.Println("No OOB tests found")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(oobTests, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(getOOBTestsCmd)
	getOOBTestsCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	getOOBTestsCmd.Flags().UintVarP(&filterTaskID, "task", "t", 0, "Task ID")
	getOOBTestsCmd.Flags().UintVarP(&filterTaskJobID, "task-job", "j", 0, "Task Job ID")
	getOOBTestsCmd.Flags().StringSliceVar(&oobTestNames, "test-name", []string{}, "Filter by test name. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobTargets, "target", []string{}, "Filter by target URL. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobInteractionDomains, "interaction-domain", []string{}, "Filter by interaction domain. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobInteractionFullIDs, "interaction-full-id", []string{}, "Filter by interaction full ID. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobPayloads, "payload", []string{}, "Filter by payload. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobInsertionPoints, "insertion-point", []string{}, "Filter by insertion point. Can be added multiple times.")
	getOOBTestsCmd.Flags().StringSliceVar(&oobCodes, "code", []string{}, "Filter by issue code. Can be added multiple times.")
}
