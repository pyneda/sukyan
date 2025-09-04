package cleanup

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var (
	vacuumFull    bool
	vacuumAnalyze bool
	vacuumTables  []string
	vacuumVerbose bool
	vacuumDryRun  bool
	vacuumToast   bool
	vacuumWorkMem string
	vacuumYes     bool
)

var vacuumDbCmd = &cobra.Command{
	Use:   "vacuum-db",
	Short: "🗜️  Reclaim PostgreSQL dead space with VACUUM operations",
	Long: `Reclaim dead space in PostgreSQL database using VACUUM operations.

This command analyzes and cleans up dead tuples in PostgreSQL tables, particularly
focusing on the histories table and its TOAST storage. It can perform regular VACUUM
or VACUUM FULL operations to reclaim disk space.

VACUUM vs VACUUM FULL:
- VACUUM: Cleans dead tuples, keeps space for reuse (non-blocking)
- VACUUM FULL: Rebuilds tables and returns space to OS (locks tables)`,
	RunE: runVacuumDb,
}

func init() {
	vacuumDbCmd.Flags().BoolVar(&vacuumFull, "full", false, "Perform VACUUM FULL to return space to OS (locks tables)")
	vacuumDbCmd.Flags().BoolVar(&vacuumAnalyze, "analyze", true, "Run ANALYZE after VACUUM to update statistics")
	vacuumDbCmd.Flags().StringSliceVar(&vacuumTables, "tables", []string{"histories"}, "Tables to vacuum (comma-separated)")
	vacuumDbCmd.Flags().BoolVar(&vacuumVerbose, "verbose", false, "Enable verbose output from VACUUM")
	vacuumDbCmd.Flags().BoolVar(&vacuumDryRun, "dry-run", false, "Show what would be vacuumed without executing")
	vacuumDbCmd.Flags().BoolVar(&vacuumToast, "toast", true, "Include TOAST tables (recommended for large blob storage)")
	vacuumDbCmd.Flags().StringVar(&vacuumWorkMem, "work-mem", "", "Set work_mem for VACUUM operation (e.g., 256MB)")
	vacuumDbCmd.Flags().BoolVar(&vacuumYes, "yes", false, "Skip confirmation prompt and proceed automatically")
}

func runVacuumDb(cmd *cobra.Command, args []string) error {
	connection := db.Connection()
	if connection == nil {
		return fmt.Errorf("❌ Failed to connect to database")
	}

	if len(vacuumTables) == 0 {
		return fmt.Errorf("❌ At least one table must be specified")
	}

	if err := validateTablesExist(connection, vacuumTables); err != nil {
		return fmt.Errorf("❌ Table validation failed: %w", err)
	}

	fmt.Println("🗜️  PostgreSQL VACUUM Configuration")
	fmt.Println("===================================")
	fmt.Printf("📋 Tables:           %s\n", strings.Join(vacuumTables, ", "))
	fmt.Printf("💾 Operation:        %s\n", func() string {
		if vacuumFull {
			return "VACUUM FULL (returns space to OS, locks tables)"
		}
		return "VACUUM (marks space for reuse, non-blocking)"
	}())
	fmt.Printf("📊 Analyze:          %v\n", vacuumAnalyze)
	fmt.Printf("🍞 Include TOAST:    %v\n", vacuumToast)
	fmt.Printf("🗣️  Verbose:          %v\n", vacuumVerbose)
	if vacuumWorkMem != "" {
		fmt.Printf("🧠 Work Memory:      %s\n", vacuumWorkMem)
	}
	if vacuumDryRun {
		fmt.Printf("🔍 Mode:             Dry run (no changes will be made)\n")
	} else {
		fmt.Printf("⚡ Mode:             Live operation\n")
	}
	fmt.Println()

	if vacuumDryRun {
		fmt.Println("🔍 Dry run - showing what would be executed:")
		for _, table := range vacuumTables {
			command := buildVacuumCommand(table)
			fmt.Printf("  📋 %s: %s\n", table, command)

			if vacuumToast {
				toastTable := getToastTableName(connection, table)
				if toastTable != "" {
					toastCommand := buildVacuumCommand(toastTable)
					fmt.Printf("  🍞 TOAST: %s\n", toastCommand)
				} else {
					fmt.Printf("  🍞 TOAST: No TOAST table for %s\n", table)
				}
			}
		}
		fmt.Println("\n💡 Run without --dry-run to execute these commands")
		return nil
	}

	if vacuumFull {
		if !vacuumYes {
			fmt.Println("\n⚠️  VACUUM FULL will lock tables and may take significant time!")
			fmt.Println("💡 Consider running during maintenance windows.")
			fmt.Print("Continue? (y/N): ")

			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))

			if response != "y" && response != "yes" {
				fmt.Println("❌ Operation cancelled by user")
				return nil
			}
		} else {
			fmt.Println("\n✅ VACUUM FULL auto-confirmed with --yes flag")
			fmt.Println("⚠️  Tables will be locked during operation")
		}
	}

	fmt.Println("⚡ Starting VACUUM operations...")

	if vacuumWorkMem != "" {
		if err := setWorkMem(connection, vacuumWorkMem); err != nil {
			fmt.Printf("⚠️  Warning: Could not set work_mem: %v\n", err)
		} else {
			fmt.Printf("🧠 Set work_mem to %s\n", vacuumWorkMem)
		}
	}

	// Execute VACUUM for each table
	for _, table := range vacuumTables {
		fmt.Printf("\n🔄 Processing table: %s\n", table)

		command := buildVacuumCommand(table)
		if err := executeVacuum(connection, command); err != nil {
			fmt.Printf("❌ Failed to vacuum %s: %v\n", table, err)
			continue
		}
		fmt.Printf("✅ Successfully vacuumed %s\n", table)

		// Vacuum TOAST table if requested
		if vacuumToast {
			toastTable := getToastTableName(connection, table)
			if toastTable != "" {
				fmt.Printf("🔄 Processing TOAST table: %s\n", toastTable)

				toastCommand := buildVacuumCommand(toastTable)
				if err := executeVacuum(connection, toastCommand); err != nil {
					fmt.Printf("⚠️  Warning: Could not vacuum TOAST table: %v\n", err)
				} else {
					fmt.Printf("✅ Successfully vacuumed TOAST table\n")
				}
			} else {
				fmt.Printf("ℹ️  Table %s has no TOAST table\n", table)
			}
		}
	}

	fmt.Println("\n🎉 VACUUM operations completed!")
	fmt.Println("💡 Consider running ANALYZE if statistics are outdated")

	return nil
}

func buildVacuumCommand(table string) string {
	var parts []string

	parts = append(parts, "VACUUM")

	if vacuumFull {
		parts = append(parts, "FULL")
	}

	if vacuumVerbose {
		parts = append(parts, "VERBOSE")
	}

	if vacuumAnalyze {
		parts = append(parts, "ANALYZE")
	}

	quotedTable := quoteIdentifier(table)
	parts = append(parts, quotedTable)

	return strings.Join(parts, " ")
}

func quoteIdentifier(identifier string) string {
	// Handle schema.table format
	if strings.Contains(identifier, ".") {
		parts := strings.SplitN(identifier, ".", 2)
		if len(parts) == 2 {
			schema := strings.ReplaceAll(parts[0], `"`, `""`)
			table := strings.ReplaceAll(parts[1], `"`, `""`)
			return fmt.Sprintf(`"%s"."%s"`, schema, table)
		}
	}

	// Regular table name
	escaped := strings.ReplaceAll(identifier, `"`, `""`)
	return fmt.Sprintf(`"%s"`, escaped)
}

func executeVacuum(connection *db.DatabaseConnection, command string) error {
	fmt.Printf("🔧 Executing: %s\n", command)

	sqlDB := connection.RawDB()
	if sqlDB == nil {
		return fmt.Errorf("failed to get raw database connection")
	}

	_, err := sqlDB.Exec(command)
	return err
}

func setWorkMem(connection *db.DatabaseConnection, workMem string) error {
	return connection.DB().Exec("SET work_mem = ?", workMem).Error
}

func getToastTableName(connection *db.DatabaseConnection, tableName string) string {
	var toastTableName string
	query := `
		SELECT t.relname 
		FROM pg_class c 
		JOIN pg_class t ON c.reltoastrelid = t.oid 
		WHERE c.relname = ? AND c.relkind = 'r'
	`
	err := connection.DB().Raw(query, tableName).Scan(&toastTableName).Error
	if err != nil || toastTableName == "" {
		return ""
	}
	return fmt.Sprintf("pg_toast.%s", toastTableName)
}

func validateTablesExist(connection *db.DatabaseConnection, tableNames []string) error {
	for _, tableName := range tableNames {
		var exists bool
		err := connection.DB().Raw("SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = ? AND table_schema = 'public')", tableName).Scan(&exists).Error
		if err != nil {
			return fmt.Errorf("failed to check if table %s exists: %w", tableName, err)
		}
		if !exists {
			return fmt.Errorf("table %s does not exist", tableName)
		}
	}
	return nil
}
