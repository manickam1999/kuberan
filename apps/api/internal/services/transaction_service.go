package services

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// transactionService handles transaction-related business logic.
type transactionService struct {
	db             *gorm.DB
	accountService AccountServicer
}

// NewTransactionService creates a new TransactionServicer.
func NewTransactionService(db *gorm.DB, accountService AccountServicer) TransactionServicer {
	return &transactionService{
		db:             db,
		accountService: accountService,
	}
}

// CreateTransaction creates a new transaction for a user's account
func (s *transactionService) CreateTransaction(
	userID string,
	accountID string,
	categoryID *string,
	transactionType models.TransactionType,
	amount int64,
	description string,
	date time.Time,
) (*models.Transaction, error) {
	// Validate input
	if amount <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "amount must be greater than zero")
	}

	if accountID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account ID is required")
	}

	// Default date to now if not provided
	if date.IsZero() {
		date = time.Now()
	}

	// Get the account to ensure it exists and belongs to the user
	account, err := s.accountService.GetAccountByID(userID, accountID)
	if err != nil {
		return nil, err
	}

	var result *models.Transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var txErr error
		result, txErr = s.createTransactionWithDB(tx, userID, account, categoryID, transactionType, amount, description, date)
		return txErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// createTransactionWithDB creates a transaction with a given database connection (useful for transactions)
func (s *transactionService) createTransactionWithDB(
	tx *gorm.DB,
	userID string,
	account *models.Account,
	categoryID *string,
	transactionType models.TransactionType,
	amount int64,
	description string,
	date time.Time,
) (*models.Transaction, error) {
	// Create transaction record
	transaction := &models.Transaction{
		UserID:      userID,
		AccountID:   account.ID,
		CategoryID:  categoryID,
		Type:        transactionType,
		Amount:      amount,
		Description: description,
		Date:        date,
	}

	if err := tx.Create(transaction).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.accountService.UpdateAccountBalance(tx, account, transactionType, amount); err != nil {
		return nil, err
	}

	return transaction, nil
}

// CreateTransfer creates an account-to-account transfer within a single DB transaction.
func (s *transactionService) CreateTransfer(
	userID, fromAccountID, toAccountID string,
	amount int64,
	description string,
	date time.Time,
) (*models.Transaction, error) {
	if fromAccountID == toAccountID {
		return nil, apperrors.ErrSameAccountTransfer
	}

	if amount <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "amount must be greater than zero")
	}

	if date.IsZero() {
		date = time.Now()
	}

	fromAccount, err := s.accountService.GetAccountByID(userID, fromAccountID)
	if err != nil {
		return nil, err
	}

	toAccount, err := s.accountService.GetAccountByID(userID, toAccountID)
	if err != nil {
		return nil, err
	}

	if fromAccount.Type != models.AccountTypeCreditCard && fromAccount.Balance < amount {
		return nil, apperrors.ErrInsufficientBalance
	}

	var result *models.Transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		transaction := &models.Transaction{
			UserID:      userID,
			AccountID:   fromAccountID,
			ToAccountID: &toAccountID,
			Type:        models.TransactionTypeTransfer,
			Amount:      amount,
			Description: description,
			Date:        date,
		}
		if txErr := tx.Create(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		if txErr := s.accountService.UpdateAccountBalance(tx, fromAccount, models.TransactionTypeExpense, amount); txErr != nil {
			return txErr
		}
		if txErr := s.accountService.UpdateAccountBalance(tx, toAccount, models.TransactionTypeIncome, amount); txErr != nil {
			return txErr
		}

		result = transaction
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// reverseType flips income↔expense for balance reversal.
func reverseType(t models.TransactionType) models.TransactionType {
	if t == models.TransactionTypeIncome {
		return models.TransactionTypeExpense
	}
	return models.TransactionTypeIncome
}

// UpdateTransaction updates an existing income/expense transaction.
// Transfer and investment transactions cannot be edited.
func (s *transactionService) UpdateTransaction(userID, transactionID string, updates TransactionUpdateFields) (*models.Transaction, error) {
	transaction, err := s.GetTransactionByID(userID, transactionID)
	if err != nil {
		return nil, err
	}

	// Reject transfers and investment transactions
	if transaction.Type == models.TransactionTypeTransfer || transaction.Type == models.TransactionTypeInvestment {
		return nil, apperrors.ErrTransactionNotEditable
	}

	// If type change requested, reject changes to/from transfer or investment
	if updates.Type != nil {
		newType := *updates.Type
		if newType == models.TransactionTypeTransfer || newType == models.TransactionTypeInvestment {
			return nil, apperrors.ErrInvalidTypeChange
		}
	}

	// Capture old values
	oldAccountID := transaction.AccountID
	oldType := transaction.Type
	oldAmount := transaction.Amount

	// Determine new values
	newAccountID := oldAccountID
	if updates.AccountID != nil {
		newAccountID = *updates.AccountID
	}
	newType := oldType
	if updates.Type != nil {
		newType = *updates.Type
	}
	newAmount := oldAmount
	if updates.Amount != nil {
		newAmount = *updates.Amount
	}

	// Fetch old account
	oldAccount, err := s.accountService.GetAccountByID(userID, oldAccountID)
	if err != nil {
		return nil, err
	}

	// If account is changing, fetch the new account
	var targetAccount *models.Account
	if newAccountID != oldAccountID {
		targetAccount, err = s.accountService.GetAccountByID(userID, newAccountID)
		if err != nil {
			return nil, err
		}
	} else {
		targetAccount = oldAccount
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Reverse old impact on old account
		if txErr := s.accountService.UpdateAccountBalance(tx, oldAccount, reverseType(oldType), oldAmount); txErr != nil {
			return txErr
		}

		// Apply field updates
		if updates.AccountID != nil {
			transaction.AccountID = *updates.AccountID
		}
		if updates.Type != nil {
			transaction.Type = *updates.Type
		}
		if updates.Amount != nil {
			transaction.Amount = *updates.Amount
		}
		if updates.Description != nil {
			transaction.Description = *updates.Description
		}
		if updates.Date != nil {
			transaction.Date = *updates.Date
		}
		if updates.CategoryID != nil {
			transaction.CategoryID = *updates.CategoryID
		}

		if txErr := tx.Save(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Apply new impact on target account
		if txErr := s.accountService.UpdateAccountBalance(tx, targetAccount, newType, newAmount); txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return transaction, nil
}

// GetAccountTransactions retrieves a paginated, filtered list of transactions for a specific account.
func (s *transactionService) GetAccountTransactions(userID, accountID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
	// First verify the account belongs to the user
	_, err := s.accountService.GetAccountByID(userID, accountID)
	if err != nil {
		return nil, err
	}

	page.Defaults()

	base := s.db.Model(&models.Transaction{}).Where("user_id = ? AND account_id = ?", userID, accountID)
	base = applyTransactionFilters(base, filter)

	var totalItems int64
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var transactions []models.Transaction
	if err := base.Scopes(pagination.Paginate(page)).
		Order("date DESC, created_at DESC").
		Find(&transactions).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.populateAttachmentCounts(transactions); err != nil {
		return nil, err
	}

	result := pagination.NewPageResponse(transactions, page.Page, page.PageSize, totalItems)
	return &result, nil
}

func applyTransactionFilters(q *gorm.DB, f TransactionFilter) *gorm.DB {
	if f.FromDate != nil {
		q = q.Where("date >= ?", *f.FromDate)
	}
	if f.ToDate != nil {
		q = q.Where("date <= ?", *f.ToDate)
	}
	if f.Type != nil {
		q = q.Where("type = ?", *f.Type)
	}
	if f.CategoryID != nil {
		q = q.Where("category_id = ?", *f.CategoryID)
	}
	if f.MinAmount != nil {
		q = q.Where("amount >= ?", *f.MinAmount)
	}
	if f.MaxAmount != nil {
		q = q.Where("amount <= ?", *f.MaxAmount)
	}
	if f.AccountID != nil {
		q = q.Where("account_id = ?", *f.AccountID)
	}
	return q
}

// GetUserTransactions retrieves a paginated, filtered list of all transactions for a user across all accounts.
func (s *transactionService) GetUserTransactions(userID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
	page.Defaults()

	base := s.db.Model(&models.Transaction{}).Where("user_id = ?", userID)
	base = applyTransactionFilters(base, filter)

	var totalItems int64
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var transactions []models.Transaction
	if err := base.Preload("Category").
		Scopes(pagination.Paginate(page)).
		Order("date DESC, created_at DESC").
		Find(&transactions).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.populateAttachmentCounts(transactions); err != nil {
		return nil, err
	}

	result := pagination.NewPageResponse(transactions, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// populateAttachmentCounts fills in the derived AttachmentsCount for the given
// page of transactions using a single grouped query, so the list can render a
// receipt indicator without preloading attachment rows. Soft-deleted
// attachments are excluded by GORM's default scope.
func (s *transactionService) populateAttachmentCounts(transactions []models.Transaction) error {
	if len(transactions) == 0 {
		return nil
	}

	ids := make([]string, len(transactions))
	for i := range transactions {
		ids[i] = transactions[i].ID
	}

	type countRow struct {
		TransactionID string
		Count         int
	}
	var rows []countRow
	if err := s.db.Model(&models.TransactionAttachment{}).
		Select("transaction_id, COUNT(*) AS count").
		Where("transaction_id IN ?", ids).
		Group("transaction_id").
		Scan(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.TransactionID] = r.Count
	}
	for i := range transactions {
		transactions[i].AttachmentsCount = counts[transactions[i].ID]
	}
	return nil
}

// GetTransactionByID retrieves a transaction by ID for a specific user
func (s *transactionService) GetTransactionByID(userID, transactionID string) (*models.Transaction, error) {
	var transaction models.Transaction
	if err := s.db.Where("id = ? AND user_id = ?", transactionID, userID).First(&transaction).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTransactionNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &transaction, nil
}

// DeleteTransaction deletes a transaction and updates the account balance
func (s *transactionService) DeleteTransaction(userID, transactionID string) error {
	transaction, err := s.GetTransactionByID(userID, transactionID)
	if err != nil {
		return err
	}

	account, err := s.accountService.GetAccountByID(userID, transaction.AccountID)
	if err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if txErr := tx.Delete(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		switch transaction.Type {
		case models.TransactionTypeIncome:
			return s.accountService.UpdateAccountBalance(tx, account, models.TransactionTypeExpense, transaction.Amount)
		case models.TransactionTypeExpense:
			return s.accountService.UpdateAccountBalance(tx, account, models.TransactionTypeIncome, transaction.Amount)
		case models.TransactionTypeTransfer:
			if transaction.ToAccountID == nil {
				return apperrors.ErrInvalidTransactionType
			}
			toAccount, toErr := s.accountService.GetAccountByID(userID, *transaction.ToAccountID)
			if toErr != nil {
				return toErr
			}
			// Reverse: add back to from-account, subtract from to-account
			if txErr := s.accountService.UpdateAccountBalance(tx, account, models.TransactionTypeIncome, transaction.Amount); txErr != nil {
				return txErr
			}
			return s.accountService.UpdateAccountBalance(tx, toAccount, models.TransactionTypeExpense, transaction.Amount)
		default:
			return apperrors.ErrInvalidTransactionType
		}
	})
}

// GetMonthlySummary returns monthly income and expense totals for the last N months.
func (s *transactionService) GetMonthlySummary(userID string, months int) ([]MonthlySummaryItem, error) {
	now := time.Now()
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)

	items := make([]MonthlySummaryItem, 0, months)

	current := startMonth
	for i := 0; i < months; i++ {
		monthStart := current
		monthEnd := current.AddDate(0, 1, 0).Add(-time.Nanosecond)

		var income int64
		if err := s.db.Model(&models.Transaction{}).
			Select("COALESCE(SUM(amount), 0)").
			Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ? AND description != ?",
				userID, models.TransactionTypeIncome, monthStart, monthEnd, "Initial balance").
			Scan(&income).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		var expenses int64
		if err := s.db.Model(&models.Transaction{}).
			Select("COALESCE(SUM(amount), 0)").
			Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
				userID, models.TransactionTypeExpense, monthStart, monthEnd).
			Scan(&expenses).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		items = append(items, MonthlySummaryItem{
			Month:    monthStart.Format("2006-01"),
			Income:   income,
			Expenses: expenses,
		})

		current = current.AddDate(0, 1, 0)
	}

	return items, nil
}

// GetDailySpending returns daily expense totals for a date range.
func (s *transactionService) GetDailySpending(userID string, from, to time.Time) ([]DailySpendingItem, error) {
	// Normalize to start/end of day
	current := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)

	var items []DailySpendingItem

	for !current.After(end) {
		dayStart := current
		dayEnd := current.Add(24*time.Hour - time.Nanosecond)

		var total int64
		if err := s.db.Model(&models.Transaction{}).
			Select("COALESCE(SUM(amount), 0)").
			Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
				userID, models.TransactionTypeExpense, dayStart, dayEnd).
			Scan(&total).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		items = append(items, DailySpendingItem{
			Date:  dayStart.Format("2006-01-02"),
			Total: total,
		})

		current = current.AddDate(0, 0, 1)
	}

	if items == nil {
		items = []DailySpendingItem{}
	}

	return items, nil
}

// categoryColorPalette provides fallback colors for categories that don't have a color set.
// These are visually distinct and work well on both light and dark backgrounds.
var categoryColorPalette = []string{
	"#22C55E", // green
	"#3B82F6", // blue
	"#F59E0B", // amber
	"#EF4444", // red
	"#8B5CF6", // violet
	"#EC4899", // pink
	"#06B6D4", // cyan
	"#F97316", // orange
	"#14B8A6", // teal
	"#A855F7", // purple
}

// getCategoryColorFromID returns a deterministic color for a UUID using FNV-1a hash.
// This provides better distribution than string-based rolling hashes, reducing color collisions.
func getCategoryColorFromID(id string) string {
	// Parse UUID to get raw bytes for better distribution
	parsed, err := uuid.Parse(id)
	if err != nil {
		// Fallback to default gray if UUID parsing fails
		return "#9CA3AF"
	}

	// FNV-1a hash for better distribution
	const fnvOffset uint64 = 14695981039346656037
	const fnvPrime uint64 = 1099511628211

	hash := fnvOffset
	for _, b := range parsed[:] {
		hash ^= uint64(b)
		hash *= fnvPrime
	}

	return categoryColorPalette[hash%uint64(len(categoryColorPalette))]
}

// GetSpendingByCategory returns expense totals grouped by category for a date range.
func (s *transactionService) GetSpendingByCategory(userID string, from, to time.Time) (*SpendingByCategory, error) {
	type categorySpend struct {
		CategoryID *string
		Total      int64
	}

	var results []categorySpend
	err := s.db.Model(&models.Transaction{}).
		Select("category_id, COALESCE(SUM(amount), 0) as total").
		Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
			userID, models.TransactionTypeExpense, from, to).
		Group("category_id").
		Scan(&results).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var items []SpendingByCategoryItem
	var totalSpent int64

	for _, r := range results {
		item := SpendingByCategoryItem{
			CategoryID: r.CategoryID,
			Total:      r.Total,
		}

		if r.CategoryID != nil {
			var category models.Category
			if catErr := s.db.Where("id = ?", *r.CategoryID).First(&category).Error; catErr != nil {
				item.CategoryName = "Unknown Category"
				item.CategoryColor = "#9CA3AF"
			} else {
				item.CategoryName = category.Name
				item.CategoryColor = category.Color
				item.CategoryIcon = category.Icon
				// Use hash of UUID bytes for deterministic color if category has no color set
				if item.CategoryColor == "" {
					item.CategoryColor = getCategoryColorFromID(*r.CategoryID)
				}
			}
		} else {
			item.CategoryName = "Uncategorized"
			item.CategoryColor = "#9CA3AF"
		}

		totalSpent += r.Total
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Total > items[j].Total
	})

	if items == nil {
		items = []SpendingByCategoryItem{}
	}

	return &SpendingByCategory{
		Items:      items,
		TotalSpent: totalSpent,
		FromDate:   from,
		ToDate:     to,
	}, nil
}
