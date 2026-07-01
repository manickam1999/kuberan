// Package testutil provides test helpers for setting up in-memory databases,
// creating fixtures, and making assertions.
package testutil

import (
	"testing"

	"kuberan/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// allModels is the list of all GORM models to auto-migrate in tests.
var allModels = []interface{}{
	&models.User{},
	&models.Account{},
	&models.Category{},
	&models.Transaction{},
	&models.Budget{},
	&models.Security{},
	&models.Investment{},
	&models.InvestmentTransaction{},
	&models.SecurityPrice{},
	&models.PortfolioSnapshot{},
	&models.AuditLog{},
	&models.TelegramLink{},
	&models.TrustedOAuthClient{},
}

// SetupTestDB creates an in-memory SQLite database with all models migrated.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(allModels...); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

// TeardownTestDB closes the underlying database connection.
func TeardownTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Errorf("failed to get underlying DB for teardown: %v", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		t.Errorf("failed to close test database: %v", err)
	}
}
