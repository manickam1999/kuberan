package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// accountService handles account-related business logic.
type accountService struct {
	db *gorm.DB
}

// NewAccountService creates a new AccountServicer.
func NewAccountService(db *gorm.DB) AccountServicer {
	return &accountService{db: db}
}

// CreateCashAccount creates a new cash account for a user
func (s *accountService) CreateCashAccount(userID, name, description, currency string, initialBalance int64) (*models.Account, error) {
	// Validate input
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD" // Default currency
	}

	// Create account
	account := &models.Account{
		UserID:      userID,
		Name:        name,
		Type:        models.AccountTypeCash,
		Description: description,
		Balance:     initialBalance,
		Currency:    currency,
		IsActive:    true,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		if initialBalance > 0 {
			transaction := &models.Transaction{
				UserID:      userID,
				AccountID:   account.ID,
				Type:        models.TransactionTypeIncome,
				Amount:      initialBalance,
				Description: "Initial balance",
				Date:        time.Now(),
			}
			if err := tx.Create(transaction).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return account, nil
}

// CreateInvestmentAccount creates a new investment account for a user.
func (s *accountService) CreateInvestmentAccount(userID, name, description, currency, broker, accountNumber string) (*models.Account, error) {
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD"
	}

	account := &models.Account{
		UserID:        userID,
		Name:          name,
		Type:          models.AccountTypeInvestment,
		Description:   description,
		Currency:      currency,
		Broker:        broker,
		AccountNumber: accountNumber,
		IsActive:      true,
	}

	if err := s.db.Create(account).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return account, nil
}

// CreateCreditCardAccount creates a new credit card account for a user.
func (s *accountService) CreateCreditCardAccount(userID, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error) {
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD"
	}

	account := &models.Account{
		UserID:       userID,
		Name:         name,
		Type:         models.AccountTypeCreditCard,
		Description:  description,
		Balance:      0,
		Currency:     currency,
		IsActive:     true,
		CreditLimit:  creditLimit,
		InterestRate: interestRate,
	}

	if dueDate != nil {
		account.DueDate = *dueDate
	}

	if err := s.db.Create(account).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return account, nil
}

// GetUserAccounts retrieves a paginated list of accounts for a user.
func (s *accountService) GetUserAccounts(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Account{}).Where("user_id = ? AND is_active = ?", userID, true)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var accounts []models.Account
	if err := base.Scopes(pagination.Paginate(page)).Find(&accounts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.enrichInvestmentBalances(accounts); err != nil {
		return nil, err
	}

	result := pagination.NewPageResponse(accounts, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetAccountByID retrieves an account by ID for a specific user
func (s *accountService) GetAccountByID(userID, accountID string) (*models.Account, error) {
	var account models.Account
	if err := s.db.Where("id = ? AND user_id = ? AND is_active = ?", accountID, userID, true).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAccountNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if account.Type == models.AccountTypeInvestment {
		accounts := []models.Account{account}
		if err := s.enrichInvestmentBalances(accounts); err != nil {
			return nil, err
		}
		account = accounts[0]
	}

	return &account, nil
}

// UpdateAccount updates an existing account for any account type.
// Only fields relevant to the account's type are applied.
func (s *accountService) UpdateAccount(userID, accountID string, fields AccountUpdateFields) (*models.Account, error) {
	account, err := s.GetAccountByID(userID, accountID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	// Common fields (all account types)
	if fields.Name != nil && *fields.Name != "" {
		updates["name"] = *fields.Name
	}
	if fields.Description != nil {
		updates["description"] = *fields.Description
	}
	if fields.IsActive != nil {
		updates["is_active"] = *fields.IsActive
	}

	// Investment-only fields
	if account.Type == models.AccountTypeInvestment {
		if fields.Broker != nil {
			updates["broker"] = *fields.Broker
		}
		if fields.AccountNumber != nil {
			updates["account_number"] = *fields.AccountNumber
		}
	}

	// Credit card-only fields
	if account.Type == models.AccountTypeCreditCard {
		if fields.InterestRate != nil {
			updates["interest_rate"] = *fields.InterestRate
		}
		if fields.DueDate != nil {
			updates["due_date"] = *fields.DueDate
		}
		if fields.CreditLimit != nil {
			updates["credit_limit"] = *fields.CreditLimit
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(account).Updates(updates).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		// Reload to get fresh data
		if err := s.db.Where("id = ?", account.ID).First(account).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	return account, nil
}

// UpdateAccountBalance updates the balance of an account based on transaction
func (s *accountService) UpdateAccountBalance(tx *gorm.DB, account *models.Account, transactionType models.TransactionType, amount int64) error {
	// Update the balance based on transaction type and account type
	// Credit cards: positive balance = amount owed (expense increases, income/payment decreases)
	// All others: income adds, expense subtracts
	switch transactionType {
	case models.TransactionTypeIncome:
		if account.Type == models.AccountTypeCreditCard {
			account.Balance -= amount
		} else {
			account.Balance += amount
		}
	case models.TransactionTypeExpense:
		if account.Type == models.AccountTypeCreditCard {
			account.Balance += amount
		} else {
			account.Balance -= amount
		}
	}

	// Save the updated balance
	if err := tx.Model(account).Update("balance", account.Balance).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// enrichInvestmentBalances computes market-value balances for investment accounts
// in the given slice. Non-investment accounts are left unchanged.
func (s *accountService) enrichInvestmentBalances(accounts []models.Account) error {
	// Collect investment account IDs
	var investmentAccountIDs []string
	for i := range accounts {
		if accounts[i].Type == models.AccountTypeInvestment {
			investmentAccountIDs = append(investmentAccountIDs, accounts[i].ID)
		}
	}
	if len(investmentAccountIDs) == 0 {
		return nil
	}

	// Fetch investments for those accounts
	type holding struct {
		AccountID  string
		SecurityID string
		Quantity   float64
	}
	var holdings []holding
	if err := s.db.Table("investments").
		Select("account_id, security_id, quantity").
		Where("account_id IN ? AND deleted_at IS NULL", investmentAccountIDs).
		Scan(&holdings).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if len(holdings) == 0 {
		return nil
	}

	// Collect distinct security IDs
	secIDSet := make(map[string]struct{})
	for _, h := range holdings {
		secIDSet[h.SecurityID] = struct{}{}
	}
	secIDs := make([]string, 0, len(secIDSet))
	for id := range secIDSet {
		secIDs = append(secIDs, id)
	}

	// Batch-fetch latest prices
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return err
	}

	// Accumulate market value per account
	balances := make(map[string]int64)
	for _, h := range holdings {
		balances[h.AccountID] += int64(h.Quantity * float64(prices[h.SecurityID]))
	}

	// Set balances on the account slice
	for i := range accounts {
		if accounts[i].Type == models.AccountTypeInvestment {
			accounts[i].Balance = balances[accounts[i].ID]
		}
	}

	return nil
}
