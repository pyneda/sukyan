package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/projectdiscovery/interactsh/pkg/server"

	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/spf13/cobra"
)

func TestInteractionCallback(interaction *server.Interaction) {
	fmt.Printf("Got Interaction: %v => %v => %v => %v\n", interaction.Protocol, interaction.FullId, interaction.UniqueID, interaction.RemoteAddress)
}

// interactCmd represents the interact command
var interactCmd = &cobra.Command{
	Use:   "interact",
	Short: "Starts an interactsh client and listens for interactions",
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
	utilsCmd.AddCommand(interactCmd)
}
