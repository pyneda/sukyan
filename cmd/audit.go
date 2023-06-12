package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/pkg/scan"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var auditCmdScanner scan.Scanner

// auditCmd represents the audit command
var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Full site audit, including crawl + scan each url",
	Long:  `Runs a configurable audit either to a simple url or to different sites if crawl and multiple initial urls domains are provided`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("audit called")

		if len(auditCmdScanner.StartUrls) == 0 {
			log.Error().Msg("At least one crawl starting url should be provided")
			os.Exit(1)
		}
		log.Info().Strs("urls", auditCmdScanner.StartUrls).Int("count", len(auditCmdScanner.StartUrls)).Msg("Starting the audit")
		// s := scan.Scanner{
		// 	StartUrls:       startUrls,
		// 	Depth:           depth,
		// 	MaxPagesToCrawl: maxPagesToCrawl,
		// 	PagesPoolSize:   pagesPoolSize,
		// 	ShouldCrawl:     true,
		// }
		auditCmdScanner.Run()
		log.Info().Msg("Audit finished, waiting for 30 seconds for possible interactions...")
		time.Sleep(30 * time.Second)
	},
}

func init() {
	rootCmd.AddCommand(auditCmd)
	auditCmd.Flags().StringArrayVarP(&auditCmdScanner.StartUrls, "url", "u", nil, "Target start url(s)")
	auditCmd.Flags().BoolVar(&auditCmdScanner.AuditHeaders, "audit-headers", true, "Audit HTTP headers")
	auditCmd.Flags().BoolVar(&auditCmdScanner.DiscoverParams, "param-discovery", true, "Enables parameter discovery (Not implemented yet)")
	auditCmd.Flags().BoolVar(&auditCmdScanner.ShouldCrawl, "crawl", false, "Enables the crawler")
	auditCmd.Flags().IntVar(&auditCmdScanner.PagesPoolSize, "pool-size", 10, "Page pool size (not used)")
	auditCmd.Flags().IntVar(&auditCmdScanner.Depth, "depth", 5, "Max crawl depth")
}
