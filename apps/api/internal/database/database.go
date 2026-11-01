package database

import (
	"fmt"

	"kuberan/internal/logger"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Manager handles database operations
type Manager struct {
	db  *gorm.DB
	dsn string
}

// NewManager creates a new database manager
func NewManager(config *Config) (*Manager, error) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  config.DSN(),
		PreferSimpleProtocol: true, // Required for Supabase Supavisor; harmless for direct connections
	}), &gorm.Config{
		// Single statements are atomic on their own; multi-statement writes
		// already use explicit db.Transaction blocks.
		SkipDefaultTransaction: true,
		Logger:                 newGormLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(min(config.MaxIdleConns, config.MaxOpenConns))
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)

	pgURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.User, config.Password, config.Host, config.Port, config.DBName, config.SSLMode)

	return &Manager{db: db, dsn: pgURL}, nil
}

// RunMigrations applies pending SQL migrations from the migrations/ directory.
func (m *Manager) RunMigrations() error {
	logger.Get().Info("Running database migrations...")

	mig, err := migrate.New("file://migrations", m.dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		srcErr, dbErr := mig.Close()
		if srcErr != nil {
			logger.Get().Warnf("migrate source close error: %v", srcErr)
		}
		if dbErr != nil {
			logger.Get().Warnf("migrate database close error: %v", dbErr)
		}
	}()

	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	logger.Get().Info("Database migrations completed successfully")
	return nil
}

// DB returns the underlying GORM database instance
func (m *Manager) DB() *gorm.DB {
	return m.db
}
