package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration commands",
	Long:  `Manage database migrations using Atlas. Requires Atlas CLI to be installed.`,
}

var migrateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply pending database migrations",
	Long: `Apply all pending database migrations to the database.

This command uses Atlas to apply versioned migrations from the db/migrations directory.
The database connection is read from the POSTGRES_DSN environment variable.`,
	Run: func(cmd *cobra.Command, args []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		baseline, _ := cmd.Flags().GetString("baseline")
		allowDirty, _ := cmd.Flags().GetBool("allow-dirty")

		err := db.ApplyMigrations(db.MigrateApplyOptions{
			DryRun:     dryRun,
			Baseline:   baseline,
			AllowDirty: allowDirty,
		})
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			log.Error().Err(err).Msg("Atlas migrate apply failed")
			os.Exit(1)
		}

		if !dryRun {
			fmt.Println("Migrations applied successfully")
		}
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  `Show the current migration status of the database.`,
	Run: func(cmd *cobra.Command, args []string) {
		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			fmt.Println("Error: POSTGRES_DSN environment variable not set")
			os.Exit(1)
		}

		atlasArgs := []string{
			"migrate", "status",
			"--url", dsn,
			"--dir", "file://db/migrations",
		}

		atlasCmd := exec.Command("atlas", atlasArgs...)
		atlasCmd.Stdout = os.Stdout
		atlasCmd.Stderr = os.Stderr

		if err := atlasCmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Printf("Error running atlas: %v\n", err)
			os.Exit(1)
		}
	},
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff [name]",
	Short: "Generate a new migration from model changes",
	Long: `Generate a new migration file by comparing GORM models to the current migration state.

This command requires Docker to be running as Atlas uses a dev database for diffing.

Example:
  sukyan migrate diff add_user_roles
  sukyan migrate diff initial`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		atlasArgs := []string{
			"migrate", "diff", name,
			"--env", "gorm",
		}

		log.Info().Strs("args", atlasArgs).Msg("Running Atlas migrate diff")

		atlasCmd := exec.Command("atlas", atlasArgs...)
		atlasCmd.Stdout = os.Stdout
		atlasCmd.Stderr = os.Stderr

		if err := atlasCmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Printf("Error running atlas: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Migration '%s' generated successfully in db/migrations/\n", name)
	},
}

func init() {
	migrateCmd.AddCommand(migrateApplyCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateDiffCmd)

	migrateApplyCmd.Flags().Bool("dry-run", false, "Print SQL statements without executing them")
	migrateApplyCmd.Flags().String("baseline", "", "Mark migrations up to and including this version as applied")
	migrateApplyCmd.Flags().Bool("allow-dirty", false, "Allow running on non-clean database")

	rootCmd.AddCommand(migrateCmd)
}
