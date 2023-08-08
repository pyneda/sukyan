package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/pkg/crawl"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var startUrls []string
var depth int
var maxPagesToCrawl int
var pagesPoolSize int

// crawlCmd represents the crawl command
var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Crawl the provided site",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("crawl called")
		if len(startUrls) == 0 {
			log.Error().Msg("At least one crawl starting url should be provided")
			os.Exit(1)
		}
		log.Info().Strs("startUrls", startUrls).Int("count", len(startUrls)).Msg("Creating and scheduling the crawler")
		crawler := crawl.NewCrawler(startUrls, maxPagesToCrawl, depth, pagesPoolSize, crawlExcludePatterns, workspaceID)
		crawler.Run()
	},
}

func init() {
	rootCmd.AddCommand(crawlCmd)
	crawlCmd.Flags().StringArrayVar(&startUrls, "url", nil, "Target start url(s)")
	crawlCmd.Flags().StringArrayVar(&crawlExcludePatterns, "exclude-pattern", nil, "URL patterns to ignore when crawling")
	crawlCmd.Flags().IntVar(&pagesPoolSize, "pool-size", 4, "Page pool size")
	crawlCmd.Flags().IntVar(&depth, "depth", 0, "Max crawl depth")
	crawlCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
}
