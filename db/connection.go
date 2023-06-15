package db

import (
	"database/sql"
	"log"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DatabaseConnection struct {
	db    *gorm.DB
	sqlDb *sql.DB
}

var Connection = InitDb()

func InitDb() DatabaseConnection {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second,   // Slow SQL threshold
			LogLevel:                  logger.Silent, // Log level
			IgnoreRecordNotFoundError: true,          // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      true,          // Don't include params in the SQL log
			Colorful:                  false,         // Disable color
		},
	)
	db, err := gorm.Open(sqlite.Open("sukyan.db"), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic("failed to connect database")
	}
	db.AutoMigrate(&Issue{}, &History{}, &OOBTest{}, &OOBInteraction{})
	// Get generic database object sql.DB to use its functions
	sqlDB, err := db.DB()

	// SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
	sqlDB.SetMaxIdleConns(10)

	// SetMaxOpenConns sets the maximum number of open connections to the database.
	sqlDB.SetMaxOpenConns(100)

	// SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
	sqlDB.SetConnMaxLifetime(time.Hour)

	return DatabaseConnection{
		db:    db,
		sqlDb: sqlDB,
	}
}

// func GetDbConnection() {
// 	db, err := gorm.Open(sqlite.Open("sukyan.db"), &gorm.Config{})
// 	if err != nil {
// 		panic("failed to connect database")
// 	}
// 	db.AutoMigrate(&Issue{})
// }
