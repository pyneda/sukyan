package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		file := viper.ConfigFileUsed()
		fmt.Printf("Using config file: %s\n", file)
		fmt.Println("Current configuration:")
		settings := viper.AllSettings()
		output, _ := yaml.Marshal(settings)
		fmt.Println(string(output))
	},
}

var configDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump configuration to file",
	Run: func(cmd *cobra.Command, args []string) {
		outputPath, _ := cmd.Flags().GetString("output")
		force, _ := cmd.Flags().GetBool("force")

		if outputPath == "" {
			fmt.Println("Output path is required")
			os.Exit(1)
		}

		if _, err := os.Stat(outputPath); err == nil && !force {
			fmt.Printf("File %s already exists. Use --force to overwrite.\n", outputPath)
			os.Exit(1)
		}

		dir := filepath.Dir(outputPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %s\n", dir, err)
			os.Exit(1)
		}

		viper.SetConfigFile(outputPath)
		if err := viper.WriteConfigAs(outputPath); err != nil {
			fmt.Printf("Error writing config: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Configuration saved to %s\n", outputPath)
	},
}

func init() {
	configCmd.AddCommand(configDumpCmd)

	configDumpCmd.Flags().StringP("output", "o", "config.yml", "Output file path")
	configDumpCmd.Flags().BoolP("force", "f", false, "Force overwrite existing file")

	rootCmd.AddCommand(configCmd)
}
