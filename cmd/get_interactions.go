package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	qTypes    []string
	protocols []string
	fullIDs   []string
)

// getInteractionsCmd representa el comando para obtener interacciones
var getInteractionsCmd = &cobra.Command{
	Use:     "interactions",
	Aliases: []string{"interaction", "int", "oob"},
	Short:   "List out-of-band interactions",
	Run: func(cmd *cobra.Command, args []string) {
		filters := db.InteractionsFilter{
			QTypes:      qTypes,
			Protocols:   protocols,
			FullIDs:     fullIDs,
			Pagination:  db.Pagination{PageSize: pageSize, Page: page},
			WorkspaceID: workspaceID,
		}

		interactions, count, err := db.Connection.ListInteractions(filters)
		if err != nil {
			log.Error().Err(err).Msg("Error listing interactions")
			return
		}

		if count == 0 {
			fmt.Println("No interactions found")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(interactions, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(getInteractionsCmd)
	getInteractionsCmd.Flags().StringSliceVarP(&qTypes, "qtype", "t", []string{}, "Filter by qtype. Can be added multiple times.")
	getInteractionsCmd.Flags().StringSliceVar(&protocols, "protocol", []string{}, "Filter by protocol. Can be added multiple times.")
	getInteractionsCmd.Flags().StringSliceVar(&fullIDs, "full-id", []string{}, "Filter by OOB server interaction full ID. Can be added multiple times.")
}
