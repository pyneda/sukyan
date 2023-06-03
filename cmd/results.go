
package cmd

import (
	"sukyan/db"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var filterIssueCodes []string

// resultsCmd represents the results command
var resultsCmd = &cobra.Command{
	Use:   "results",
	Short: "List existing database results",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		issues, _, err := db.Connection.ListIssues(db.IssueFilter{
			Codes: filterIssueCodes,
		})
		if err != nil {
			log.Error().Err(err).Msg("Error received trying to get issues from db")
		}
		db.PrintIssueTable(issues)
		// for _, issue := range issues {
		// 	log.Info().Interface("issue", issue).Msg("Issue from db")

		// }
	},
}

func init() {
	rootCmd.AddCommand(resultsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// resultsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// resultsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	resultsCmd.Flags().StringSliceVarP(&filterIssueCodes, "code", "c", []string{}, "Filter by issue code. Can be added multiple times.")
}
