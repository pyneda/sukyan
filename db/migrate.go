package db

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

// MigrateApplyOptions controls behavior of ApplyMigrations.
type MigrateApplyOptions struct {
	DryRun     bool
	Baseline   string
	AllowDirty bool
	// Dir overrides the migrations directory. Defaults to "file://db/migrations".
	Dir string
}

// ApplyMigrations shells out to the Atlas CLI to apply pending migrations
// against the database identified by POSTGRES_DSN. Returns an error if the
// CLI fails or the env var is unset; callers decide whether to fail-fast.
func ApplyMigrations(opts MigrateApplyOptions) error {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return fmt.Errorf("POSTGRES_DSN environment variable not set")
	}

	dir := opts.Dir
	if dir == "" {
		dir = "file://db/migrations"
	}

	args := []string{"migrate", "apply", "--url", dsn, "--dir", dir}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Baseline != "" {
		args = append(args, "--baseline", opts.Baseline)
	}
	if opts.AllowDirty {
		args = append(args, "--allow-dirty")
	}

	log.Info().Strs("args", args).Msg("Running Atlas migrate apply")

	cmd := exec.Command("atlas", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas migrate apply: %w", err)
	}
	return nil
}
