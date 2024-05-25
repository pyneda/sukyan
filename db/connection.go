package db

import (
	"database/sql"
	stdlog "log"
	"os"
	"time"

	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DatabaseConnection struct {
	db    *gorm.DB
	sqlDb *sql.DB
}

var Connection = InitDb()

func InitDb() *DatabaseConnection {
	// Set up viper to read from the environment
	viper.AutomaticEnv()

	var dialector gorm.Dialector

	dsn := viper.GetString("POSTGRES_DSN")
	if dsn == "" {
		log.Error().Msg("POSTGRES_DSN environment variable not set")
		os.Exit(1)
	}
	dialector = postgres.Open(dsn)

	newLogger := logger.New(
		stdlog.New(os.Stdout, "\r\n", stdlog.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Silent,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to database")
		os.Exit(1)
	}
	sql := `DO $$ BEGIN
		CREATE TYPE severity AS ENUM ('Unknown', 'Info', 'Low', 'Medium', 'High', 'Critical');
	EXCEPTION
		WHEN duplicate_object THEN null;
	END $$;`
	db.Exec(sql)
	db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)

	// Migrate Issue separately after enum creation
	if err := db.AutoMigrate(&Issue{}); err != nil {
		log.Error().Err(err).Msg("Failed to migrate Issue table")
		os.Exit(1)
	}

	// Migrate other tables
	if err := db.AutoMigrate(&Workspace{}, &History{}, &OOBTest{}, &OOBInteraction{}, &Task{}, &TaskJob{}, &WebSocketConnection{}, &WebSocketMessage{}, &JsonWebToken{}, &User{}, &RefreshToken{}); err != nil {
		log.Error().Err(err).Msg("Failed to migrate other tables")
		os.Exit(1)
	}

	if err := db.AutoMigrate(&PlaygroundCollection{}, &PlaygroundSession{}); err != nil {
		log.Error().Err(err).Msg("Failed to migrate PlaygroundCollection or PlaygroundSession table")
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get underlying database connection")
		os.Exit(1)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(80)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &DatabaseConnection{
		db:    db,
		sqlDb: sqlDB,
	}
}
