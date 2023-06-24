package db

import (
	"database/sql"
	"github.com/spf13/viper"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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

	// Default to sqlite if no DATABASE_TYPE is set
	dbType := viper.GetString("DATABASE_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	var dialector gorm.Dialector
	if dbType == "sqlite" {
		dialector = sqlite.Open("sukyan.db")
	} else if dbType == "postgres" {
		// Get the connection string from the environment variable
		dsn := viper.GetString("POSTGRES_DSN")
		if dsn == "" {
			log.Fatalf("No Postgres DSN provided")
		}
		dialector = postgres.Open(dsn)
	} else {
		log.Fatalf("Unknown database type: %s", dbType)
	}

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
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
		panic("failed to connect database")
	}
	db.AutoMigrate(&Workspace{}, &Issue{}, &History{}, &OOBTest{}, &OOBInteraction{}, &Task{}, &TaskJob{}, &WebSocketConnection{}, &WebSocketMessage{}, &JsonWebToken{})
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get underlying sql.DB")
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(80)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &DatabaseConnection{
		db:    db,
		sqlDb: sqlDB,
	}
}
