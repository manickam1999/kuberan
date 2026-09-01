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
	ruleService    RuleServicer
}

// NewTransactionService creates a new TransactionServicer. The ruleService is
// used to auto-categorize new income/expense transactions and to run rule
// backfills; the dependency is one-way (rules never depend on transactions).
func NewTransactionService(db *gorm.DB, accountService AccountServicer, ruleService RuleServicer) TransactionServicer {
	return &transactionService{
		db:             db,
		accountService: accountService,
		ruleService:    ruleService,
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

	// Auto-categorize via rules when the caller didn't specify a category and the
	// transaction is a categorizable income/expense (plan 018). Resolved outside
	// the write transaction so rule reads don't extend the account-row lock. A
	// rule-resolution error never blocks transaction creation.
	if categoryID == nil && isCategorizableType(transactionType) && s.ruleService != nil {
		if res, rerr := s.ruleService.ResolveForUser(userID, RuleInput{
			Description: description,
			Amount:      amount,
			AccountID:   accountID,
			Type:        transactionType,
		}); rerr == nil {
			categoryID = res.CategoryID
		}
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

// isCategorizableType reports whether a transaction type is eligible for
// auto-categorization. Transfers and investment legs are excluded by design.
func isCategorizableType(t models.TransactionType) bool {
	return t == models.TransactionTypeIncome || t == models.TransactionTypeExpense
}

// ruleSampleLimit caps the number of sample transactions returned by preview/apply.
const ruleSampleLimit = 10

// candidateTransactions returns a user's categorizable (income/expense)
// transactions, newest first. scope filters to uncategorized rows when given.
func (s *transactionService) candidateTransactions(userID string, uncategorizedOnly bool) ([]models.Transaction, error) {
	q := s.db.
		Where("user_id = ? AND type IN ?", userID,
			[]models.TransactionType{models.TransactionTypeIncome, models.TransactionTypeExpense})
	if uncategorizedOnly {
		q = q.Where("category_id IS NULL")
	}
	var txns []models.Transaction
	if err := q.Order("date DESC, created_at DESC").Find(&txns).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return txns, nil
}

// txToRuleInput projects a transaction into the matcher's DB-free input.
func txToRuleInput(t *models.Transaction) RuleInput {
	return RuleInput{
		Description: t.Description,
		Amount:      t.Amount,
		AccountID:   t.AccountID,
		Type:        t.Type,
	}
}

// PreviewRuleMatches counts existing transactions matching a set of unsaved
// conditions, returning a small sample. Read-only; used by the rule builder UI.
func (s *transactionService) PreviewRuleMatches(userID string, conditions []RuleConditionInput) (*RuleMatchPreview, error) {
	modelConditions := inputsToConditions(conditions)

	txns, err := s.candidateTransactions(userID, false)
	if err != nil {
		return nil, err
	}

	preview := &RuleMatchPreview{Sample: []models.Transaction{}}
	for i := range txns {
		if ConditionsMatch(modelConditions, txToRuleInput(&txns[i])) {
			preview.Count++
			if len(preview.Sample) < ruleSampleLimit {
				preview.Sample = append(preview.Sample, txns[i])
			}
		}
	}
	return preview, nil
}

// ApplyRule backfills an existing rule over existing transactions. It resolves
// each candidate through the shared matcher and applies a balance-neutral,
// category-only update (never the balance-reversing UpdateTransaction path).
// On dry run it reports the count and a sample without writing.
func (s *transactionService) ApplyRule(userID, ruleID string, opts ApplyRuleOptions) (*ApplyRuleResult, error) {
	rule, err := s.ruleService.GetRule(userID, ruleID)
	if err != nil {
		return nil, err
	}

	uncategorizedOnly := opts.Scope != RuleApplyScopeAll
	txns, err := s.candidateTransactions(userID, uncategorizedOnly)
	if err != nil {
		return nil, err
	}

	// A backfill is an explicit action on a specific rule, so honor it even when
	// the rule is paused (is_active=false). Target validity and type-safety are
	// still enforced by the matcher (a deleted target category won't apply).
	active := *rule
	active.IsActive = true
	rules := []models.TransactionRule{active}
	result := &ApplyRuleResult{Sample: []models.Transaction{}}

	var eligible []models.Transaction
	for i := range txns {
		t := &txns[i]
		res := Match(rules, txToRuleInput(t))
		if res.CategoryID == nil {
			continue
		}
		// Skip if the target equals the current category (no-op) or the row is
		// already categorized and overwrite wasn't requested.
		if t.CategoryID != nil {
			if !opts.Overwrite || *t.CategoryID == *res.CategoryID {
				continue
			}
		}
		result.Count++
		if len(result.Sample) < ruleSampleLimit {
			result.Sample = append(result.Sample, *t)
		}
		eligible = append(eligible, models.Transaction{Base: models.Base{ID: t.ID}, CategoryID: res.CategoryID})
	}

	if opts.DryRun || len(eligible) == 0 {
		return result, nil
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		for i := range eligible {
			if txErr := tx.Model(&models.Transaction{}).
				Where("id = ? AND user_id = ?", eligible[i].ID, userID).
				Update("category_id", eligible[i].CategoryID).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result.Applied = len(eligible)
	return result, nil
}

// inputsToConditions maps condition inputs into model conditions for the matcher.
func inputsToConditions(conditions []RuleConditionInput) []models.TransactionRuleCondition {
	out := make([]models.TransactionRuleCondition, len(conditions))
	for i, c := range conditions {
		out[i] = models.TransactionRuleCondition{
			Field:     c.Field,
			Operator:  c.Operator,
			ValueText: c.ValueText,
			AmountMin: c.AmountMin,
			AmountMax: c.AmountMax,
		}
	}
	return out
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

// GetDailySummary returns daily income and expense totals for a date range.
func (s *transactionService) GetDailySummary(userID string, from, to time.Time) ([]DailySummaryItem, error) {
	// Normalize to start/end of day
	current := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)

	var items []DailySummaryItem

	for !current.After(end) {
		dayStart := current
		dayEnd := current.Add(24*time.Hour - time.Nanosecond)

		var income int64
		if err := s.db.Model(&models.Transaction{}).
			Select("COALESCE(SUM(amount), 0)").
			Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ? AND description != ?",
				userID, models.TransactionTypeIncome, dayStart, dayEnd, "Initial balance").
			Scan(&income).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		var expenses int64
		if err := s.db.Model(&models.Transaction{}).
			Select("COALESCE(SUM(amount), 0)").
			Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
				userID, models.TransactionTypeExpense, dayStart, dayEnd).
			Scan(&expenses).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		items = append(items, DailySummaryItem{
			Date:     dayStart.Format("2006-01-02"),
			Income:   income,
			Expenses: expenses,
		})

		current = current.AddDate(0, 0, 1)
	}

	if items == nil {
		items = []DailySummaryItem{}
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

// totalsByCategory groups transactions of a single type by category and enriches
// each row with category metadata (name, color, icon). Rows are sorted by amount
// descending. Uncategorized transactions collapse into a single "Uncategorized"
// row. The returned slice is never nil.
func (s *transactionService) totalsByCategory(userID string, txnType models.TransactionType, from, to time.Time) ([]SpendingByCategoryItem, int64, error) {
	type categorySpend struct {
		CategoryID *string
		Total      int64
	}

	var results []categorySpend
	err := s.db.Model(&models.Transaction{}).
		Select("category_id, COALESCE(SUM(amount), 0) as total").
		Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
			userID, txnType, from, to).
		Group("category_id").
		Scan(&results).Error
	if err != nil {
		return nil, 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var total int64
	items := make([]SpendingByCategoryItem, 0, len(results))
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

		total += r.Total
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Total > items[j].Total
	})

	return items, total, nil
}

// GetSpendingByCategory returns expense totals grouped by category for a date range.
func (s *transactionService) GetSpendingByCategory(userID string, from, to time.Time) (*SpendingByCategory, error) {
	items, totalSpent, err := s.totalsByCategory(userID, models.TransactionTypeExpense, from, to)
	if err != nil {
		return nil, err
	}

	return &SpendingByCategory{
		Items:      items,
		TotalSpent: totalSpent,
		FromDate:   from,
		ToDate:     to,
	}, nil
}

// GetCashflow returns income and expense totals grouped by category for a date
// range, for rendering the income-to-expense cashflow Sankey. Transfers and
// investment transactions are excluded (only "income" and "expense" types are
// aggregated), so the flows reflect real cash entering and leaving.
func (s *transactionService) GetCashflow(userID string, from, to time.Time) (*Cashflow, error) {
	income, totalIncome, err := s.totalsByCategory(userID, models.TransactionTypeIncome, from, to)
	if err != nil {
		return nil, err
	}

	expenses, totalExpenses, err := s.totalsByCategory(userID, models.TransactionTypeExpense, from, to)
	if err != nil {
		return nil, err
	}

	return &Cashflow{
		Income:        income,
		Expenses:      expenses,
		TotalIncome:   totalIncome,
		TotalExpenses: totalExpenses,
		FromDate:      from,
		ToDate:        to,
	}, nil
}

// GetTopExpenses returns the largest expense transactions for a date range, ordered by amount descending.
func (s *transactionService) GetTopExpenses(userID string, from, to time.Time, limit int, categoryID *string) (*TopExpenses, error) {
	type expenseRow struct {
		ID          string
		AccountID   string
		CategoryID  *string
		Amount      int64
		Description string
		Date        time.Time
	}

	query := s.db.Model(&models.Transaction{}).
		Select("id, account_id, category_id, amount, description, date").
		Where("user_id = ? AND type = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?",
			userID, models.TransactionTypeExpense, from, to)
	if categoryID != nil {
		query = query.Where("category_id = ?", *categoryID)
	}

	var rows []expenseRow
	err := query.
		Order("amount DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	items := make([]TopExpenseItem, 0, len(rows))
	for _, r := range rows {
		item := TopExpenseItem{
			ID:          r.ID,
			AccountID:   r.AccountID,
			CategoryID:  r.CategoryID,
			Amount:      r.Amount,
			Description: r.Description,
			Date:        r.Date,
		}

		var account models.Account
		if accErr := s.db.Select("name").Where("id = ?", r.AccountID).First(&account).Error; accErr == nil {
			item.AccountName = account.Name
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
				if item.CategoryColor == "" {
					item.CategoryColor = getCategoryColorFromID(*r.CategoryID)
				}
			}
		} else {
			item.CategoryName = "Uncategorized"
			item.CategoryColor = "#9CA3AF"
		}

		items = append(items, item)
	}

	return &TopExpenses{
		Items:    items,
		FromDate: from,
		ToDate:   to,
	}, nil
}
