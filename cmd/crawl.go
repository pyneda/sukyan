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
		// crawler := crawl.Crawler{
		// 	StartUrls:       startUrls,
		// 	Depth:           depth,
		// 	MaxPagesToCrawl: maxPagesToCrawl,
		// 	PagesPoolSize:   pagesPoolSize,
		// }
		crawler := crawl.NewCrawler(startUrls, maxPagesToCrawl, depth, pagesPoolSize)
		crawler.Run()
	},
}

func init() {
	rootCmd.AddCommand(crawlCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// crawlCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// crawlCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	crawlCmd.Flags().StringArrayVar(&startUrls, "url", nil, "Target start url(s)")
	crawlCmd.Flags().IntVar(&pagesPoolSize, "pool-size", 4, "Page pool size")
	crawlCmd.Flags().IntVar(&depth, "depth", 0, "Max crawl depth")
}
