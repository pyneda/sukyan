package cmd

import (
	//_ "embed"
	"fmt"
	"os"

	"github.com/pyneda/sukyan/pkg/active"

	"github.com/spf13/cobra"
)

var wordlist string
var targets []string
var testParams []string
var urlEncode bool

// https://tip.golang.org/pkg/embed/
// go  :embed "payloads.txt"
// var payloads []byte

// xssCmd represents the xss command
var xssCmd = &cobra.Command{
	Use:   "xss",
	Short: "Test a list of XSS payloads against a parameter",
	Args: func(cmd *cobra.Command, urls []string) error {

		return nil
	},
	Run: func(cmd *cobra.Command, urls []string) {
		if len(targets) == 0 {
			fmt.Println("At least one target needs to be provided")
			os.Exit(1)
		}
		if _, err := os.Stat(wordlist); os.IsNotExist(err) {
			fmt.Printf("Wordlist does not exist: %s\n", wordlist)
			os.Exit(1)
		}
		fmt.Println("Checking XSS with payloads...")
		for _, target := range targets {
			xss := active.XSSAudit{
				WorkspaceID: workspaceID,
			}
			xss.Run(target, testParams, wordlist, urlEncode)
		}

	},
}

func init() {
	utilsCmd.AddCommand(xssCmd)
	xssCmd.Flags().BoolP("screenshot", "s", false, "Screenshot when an XSS is validated")
	xssCmd.Flags().BoolVarP(&urlEncode, "encode", "e", false, "URL encode the whole path (including the payload)")
	xssCmd.Flags().StringVar(&wordlist, "wordlist", "default.txt", "XSS payloads wordlist")
	xssCmd.Flags().StringArrayVarP(&targets, "url", "u", nil, "Targets")
	xssCmd.Flags().StringSliceVarP(&testParams, "params", "p", testParams, "Parameters to test.")
	xssCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
}
