package testutil

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"kuberan/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// counter provides unique values across fixtures within a test run.
var counter atomic.Int64

func nextID() int64 {
	return counter.Add(1)
}

// CreateTestUser creates a user with a hashed password and unique email.
func CreateTestUser(t *testing.T, db *gorm.DB) *models.User {
	t.Helper()
	email := fmt.Sprintf("user%d@test.com", nextID())
	return CreateTestUserWithEmail(t, db, email)
}

// CreateTestUserWithEmail creates a user with the given email.
func CreateTestUserWithEmail(t *testing.T, db *gorm.DB, email string) *models.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := &models.User{
		Email:    email,
		Password: string(hash),
		IsActive: true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

// CreateTestCashAccount creates a cash account with zero balance.
func CreateTestCashAccount(t *testing.T, db *gorm.DB, userID string) *models.Account {
	t.Helper()
	return CreateTestCashAccountWithBalance(t, db, userID, 0)
}

// CreateTestCashAccountWithBalance creates a cash account with the given balance (in cents).
func CreateTestCashAccountWithBalance(t *testing.T, db *gorm.DB, userID string, balance int64) *models.Account {
	t.Helper()

	account := &models.Account{
		UserID:   userID,
		Name:     fmt.Sprintf("Test Account %d", nextID()),
		Type:     models.AccountTypeCash,
		Balance:  balance,
		Currency: "USD",
		IsActive: true,
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("failed to create test cash account: %v", err)
	}
	return account
}

// CreateTestInvestmentAccount creates an investment account.
func CreateTestInvestmentAccount(t *testing.T, db *gorm.DB, userID string) *models.Account {
	t.Helper()

	account := &models.Account{
		UserID:   userID,
		Name:     fmt.Sprintf("Test Investment Account %d", nextID()),
		Type:     models.AccountTypeInvestment,
		Currency: "USD",
		IsActive: true,
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("failed to create test investment account: %v", err)
	}
	return account
}

// CreateTestCreditCardAccount creates a credit card account with the given balance.
func CreateTestCreditCardAccount(t *testing.T, db *gorm.DB, userID string, balance int64) *models.Account {
	t.Helper()

	account := &models.Account{
		UserID:      userID,
		Name:        fmt.Sprintf("Test Credit Card %d", nextID()),
		Type:        models.AccountTypeCreditCard,
		Balance:     balance,
		Currency:    "USD",
		IsActive:    true,
		CreditLimit: 500000, // $5000.00
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("failed to create test credit card account: %v", err)
	}
	return account
}

// CreateTestDebtAccount creates a debt account with the given balance.
func CreateTestDebtAccount(t *testing.T, db *gorm.DB, userID string, balance int64) *models.Account {
	t.Helper()

	account := &models.Account{
		UserID:       userID,
		Name:         fmt.Sprintf("Test Debt Account %d", nextID()),
		Type:         models.AccountTypeDebt,
		Balance:      balance,
		Currency:     "USD",
		IsActive:     true,
		InterestRate: 5.0,
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("failed to create test debt account: %v", err)
	}
	return account
}

// CreateTestCategory creates a category of the given type.
func CreateTestCategory(t *testing.T, db *gorm.DB, userID string, categoryType models.CategoryType) *models.Category {
	t.Helper()

	category := &models.Category{
		UserID: userID,
		Name:   fmt.Sprintf("Test Category %d", nextID()),
		Type:   categoryType,
	}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	return category
}

// CreateTestTransaction creates a transaction of the given type and amount (in cents).
func CreateTestTransaction(t *testing.T, db *gorm.DB, userID, accountID string, txType models.TransactionType, amount int64) *models.Transaction {
	t.Helper()

	tx := &models.Transaction{
		UserID:    userID,
		AccountID: accountID,
		Type:      txType,
		Amount:    amount,
		Date:      time.Now(),
	}
	if err := db.Create(tx).Error; err != nil {
		t.Fatalf("failed to create test transaction: %v", err)
	}
	return tx
}

// CreateTestBudget creates a monthly budget for the given category.
func CreateTestBudget(t *testing.T, db *gorm.DB, userID, categoryID string) *models.Budget {
	t.Helper()

	budget := &models.Budget{
		UserID:     userID,
		CategoryID: categoryID,
		Name:       fmt.Sprintf("Test Budget %d", nextID()),
		Amount:     10000, // $100.00
		Period:     models.BudgetPeriodMonthly,
		IsActive:   true,
	}
	if err := db.Create(budget).Error; err != nil {
		t.Fatalf("failed to create test budget: %v", err)
	}
	return budget
}

// CreateTestSecurity creates a security with default values.
func CreateTestSecurity(t *testing.T, db *gorm.DB) *models.Security {
	t.Helper()
	n := nextID()
	return CreateTestSecurityWithParams(t, db, fmt.Sprintf("SEC%d", n), fmt.Sprintf("Test Security %d", n), models.AssetTypeStock, "NYSE")
}

// CreateTestSecurityWithParams creates a security with the specified parameters.
func CreateTestSecurityWithParams(t *testing.T, db *gorm.DB, symbol, name string, assetType models.AssetType, exchange string) *models.Security {
	t.Helper()
	sec := &models.Security{
		Symbol:    symbol,
		Name:      name,
		AssetType: assetType,
		Currency:  "USD",
		Exchange:  exchange,
	}
	if err := db.Create(sec).Error; err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

// CreateTestInvestment creates an investment holding in the given account.
func CreateTestInvestment(t *testing.T, db *gorm.DB, accountID, securityID string) *models.Investment {
	t.Helper()

	inv := &models.Investment{
		AccountID:  accountID,
		SecurityID: securityID,
		Quantity:   10.0,
		CostBasis:  100000, // $1000.00
	}
	if err := db.Create(inv).Error; err != nil {
		t.Fatalf("failed to create test investment: %v", err)
	}
	return inv
}

// CreateTestSecurityPrice creates a security price record for testing.
func CreateTestSecurityPrice(t *testing.T, db *gorm.DB, securityID string, price int64, recordedAt time.Time) *models.SecurityPrice {
	t.Helper()

	sp := &models.SecurityPrice{
		SecurityID: securityID,
		Price:      price,
		RecordedAt: recordedAt,
	}
	if err := db.Create(sp).Error; err != nil {
		t.Fatalf("failed to create test security price: %v", err)
	}
	return sp
}

// CreateTestTelegramLink creates a linked TelegramLink for testing.
func CreateTestTelegramLink(t *testing.T, db *gorm.DB, userID string, telegramUserID int64) *models.TelegramLink {
	t.Helper()
	link := &models.TelegramLink{
		UserID:            userID,
		TelegramUserID:    telegramUserID,
		TelegramUsername:  "testuser",
		TelegramFirstName: "Test",
		DefaultCurrency:   "MYR",
		IsActive:          true,
	}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("failed to create test telegram link: %v", err)
	}
	return link
}
