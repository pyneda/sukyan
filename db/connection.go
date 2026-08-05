package db

import (
	"database/sql"
	stdlog "log"
	"os"
	"sync"
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

var (
	connection *DatabaseConnection
	once       sync.Once
	mutex      sync.Mutex
)

// Connection returns the singleton database connection
func Connection() *DatabaseConnection {
	once.Do(func() {
		connection = initDb()
	})
	return connection
}

// Close closes the database connection
func (dc *DatabaseConnection) Close() {
	if dc.sqlDb != nil {
		dc.sqlDb.Close()
	}
}

// DB returns the GORM database connection
func (dc *DatabaseConnection) DB() *gorm.DB {
	return dc.db
}

// RawDB returns the underlying sql.DB connection
func (dc *DatabaseConnection) RawDB() *sql.DB {
	return dc.sqlDb
}

func initDb() *DatabaseConnection {
	mutex.Lock()
	defer mutex.Unlock()

	// Return existing connection if already initialized
	if connection != nil {
		return connection
	}

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Error().Msg("POSTGRES_DSN environment variable not set")
		os.Exit(1)
	}

	dialector := postgres.Open(dsn)

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

	// Create PostgreSQL extensions that migrations may depend on
	db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)

	sqlDB, err := db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get underlying database connection")
		os.Exit(1)
	}

	maxOpenConns := viper.GetInt("db.max_open_conns")
	if maxOpenConns == 0 {
		maxOpenConns = 50
	}

	// Keeping idle well below open makes database/sql close and reopen a
	// connection for every request past the idle mark, and each reopen pays a
	// full SCRAM-SHA-256 handshake. Under scan load that measured as ~10% of
	// process CPU with every backend under 10s old, so idle tracks open and
	// conn_max_idle_time is what releases connections when the process is quiet.
	maxIdleConns := viper.GetInt("db.max_idle_conns")
	if maxIdleConns == 0 {
		maxIdleConns = 40
	}
	if maxIdleConns > maxOpenConns {
		maxIdleConns = maxOpenConns
	}

	connMaxLifetime := viper.GetDuration("db.conn_max_lifetime")
	if connMaxLifetime == 0 {
		connMaxLifetime = 1 * time.Hour
	}

	connMaxIdleTime := viper.GetDuration("db.conn_max_idle_time")
	if connMaxIdleTime == 0 {
		connMaxIdleTime = 5 * time.Minute
	}

	log.Debug().
		Int("max_idle_conns", maxIdleConns).
		Int("max_open_conns", maxOpenConns).
		Dur("conn_max_lifetime", connMaxLifetime).
		Dur("conn_max_idle_time", connMaxIdleTime).
		Msg("Configuring database connection pool")

	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	return &DatabaseConnection{
		db:    db,
		sqlDb: sqlDB,
	}
}

// Cleanup closes the database connection
func Cleanup() {
	if connection != nil {
		if connection.sqlDb != nil {
			log.Debug().Msg("Closing database connection")
			connection.sqlDb.Close()
		}
		connection = nil
	}
}
