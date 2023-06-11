package cmd

import (
	"fmt"
	"github.com/projectdiscovery/interactsh/pkg/server"
	"os"
	"os/signal"
	"time"

	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/spf13/cobra"
)

func TestInteractionCallback(interaction *server.Interaction) {
	fmt.Printf("Got Interaction: %v => %v => %v => %v\n", interaction.Protocol, interaction.FullId, interaction.UniqueID, interaction.RemoteAddress)
}

// interactCmd represents the interact command
var interactCmd = &cobra.Command{
	Use:   "interact",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting the client")
		manager := integrations.InteractionsManager{
			GetAsnInfo:            false,
			PollingInterval:       time.Duration(5 * time.Second),
			OnInteractionCallback: TestInteractionCallback,
		}
		manager.Start()
		url := manager.GetURL()
		fmt.Println(url.URL)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		for range c {
			fmt.Println("Closing...")
			manager.Stop()
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(interactCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// interactCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// interactCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
