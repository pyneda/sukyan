package utils

import (
	//_ "embed"

	"os"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
)

var wordlist string
var targets []string
var testParams []string
var urlEncode bool
var urlFile string

// xssCmd represents the xss command
var xssCmd = &cobra.Command{
	Use:   "xss",
	Short: "Test a list of XSS payloads against a parameter",
	Args: func(cmd *cobra.Command, urls []string) error {

		return nil
	},
	Run: func(cmd *cobra.Command, urls []string) {

		if workspaceID == 0 {
			log.Error().Msg("Workspace ID is required")
			os.Exit(1)
		}

		if urlFile != "" {
			urlsFromFile, err := lib.ReadFileByLines(urlFile)
			if err != nil {
				log.Error().Err(err).Msg("Failed to read URLs from file")
				os.Exit(1)
			}
			targets = append(targets, urlsFromFile...)
		}

		targets = lib.GetUniqueItems(targets)

		if len(targets) == 0 {
			log.Error().Msg("At least one target url should be provided")
			os.Exit(1)
		}

		if _, err := os.Stat(wordlist); os.IsNotExist(err) {
			log.Warn().Str("wordlist", wordlist).Msg("Wordlist does not exist")
			os.Exit(1)
		}
		log.Info().Msg("Starting XSS testing")
		for _, target := range targets {
			xss := active.XSSAudit{
				WorkspaceID: workspaceID,
			}
			xss.Run(target, testParams, wordlist, urlEncode)
			log.Info().Str("url", target).Msg("XSS audit completed")
		}

	},
}

func init() {
	UtilsCmd.AddCommand(xssCmd)
	xssCmd.Flags().BoolP("screenshot", "s", false, "Screenshot when an XSS is validated")
	xssCmd.Flags().BoolVarP(&urlEncode, "encode", "e", false, "URL encode the whole path (including the payload)")
	xssCmd.Flags().StringVar(&wordlist, "wordlist", "default.txt", "XSS payloads wordlist")
	xssCmd.Flags().StringArrayVarP(&targets, "url", "u", nil, "Targets")
	xssCmd.Flags().StringVarP(&urlFile, "file", "f", "", "File containing multiple URLs to scan")
	xssCmd.Flags().StringSliceVarP(&testParams, "params", "p", testParams, "Parameters to test.")
	xssCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
}
