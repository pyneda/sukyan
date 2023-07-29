package db

import (
	"database/sql"
	"github.com/spf13/viper"
	stdlog "log"
	"os"
	"time"

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
	migrateError := db.AutoMigrate(&Workspace{}, &Issue{}, &History{}, &OOBTest{}, &OOBInteraction{}, &Task{}, &TaskJob{}, &WebSocketConnection{}, &WebSocketMessage{}, &JsonWebToken{})
	if migrateError != nil {
		log.Error().Err(migrateError).Msg("Failed to migrate database")
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
