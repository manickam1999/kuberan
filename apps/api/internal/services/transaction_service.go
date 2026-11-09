package services

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// transactionService handles transaction-related business logic.
type transactionService struct {
	db             *gorm.DB
	accountService AccountServicer
	ruleService    RuleServicer
	fxService      FXServicer // optional; suggests a to_amount for cross-currency transfers
}

// TransactionServiceOption configures optional dependencies of the transaction service.
type TransactionServiceOption func(*transactionService)

// WithTransactionFXService sets the FX service used to suggest the destination
// amount of a cross-currency transfer. Without it the FX_AMOUNT_REQUIRED error
// carries no suggestion.
func WithTransactionFXService(fxs FXServicer) TransactionServiceOption {
	return func(s *transactionService) { s.fxService = fxs }
}

// NewTransactionService creates a new TransactionServicer. The ruleService is
// used to auto-categorize new income/expense transactions and to run rule
// backfills; the dependency is one-way (rules never depend on transactions).
func NewTransactionService(db *gorm.DB, accountService AccountServicer, ruleService RuleServicer, opts ...TransactionServiceOption) TransactionServicer {
	s := &transactionService{
		db:             db,
		accountService: accountService,
		ruleService:    ruleService,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// effectiveAmountSQL is the SQL expression for the amount that counts as the
// user's own spending: the personal share of a shared expense, or the full amount
// when the expense is not shared (personal_amount IS NULL). Every spending
// aggregate over expenses must sum this instead of the raw amount (plan 021, D1).
const effectiveAmountSQL = "COALESCE(personal_amount, amount)"

// CreateTransaction creates a new transaction for a user's account. The account
// is row-locked inside the DB transaction and its balance moves by a relative
// update, so concurrent writers never overwrite each other (plan 021, D10).
func (s *transactionService) CreateTransaction(
	userID string,
	accountID string,
	categoryID *string,
	transactionType models.TransactionType,
	amount int64,
	description string,
	date time.Time,
	meta TransactionMeta,
) (*models.Transaction, error) {
	if err := validateTransactionInput(accountID, amount, description); err != nil {
		return nil, err
	}
	meta, err := normalizeMeta(meta)
	if err != nil {
		return nil, err
	}
	// Opening balances are written only by account creation.
	if transactionType == models.TransactionTypeOpeningBalance {
		return nil, apperrors.ErrInvalidTransactionType
	}
	if date.IsZero() {
		date = time.Now()
	}
	if categoryID != nil && *categoryID == "" {
		categoryID = nil
	}
	if err := ensureCategoryOwned(s.db, userID, categoryID); err != nil {
		return nil, err
	}

	// Auto-categorize via rules when the caller didn't specify a category and the
	// transaction is a categorizable income/expense (plan 018). Rule actions are
	// validated against the user's categories when the rule is saved.
	categoryID = resolveRuleCategory(s.ruleService, userID, categoryID, RuleInput{
		Description: description,
		Amount:      amount,
		AccountID:   accountID,
		Type:        transactionType,
	})

	var result *models.Transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		accounts, txErr := lockAccounts(tx, userID, []string{accountID}, true)
		if txErr != nil {
			return txErr
		}
		result, txErr = insertTransaction(tx, s.accountService, userID, accounts[accountID],
			categoryID, transactionType, amount, description, date, meta)
		return txErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CreateTransfer creates an account-to-account transfer within a single DB
// transaction. Both accounts are row-locked (in id order) and the balances move
// by relative updates; the insufficient-balance check runs on the locked row.
//
// The source moves by amount and the destination by the effective destination
// amount. For accounts in the same currency toAmount must be nil or equal to
// amount (nothing is stored). For different currencies toAmount is required
// (FX_AMOUNT_REQUIRED with a suggestion otherwise) and is stored on the row.
func (s *transactionService) CreateTransfer(
	userID, fromAccountID, toAccountID string,
	amount int64,
	toAmount *int64,
	description string,
	date time.Time,
	meta TransactionMeta,
) (*models.Transaction, error) {
	if fromAccountID == toAccountID {
		return nil, apperrors.ErrSameAccountTransfer
	}

	if amount <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "amount must be greater than zero")
	}
	if err := validateDescription(description); err != nil {
		return nil, err
	}
	meta, err := normalizeMeta(meta)
	if err != nil {
		return nil, err
	}

	if date.IsZero() {
		date = time.Now()
	}

	var result *models.Transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		accounts, txErr := lockAccounts(tx, userID, []string{fromAccountID, toAccountID}, true)
		if txErr != nil {
			return txErr
		}
		fromAccount, toAccount := accounts[fromAccountID], accounts[toAccountID]

		stored, txErr := s.resolveToAmount(fromAccount, toAccount, amount, toAmount, date)
		if txErr != nil {
			return txErr
		}

		if !isLiabilityAccount(fromAccount.Type) && fromAccount.Balance < amount {
			return apperrors.ErrInsufficientBalance
		}

		transaction := &models.Transaction{
			UserID:      userID,
			AccountID:   fromAccountID,
			ToAccountID: &toAccountID,
			Type:        models.TransactionTypeTransfer,
			Amount:      amount,
			ToAmount:    stored,
			Description: description,
			Date:        date,

			Notes:          meta.Notes,
			Source:         meta.Source,
			SourceClientID: sourceClientPtr(meta.SourceClientID),
		}
		if txErr := tx.Create(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		transaction.FillImpliedRate()

		if txErr := s.accountService.ApplyBalanceDelta(tx, fromAccount.ID, fromAccount.Type, models.TransactionTypeExpense, amount); txErr != nil {
			return txErr
		}
		if txErr := s.accountService.ApplyBalanceDelta(tx, toAccount.ID, toAccount.Type, models.TransactionTypeIncome, transaction.EffectiveToAmount()); txErr != nil {
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

// resolveToAmount validates a transfer's destination amount against the two
// accounts' currencies and returns the value to store in transactions.to_amount:
// nil for a same-currency transfer, the given amount for a cross-currency one.
func (s *transactionService) resolveToAmount(from, to *models.Account, amount int64, toAmount *int64, at time.Time) (*int64, error) {
	if toAmount != nil && *toAmount <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_amount must be greater than zero")
	}
	if strings.EqualFold(from.Currency, to.Currency) {
		if toAmount != nil && *toAmount != amount {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
				"to_amount must be omitted or equal amount when both accounts share a currency")
		}
		return nil, nil
	}
	if toAmount == nil {
		return nil, fxAmountRequiredError(s.fxService, strings.ToUpper(from.Currency), strings.ToUpper(to.Currency), amount, at, "to_amount")
	}
	v := *toAmount
	return &v, nil
}

// isCategorizableType reports whether a transaction type is eligible for
// auto-categorization. Transfers and investment legs are excluded by design.
func isCategorizableType(t models.TransactionType) bool {
	return t == models.TransactionTypeIncome || t == models.TransactionTypeExpense
}

// ruleSampleLimit caps the number of sample transactions returned by preview/apply.
const ruleSampleLimit = 10

// ruleCandidate is the projection of a transaction the rule matcher needs.
type ruleCandidate struct {
	ID          string
	AccountID   string
	CategoryID  *string
	Type        models.TransactionType
	Amount      int64
	Description string
}

// ruleUpdateBatchSize bounds the id list of each grouped category UPDATE.
const ruleUpdateBatchSize = 500

// candidateTransactions returns a user's categorizable (income/expense)
// transactions, newest first, projected to the columns the matcher needs.
// scope filters to uncategorized rows when given.
func (s *transactionService) candidateTransactions(userID string, uncategorizedOnly bool) ([]ruleCandidate, error) {
	q := s.db.Model(&models.Transaction{}).
		Select("id", "account_id", "category_id", "type", "amount", "description").
		Where("user_id = ? AND type IN ?", userID,
			[]models.TransactionType{models.TransactionTypeIncome, models.TransactionTypeExpense})
	if uncategorizedOnly {
		q = q.Where("category_id IS NULL")
	}
	var txns []ruleCandidate
	if err := q.Order("date DESC, created_at DESC").Scan(&txns).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return txns, nil
}

// loadSampleTransactions loads full transaction rows for the given ids,
// returned in the order of ids (used for the small preview/apply sample).
func (s *transactionService) loadSampleTransactions(userID string, ids []string) ([]models.Transaction, error) {
	out := make([]models.Transaction, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []models.Transaction
	if err := s.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	byID := make(map[string]models.Transaction, len(rows))
	for i := range rows {
		byID[rows[i].ID] = rows[i]
	}
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// txToRuleInput projects a rule candidate into the matcher's DB-free input.
func txToRuleInput(t *ruleCandidate) RuleInput {
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

	preview := &RuleMatchPreview{}
	var sampleIDs []string
	for i := range txns {
		if ConditionsMatch(modelConditions, txToRuleInput(&txns[i])) {
			preview.Count++
			if len(sampleIDs) < ruleSampleLimit {
				sampleIDs = append(sampleIDs, txns[i].ID)
			}
		}
	}
	preview.Sample, err = s.loadSampleTransactions(userID, sampleIDs)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

// ApplyRule backfills an existing rule over existing transactions. It resolves
// each candidate through the shared matcher and applies a balance-neutral,
// category-only update (never the balance-reversing UpdateTransaction path),
// issuing one user-scoped UPDATE ... WHERE id IN (...) per target category and
// id batch. On dry run it reports the count and a sample without writing.
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
	result := &ApplyRuleResult{}

	// Ids to update, grouped by target category (in first-seen order).
	byCategory := make(map[string][]string)
	var categoryOrder []string
	var sampleIDs []string
	eligible := 0
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
		eligible++
		if len(sampleIDs) < ruleSampleLimit {
			sampleIDs = append(sampleIDs, t.ID)
		}
		target := *res.CategoryID
		if _, seen := byCategory[target]; !seen {
			categoryOrder = append(categoryOrder, target)
		}
		byCategory[target] = append(byCategory[target], t.ID)
	}

	// The sample reflects the rows before the update.
	result.Sample, err = s.loadSampleTransactions(userID, sampleIDs)
	if err != nil {
		return nil, err
	}

	if opts.DryRun || eligible == 0 {
		return result, nil
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, categoryID := range categoryOrder {
			ids := byCategory[categoryID]
			for start := 0; start < len(ids); start += ruleUpdateBatchSize {
				chunk := ids[start:min(start+ruleUpdateBatchSize, len(ids))]
				if txErr := tx.Model(&models.Transaction{}).
					Where("id IN ? AND user_id = ?", chunk, userID).
					Update("category_id", categoryID).Error; txErr != nil {
					return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result.Applied = eligible
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

// reverseType returns the transaction type that undoes the balance effect of t:
// income→expense, expense→income, reimbursement→expense and opening balance→its
// reversal. Any other type has no single-leg reversal and yields
// ErrInvalidTransactionType.
func reverseType(t models.TransactionType) (models.TransactionType, error) {
	switch t {
	case models.TransactionTypeOpeningBalance:
		return openingBalanceReversal, nil
	case models.TransactionTypeIncome, models.TransactionTypeReimbursement:
		return models.TransactionTypeExpense, nil
	case models.TransactionTypeExpense:
		return models.TransactionTypeIncome, nil
	default:
		return "", apperrors.ErrInvalidTransactionType
	}
}

// UpdateTransaction updates an existing income/expense transaction, or the
// amount, date and description of an opening balance. Transfer, investment and
// reimbursement transactions cannot be edited.
//
// It runs in one DB transaction under the D10 lock order (the transaction row,
// its receivables, then the affected accounts) and moves balances with relative
// updates. On a shared expense it also rejects a type change away from expense
// (INVALID_TYPE_CHANGE) and an amount below the sum of its receivables
// (INVALID_SHARES), and recomputes personal_amount.
func (s *transactionService) UpdateTransaction(userID, transactionID string, updates TransactionUpdateFields) (*models.Transaction, error) {
	if updates.Description != nil {
		if err := validateDescription(*updates.Description); err != nil {
			return nil, err
		}
	}
	if updates.Notes != nil {
		if err := validateNotes(*updates.Notes); err != nil {
			return nil, err
		}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		transaction, err := lockTransaction(tx, userID, transactionID)
		if err != nil {
			return err
		}

		// Reject transfers, investment transactions and reimbursements (the latter are
		// managed through their receivable settlement)
		if transaction.Type == models.TransactionTypeTransfer ||
			transaction.Type == models.TransactionTypeInvestment ||
			transaction.Type == models.TransactionTypeReimbursement {
			return apperrors.ErrTransactionNotEditable
		}

		// If type change requested, reject changes to/from transfer, investment,
		// reimbursement or opening balance
		if updates.Type != nil {
			newType := *updates.Type
			if newType == models.TransactionTypeTransfer || newType == models.TransactionTypeInvestment ||
				newType == models.TransactionTypeReimbursement {
				return apperrors.ErrInvalidTypeChange
			}
			if (newType == models.TransactionTypeOpeningBalance) != (transaction.Type == models.TransactionTypeOpeningBalance) {
				return apperrors.WithMessage(apperrors.ErrInvalidTypeChange, "Cannot change a transaction to or from an opening balance")
			}
		}

		// Imported rows are derived from the statement: only description,
		// category, date and notes may change (a re-import reverses them by type).
		if transaction.ImportRef != nil &&
			((updates.Type != nil && *updates.Type != transaction.Type) ||
				(updates.Amount != nil && *updates.Amount != transaction.Amount) ||
				(updates.AccountID != nil && *updates.AccountID != transaction.AccountID)) {
			return apperrors.ErrImportedTransactionLocked
		}

		// An opening balance belongs to its account and is never categorised;
		// only its amount, date and description can change.
		if transaction.Type == models.TransactionTypeOpeningBalance {
			if updates.AccountID != nil && *updates.AccountID != transaction.AccountID {
				return apperrors.WithMessage(apperrors.ErrInvalidInput, "An opening balance cannot be moved to another account")
			}
			if updates.CategoryID != nil && *updates.CategoryID != nil {
				return apperrors.WithMessage(apperrors.ErrInvalidInput, "An opening balance cannot have a category")
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

		reversal, err := reverseType(oldType)
		if err != nil {
			return err
		}

		// Shared-expense rules (plan 021, D7): lock the receivables, then validate.
		var receivables []models.Receivable
		if oldType == models.TransactionTypeExpense {
			receivables, err = lockReceivablesForTransaction(tx, userID, transaction.ID)
			if err != nil {
				return err
			}
		}
		if len(receivables) > 0 {
			if newType != models.TransactionTypeExpense {
				return apperrors.WithMessage(apperrors.ErrInvalidTypeChange, "Cannot change the type of an expense that has receivables")
			}
			var owed int64
			for i := range receivables {
				owed += receivables[i].Amount
			}
			if newAmount < owed {
				return apperrors.WithMessage(apperrors.ErrInvalidShares, "amount cannot be less than the total owed by counterparties")
			}
		}

		accounts, err := lockAccounts(tx, userID, []string{oldAccountID, newAccountID}, true)
		if err != nil {
			return err
		}
		oldAccount, targetAccount := accounts[oldAccountID], accounts[newAccountID]
		if oldAccountID != newAccountID && !strings.EqualFold(oldAccount.Currency, targetAccount.Currency) {
			return apperrors.ErrAccountCurrencyMismatch
		}

		// Reverse old impact on old account
		if txErr := s.accountService.ApplyBalanceDelta(tx, oldAccount.ID, oldAccount.Type, reversal, oldAmount); txErr != nil {
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
		if updates.Notes != nil {
			transaction.Notes = *updates.Notes
		}
		if updates.CategoryID != nil {
			if err := ensureCategoryOwned(tx, userID, *updates.CategoryID); err != nil {
				return err
			}
			if err := ensureCategoryMatchesType(tx, userID, *updates.CategoryID, newType); err != nil {
				return err
			}
			transaction.CategoryID = *updates.CategoryID
		} else if newType != oldType && transaction.CategoryID != nil {
			// A type change keeps the existing category, which must still fit.
			// Unscoped: a soft-deleted category is still a valid reference.
			if err := ensureCategoryMatchesType(tx.Unscoped(), userID, transaction.CategoryID, newType); err != nil {
				return err
			}
		}

		// personal_amount must change in the same UPDATE as amount: the
		// transactions_personal_amount_check CHECK is evaluated per statement, so
		// writing the new amount next to the old personal_amount can violate it.
		// (A shared expense is always an expense, so this runs only for those.)
		if len(receivables) > 0 {
			personal, txErr := personalAmountFor(tx, userID, transaction.ID, transaction.Amount)
			if txErr != nil {
				return txErr
			}
			transaction.PersonalAmount = personal
		}

		if txErr := tx.Omit(clause.Associations).Save(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Apply new impact on target account
		if txErr := s.accountService.ApplyBalanceDelta(tx, targetAccount.ID, targetAccount.Type, newType, newAmount); txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return loadExpenseWithReceivables(s.db, userID, transactionID)
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
	if err := attachReceivables(s.db, userID, transactions); err != nil {
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
	if f.Search != nil {
		q = applyTransactionSearch(q, *f.Search)
	}
	if f.Shared != nil {
		q = q.Where(existsClause(*f.Shared,
			"SELECT 1 FROM receivables r WHERE r.transaction_id = transactions.id AND r.deleted_at IS NULL"))
	}
	if f.HasAttachment != nil {
		q = q.Where(existsClause(*f.HasAttachment,
			"SELECT 1 FROM transaction_attachments a WHERE a.transaction_id = transactions.id AND a.deleted_at IS NULL"))
	}
	return q
}

// existsClause returns EXISTS (sub) or NOT EXISTS (sub).
func existsClause(want bool, sub string) string {
	if want {
		return "EXISTS (" + sub + ")"
	}
	return "NOT EXISTS (" + sub + ")"
}

// amountSearchPattern matches a plain amount such as "64", "64.7" or "64.73".
var amountSearchPattern = regexp.MustCompile(`^\d{1,12}(\.\d{1,2})?$`)

// searchAmountCents parses a search term like "64.73" into cents.
func searchAmountCents(term string) (int64, bool) {
	if !amountSearchPattern.MatchString(term) {
		return 0, false
	}
	whole, frac, _ := strings.Cut(term, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false
	}
	return w*100 + f, true
}

// applyTransactionSearch matches the trimmed, length-capped term against the
// description and notes (case-insensitive substring) and, for a numeric term,
// the amount, transfer to_amount or personal_amount in cents.
func applyTransactionSearch(q *gorm.DB, raw string) *gorm.DB {
	term := strings.TrimSpace(raw)
	if r := []rune(term); len(r) > MaxSearchLen {
		term = string(r[:MaxSearchLen])
	}
	if term == "" {
		return q
	}
	pattern := "%" + likeEscaper.Replace(term) + "%"
	// ILIKE lets Postgres use the trigram indexes; SQLite (tests) lacks it.
	var cond string
	if q.Name() == "postgres" {
		cond = `(description ILIKE ? ESCAPE '\' OR notes ILIKE ? ESCAPE '\'`
	} else {
		cond = `(LOWER(description) LIKE LOWER(?) ESCAPE '\' OR LOWER(notes) LIKE LOWER(?) ESCAPE '\'`
	}
	args := []any{pattern, pattern}
	if cents, ok := searchAmountCents(term); ok {
		cond += " OR amount = ? OR to_amount = ? OR personal_amount = ?"
		args = append(args, cents, cents, cents)
	}
	return q.Where(cond+")", args...)
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
	// Account and ToAccount are preloaded (two extra queries per page, not per
	// row) so clients can show account names and currencies.
	if err := base.Preload("Category").Preload("Account").Preload("ToAccount").
		Scopes(pagination.Paginate(page)).
		Order("date DESC, created_at DESC").
		Find(&transactions).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.populateAttachmentCounts(transactions); err != nil {
		return nil, err
	}
	if err := attachReceivables(s.db, userID, transactions); err != nil {
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

// GetTransactionByID retrieves a transaction by ID for a specific user. For an
// expense with receivables they are included (without settlements).
func (s *transactionService) GetTransactionByID(userID, transactionID string) (*models.Transaction, error) {
	return loadExpenseWithReceivables(s.db, userID, transactionID)
}

// DeleteTransaction deletes a transaction and reverses its effect on the account
// balance(s) with relative updates, under the D10 lock order. Reimbursements
// cannot be deleted directly. A shared expense with any repayment is refused
// (RECEIVABLE_HAS_SETTLEMENTS); otherwise its receivables are soft-deleted in
// the same DB transaction.
func (s *transactionService) DeleteTransaction(userID, transactionID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.deleteTransactionTx(tx, userID, transactionID)
	})
}

// deleteTransactionTx is DeleteTransaction inside an open DB transaction (also
// used by the bulk delete).
func (s *transactionService) deleteTransactionTx(tx *gorm.DB, userID, transactionID string) error {
	{
		transaction, err := lockTransaction(tx, userID, transactionID)
		if err != nil {
			return err
		}

		// Reimbursements are removed by deleting their receivable settlement.
		if transaction.Type == models.TransactionTypeReimbursement {
			return apperrors.ErrTransactionNotEditable
		}

		accountIDs := []string{transaction.AccountID}
		switch transaction.Type {
		case models.TransactionTypeIncome, models.TransactionTypeExpense, models.TransactionTypeOpeningBalance:
		case models.TransactionTypeTransfer:
			if transaction.ToAccountID == nil {
				return apperrors.ErrInvalidTransactionType
			}
			accountIDs = append(accountIDs, *transaction.ToAccountID)
		default:
			return apperrors.ErrInvalidTransactionType
		}

		if transaction.Type == models.TransactionTypeExpense {
			if txErr := deleteReceivablesOfExpense(tx, userID, transaction.ID); txErr != nil {
				return txErr
			}
		}

		accounts, err := lockAccounts(tx, userID, accountIDs, true)
		if err != nil {
			return err
		}
		account := accounts[transaction.AccountID]

		if txErr := tx.Delete(transaction).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		if transaction.Type == models.TransactionTypeTransfer {
			// Add back to the from-account, subtract the effective destination
			// amount (to_amount for a cross-currency transfer) from the to-account.
			toAccount := accounts[*transaction.ToAccountID]
			if txErr := s.accountService.ApplyBalanceDelta(tx, account.ID, account.Type, models.TransactionTypeIncome, transaction.Amount); txErr != nil {
				return txErr
			}
			return s.accountService.ApplyBalanceDelta(tx, toAccount.ID, toAccount.Type, models.TransactionTypeExpense, transaction.EffectiveToAmount())
		}
		reversal, err := reverseType(transaction.Type)
		if err != nil {
			return err
		}
		return s.accountService.ApplyBalanceDelta(tx, account.ID, account.Type, reversal, transaction.Amount)
	}
}

// deleteReceivablesOfExpense implements D8 inside an open DB transaction whose
// expense row is already locked: it locks the expense's receivables, refuses
// when any non-deleted settlement exists, and otherwise soft-deletes them.
func deleteReceivablesOfExpense(tx *gorm.DB, userID, transactionID string) error {
	receivables, err := lockReceivablesForTransaction(tx, userID, transactionID)
	if err != nil {
		return err
	}
	if len(receivables) == 0 {
		return nil
	}

	ids := make([]string, len(receivables))
	for i := range receivables {
		ids[i] = receivables[i].ID
	}
	var settlements int64
	if err := tx.Model(&models.ReceivableSettlement{}).
		Where("user_id = ? AND receivable_id IN ?", userID, ids).
		Count(&settlements).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if settlements > 0 {
		return apperrors.ErrReceivableHasSettlements
	}

	if err := tx.Where("transaction_id = ? AND user_id = ?", transactionID, userID).
		Delete(&models.Receivable{}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// GetMonthlySummary returns monthly income and expense totals for the last N
// months (1..24), oldest first, with empty months zero-filled. All months are
// read in a single grouped query.
func (s *transactionService) GetMonthlySummary(userID string, months int) ([]MonthlySummaryItem, error) {
	if months < minSummaryMonths || months > maxSummaryMonths {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("months must be between %d and %d", minSummaryMonths, maxSummaryMonths))
	}

	now := time.Now()
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)
	rangeEnd := startMonth.AddDate(0, months, 0).Add(-time.Nanosecond)

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}
	totals, err := periodTotals(s.db, s.fxService, userID, base, startMonth, rangeEnd, periodMonth, true)
	if err != nil {
		return nil, err
	}

	items := make([]MonthlySummaryItem, 0, months)
	current := startMonth
	for i := 0; i < months; i++ {
		key := current.Format("2006-01")
		t := totals[key]
		items = append(items, MonthlySummaryItem{
			Month:     key,
			Income:    t.Income,
			Expenses:  t.Expenses,
			Currency:  base,
			FXMissing: t.FXMissing,
		})
		current = current.AddDate(0, 1, 0)
	}

	return items, nil
}

// GetDailySpending returns daily expense totals for a date range (at most
// maxDailyRangeDays), with empty days zero-filled, from one grouped query.
func (s *transactionService) GetDailySpending(userID string, from, to time.Time) ([]DailySpendingItem, error) {
	start, last, ok, err := dailyRange(from, to)
	if err != nil {
		return nil, err
	}
	items := []DailySpendingItem{}
	if !ok {
		return items, nil
	}

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}
	totals, err := periodTotals(s.db, s.fxService, userID, base, start, endOfDay(last), periodDay, false)
	if err != nil {
		return nil, err
	}

	for current := start; !current.After(last); current = current.AddDate(0, 0, 1) {
		key := current.Format("2006-01-02")
		items = append(items, DailySpendingItem{
			Date:      key,
			Total:     totals[key].Expenses,
			Currency:  base,
			FXMissing: totals[key].FXMissing,
		})
	}

	return items, nil
}

// GetDailySummary returns daily income and expense totals for a date range (at
// most maxDailyRangeDays), with empty days zero-filled, from one grouped query.
func (s *transactionService) GetDailySummary(userID string, from, to time.Time) ([]DailySummaryItem, error) {
	start, last, ok, err := dailyRange(from, to)
	if err != nil {
		return nil, err
	}
	items := []DailySummaryItem{}
	if !ok {
		return items, nil
	}

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}
	totals, err := periodTotals(s.db, s.fxService, userID, base, start, endOfDay(last), periodDay, true)
	if err != nil {
		return nil, err
	}

	for current := start; !current.After(last); current = current.AddDate(0, 0, 1) {
		key := current.Format("2006-01-02")
		t := totals[key]
		items = append(items, DailySummaryItem{
			Date:      key,
			Income:    t.Income,
			Expenses:  t.Expenses,
			Currency:  base,
			FXMissing: t.FXMissing,
		})
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

// categoryTotals is the result of totalsByCategory.
type categoryTotals struct {
	Items       []SpendingByCategoryItem
	Total       int64
	Currency    string
	FXMissing   bool
	Unconverted []UnconvertedAmount
}

// totalsByCategory groups transactions of a single type by category and enriches
// each row with category metadata (name, color, icon). One query joins accounts
// and groups by (UTC day, account currency, category); each group is converted
// to the user's base currency at its own day's rate and rolled up per category
// in Go. Groups with no rate are left out (never counted 1:1) and reported in
// Unconverted. Rows are sorted by amount descending. Uncategorized transactions
// collapse into a single "Uncategorized" row. Categories whose converted total
// is 0 (e.g. fully reimbursable expenses) are omitted. Items is never nil.
func (s *transactionService) totalsByCategory(userID string, txnType models.TransactionType, from, to time.Time) (*categoryTotals, error) {
	type categorySpend struct {
		Bucket     string
		At         time.Time
		Currency   string
		CategoryID *string
		Total      int64
	}

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}

	dayExpr, dayGroup, isPostgres := dayExprs(s.db)
	var results []categorySpend
	err = s.db.Table("transactions t").
		Joins("JOIN accounts a ON a.id = t.account_id").
		Select(dayExpr+", a.currency AS currency, t.category_id AS category_id, COALESCE(SUM("+effectiveAmountSQL+"), 0) AS total").
		Where("t.user_id = ? AND t.type = ? AND t.deleted_at IS NULL AND t.date BETWEEN ? AND ?",
			userID, txnType, from, to).
		Group(dayGroup + ", a.currency, t.category_id").
		Having("COALESCE(SUM(" + effectiveAmountSQL + "), 0) > 0").
		Scan(&results).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	days := map[string]struct{}{}
	currencies := map[string]struct{}{}
	for _, r := range results {
		days[dayKey(isPostgres, r.Bucket, r.At)] = struct{}{}
		currencies[normalizeCode(r.Currency)] = struct{}{}
	}
	var dv *datedValuer
	if first, last, ok := dayBounds(days); ok {
		if dv, err = newDatedValuer(s.fxService, base, first, last, currencyList(currencies)); err != nil {
			return nil, err
		}
	}

	type catKey struct {
		id  string
		set bool
	}
	sums := map[catKey]int64{}
	order := []catKey{}
	unconverted := map[string]int64{}
	for _, r := range results {
		k := catKey{}
		if r.CategoryID != nil {
			k = catKey{id: *r.CategoryID, set: true}
		}
		converted, ok := dv.Convert(r.Total, r.Currency, dayKey(isPostgres, r.Bucket, r.At))
		if !ok {
			unconverted[normalizeCode(r.Currency)] += r.Total
			continue
		}
		if _, seen := sums[k]; !seen {
			order = append(order, k)
		}
		sums[k] += converted
	}

	catIDs := make([]string, 0, len(order))
	for _, k := range order {
		if k.set {
			catIDs = append(catIDs, k.id)
		}
	}
	categories, err := loadCategoriesByID(s.db, userID, catIDs)
	if err != nil {
		return nil, err
	}

	out := &categoryTotals{
		Items:       make([]SpendingByCategoryItem, 0, len(order)),
		Currency:    base,
		FXMissing:   len(unconverted) > 0,
		Unconverted: unconvertedList(unconverted),
	}
	for _, k := range order {
		if sums[k] <= 0 {
			continue
		}
		var id *string
		if k.set {
			cid := k.id
			id = &cid
		}
		item := SpendingByCategoryItem{CategoryID: id, Total: sums[k]}
		item.CategoryName, item.CategoryColor, item.CategoryIcon = categoryDisplay(categories, id)
		out.Total += sums[k]
		out.Items = append(out.Items, item)
	}

	sort.SliceStable(out.Items, func(i, j int) bool {
		return out.Items[i].Total > out.Items[j].Total
	})
	return out, nil
}

// GetSpendingByCategory returns expense totals grouped by category for a date
// range, in the user's base currency.
func (s *transactionService) GetSpendingByCategory(userID string, from, to time.Time) (*SpendingByCategory, error) {
	t, err := s.totalsByCategory(userID, models.TransactionTypeExpense, from, to)
	if err != nil {
		return nil, err
	}

	return &SpendingByCategory{
		Items:       t.Items,
		TotalSpent:  t.Total,
		FromDate:    from,
		ToDate:      to,
		Currency:    t.Currency,
		FXMissing:   t.FXMissing,
		Unconverted: t.Unconverted,
	}, nil
}

// GetCashflow returns income and expense totals grouped by category for a date
// range, for rendering the income-to-expense cashflow Sankey. Transfers and
// investment transactions are excluded (only "income" and "expense" types are
// aggregated), so the flows reflect real cash entering and leaving. Amounts are
// in the user's base currency; Unconverted merges both directions' gaps.
func (s *transactionService) GetCashflow(userID string, from, to time.Time) (*Cashflow, error) {
	income, err := s.totalsByCategory(userID, models.TransactionTypeIncome, from, to)
	if err != nil {
		return nil, err
	}

	expenses, err := s.totalsByCategory(userID, models.TransactionTypeExpense, from, to)
	if err != nil {
		return nil, err
	}

	merged := map[string]int64{}
	for _, u := range append(append([]UnconvertedAmount{}, income.Unconverted...), expenses.Unconverted...) {
		merged[u.Currency] += u.Amount
	}

	return &Cashflow{
		Income:        income.Items,
		Expenses:      expenses.Items,
		TotalIncome:   income.Total,
		TotalExpenses: expenses.Total,
		FromDate:      from,
		ToDate:        to,
		Currency:      income.Currency,
		FXMissing:     income.FXMissing || expenses.FXMissing,
		Unconverted:   unconvertedList(merged),
	}, nil
}

// GetTopExpenses returns the largest expense transactions for a date range,
// ranked by the effective (personal) amount converted to the user's base
// currency at the expense's own day. Expenses whose effective amount is 0
// (fully reimbursable) are excluded. Amount stays the full native amount;
// AmountBase is the converted effective amount. Expenses with no rate rank
// after every converted one and carry FXMissing. When every matching expense is
// in the base currency the ranking and limit are done in SQL.
func (s *transactionService) GetTopExpenses(userID string, from, to time.Time, limit int, categoryID *string) (*TopExpenses, error) {
	type expenseRow struct {
		ID             string
		AccountID      string
		CategoryID     *string
		Amount         int64
		PersonalAmount *int64
		Description    string
		Date           time.Time
		Currency       string
		Effective      int64
	}

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}

	query := s.db.Table("transactions t").
		Joins("JOIN accounts a ON a.id = t.account_id").
		Select("t.id AS id, t.account_id AS account_id, t.category_id AS category_id, t.amount AS amount, "+
			"t.personal_amount AS personal_amount, t.description AS description, t.date AS date, "+
			"a.currency AS currency, "+effectiveAmountSQL+" AS effective").
		Where("t.user_id = ? AND t.type = ? AND t.deleted_at IS NULL AND t.date BETWEEN ? AND ?",
			userID, models.TransactionTypeExpense, from, to).
		Where(effectiveAmountSQL + " > 0")
	if categoryID != nil {
		query = query.Where("t.category_id = ?", *categoryID)
	}

	var totalCount int64
	if cntErr := query.Session(&gorm.Session{}).Count(&totalCount).Error; cntErr != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, cntErr)
	}

	// Which currencies are involved decides between the SQL-ranked fast path and
	// ranking converted amounts in Go.
	var foreign int64
	if fErr := query.Session(&gorm.Session{}).Where("UPPER(a.currency) <> ?", base).
		Count(&foreign).Error; fErr != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, fErr)
	}

	var rows []expenseRow
	if foreign == 0 {
		err = query.Order(effectiveAmountSQL + " DESC").Limit(limit).Scan(&rows).Error
	} else {
		err = query.Scan(&rows).Error
	}
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	currencies := make([]string, 0, 2)
	seenCur := map[string]struct{}{}
	var first, last time.Time
	for i, r := range rows {
		c := normalizeCode(r.Currency)
		if _, ok := seenCur[c]; !ok {
			seenCur[c] = struct{}{}
			currencies = append(currencies, c)
		}
		d := utcDay(r.Date)
		if i == 0 || d.Before(first) {
			first = d
		}
		if i == 0 || d.After(last) {
			last = d
		}
	}
	var dv *datedValuer
	if len(rows) > 0 {
		if dv, err = newDatedValuer(s.fxService, base, first, last, currencies); err != nil {
			return nil, err
		}
	}

	type ranked struct {
		row     expenseRow
		base    int64
		missing bool
	}
	rankedRows := make([]ranked, 0, len(rows))
	anyMissing := false
	for _, r := range rows {
		conv, ok := dv.Convert(r.Effective, r.Currency, r.Date.UTC().Format(dayLayout))
		if !ok {
			anyMissing = true
		}
		rankedRows = append(rankedRows, ranked{row: r, base: conv, missing: !ok})
	}
	sort.SliceStable(rankedRows, func(i, j int) bool {
		a, b := rankedRows[i], rankedRows[j]
		if a.missing != b.missing {
			return !a.missing
		}
		if a.missing {
			return a.row.Effective > b.row.Effective
		}
		return a.base > b.base
	})
	if len(rankedRows) > limit {
		rankedRows = rankedRows[:limit]
	}

	catIDs := make([]string, 0, len(rankedRows))
	accIDs := make([]string, 0, len(rankedRows))
	for i := range rankedRows {
		rr := &rankedRows[i]
		accIDs = append(accIDs, rr.row.AccountID)
		if rr.row.CategoryID != nil {
			catIDs = append(catIDs, *rr.row.CategoryID)
		}
	}
	categories, err := loadCategoriesByID(s.db, userID, catIDs)
	if err != nil {
		return nil, err
	}
	accountNames, err := loadAccountNamesByID(s.db, accIDs)
	if err != nil {
		return nil, err
	}

	items := make([]TopExpenseItem, 0, len(rankedRows))
	for i := range rankedRows {
		rr := &rankedRows[i]
		r := rr.row
		item := TopExpenseItem{
			ID:             r.ID,
			AccountID:      r.AccountID,
			AccountName:    accountNames[r.AccountID],
			CategoryID:     r.CategoryID,
			Amount:         r.Amount,
			PersonalAmount: r.PersonalAmount,
			Description:    r.Description,
			Date:           r.Date,
			Currency:       normalizeCode(r.Currency),
			AmountBase:     rr.base,
			FXMissing:      rr.missing,
		}
		item.CategoryName, item.CategoryColor, item.CategoryIcon = categoryDisplay(categories, r.CategoryID)
		items = append(items, item)
	}

	return &TopExpenses{
		Items:      items,
		TotalCount: totalCount,
		FromDate:   from,
		ToDate:     to,
		Currency:   base,
		FXMissing:  anyMissing,
	}, nil
}

// uncategorizedColor is the neutral color used for uncategorized and unknown categories.
const uncategorizedColor = "#9CA3AF"

// loadCategoriesByID batch-loads a user's non-deleted categories by id. Ids
// that are missing (deleted or not the user's) are absent from the map.
func loadCategoriesByID(db *gorm.DB, userID string, ids []string) (map[string]models.Category, error) {
	out := make(map[string]models.Category, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var cats []models.Category
	if err := db.Where("id IN ? AND user_id = ?", ids, userID).Find(&cats).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for i := range cats {
		out[cats[i].ID] = cats[i]
	}
	return out, nil
}

// loadAccountNamesByID batch-loads the names of non-deleted accounts by id.
func loadAccountNamesByID(db *gorm.DB, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var accounts []models.Account
	if err := db.Select("id", "name").Where("id IN ?", ids).Find(&accounts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for i := range accounts {
		out[accounts[i].ID] = accounts[i].Name
	}
	return out, nil
}

// categoryDisplay resolves the display name, color and icon for a category id:
// "Uncategorized" for nil, "Unknown Category" for an id missing from cats, and
// a deterministic palette color when the category has none set.
func categoryDisplay(cats map[string]models.Category, categoryID *string) (name, color, icon string) {
	if categoryID == nil {
		return "Uncategorized", uncategorizedColor, ""
	}
	category, ok := cats[*categoryID]
	if !ok {
		return "Unknown Category", uncategorizedColor, ""
	}
	color = category.Color
	if color == "" {
		color = getCategoryColorFromID(*categoryID)
	}
	return category.Name, color, category.Icon
}
