package services

import (
	"testing"

	"gorm.io/gorm"

	"kuberan/internal/models"
)

// setBaseUSD makes the user's base currency USD, matching the USD fixture
// accounts so aggregates over them need no exchange rate.
func setBaseUSD(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	if err := db.Model(&models.User{}).Where("id = ?", userID).Update("base_currency", "USD").Error; err != nil {
		t.Fatalf("set base currency: %v", err)
	}
}
