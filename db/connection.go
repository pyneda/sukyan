package db

import (
	"database/sql"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DatabaseConnection struct {
	db    *gorm.DB
	sqlDb *sql.DB
}

var Connection = InitDb()

func InitDb() DatabaseConnection {
	db, err := gorm.Open(sqlite.Open("sukyan.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	db.AutoMigrate(&Issue{}, &History{})
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
