package services

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// This file holds package-level helpers shared by the receivable service, the
// transaction service and the other balance writers. Keeping them free functions (rather than methods on
// either service) lets both use them without depending on each other.

// lockUpdate adds a row-level FOR UPDATE lock to a query (plan 021, D10). The
// SQLite driver used by the tests drops the clause, so it is a no-op there; on
// PostgreSQL it takes the real lock. It must be called on a *gorm.DB that is
// inside a transaction.
func lockUpdate(tx *gorm.DB) *gorm.DB {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// lockTransaction loads and row-locks a user's transaction. Soft-deleted rows
// are not found.
func lockTransaction(tx *gorm.DB, userID, transactionID string) (*models.Transaction, error) {
	var txn models.Transaction
	if err := lockUpdate(tx).Where("id = ? AND user_id = ?", transactionID, userID).First(&txn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTransactionNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &txn, nil
}

// lockReceivablesForTransaction loads and row-locks the non-deleted receivables
// of an expense, ordered by id so concurrent operations always lock in the same order.
func lockReceivablesForTransaction(tx *gorm.DB, userID, transactionID string) ([]models.Receivable, error) {
	var recs []models.Receivable
	if err := lockUpdate(tx).
		Where("transaction_id = ? AND user_id = ?", transactionID, userID).
		Order("id ASC").
		Find(&recs).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for i := range recs {
		recs[i].ComputeOutstanding()
	}
	return recs, nil
}

// lockAccounts row-locks the given accounts in ascending id order and returns
// them by id. Duplicate ids are locked once. When requireActive is set an
// inactive account is treated as missing, matching AccountServicer.GetAccountByID.
func lockAccounts(tx *gorm.DB, userID string, accountIDs []string, requireActive bool) (map[string]*models.Account, error) {
	ids := make([]string, 0, len(accountIDs))
	seen := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	accounts := make(map[string]*models.Account, len(ids))
	for _, id := range ids {
		q := lockUpdate(tx).Where("id = ? AND user_id = ?", id, userID)
		if requireActive {
			q = q.Where("is_active = ?", true)
		}
		var acct models.Account
		if err := q.First(&acct).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.ErrAccountNotFound
			}
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		accounts[id] = &acct
	}
	return accounts, nil
}

// personalAmountFor computes the D6 value for an expense of the given amount:
//
//	amount - SUM(receivable.amount - receivable.written_off_amount)
//
// over the expense's non-deleted receivables, or nil when it has none. It is
// the single place the invariant is computed. ErrInvalidShares is returned when
// the result would fall outside [0, amount].
func personalAmountFor(tx *gorm.DB, userID, transactionID string, amount int64) (*int64, error) {
	var agg struct {
		N    int64
		Owed int64
	}
	if err := tx.Model(&models.Receivable{}).
		Select("COUNT(*) AS n, COALESCE(SUM(amount - written_off_amount), 0) AS owed").
		Where("transaction_id = ? AND user_id = ?", transactionID, userID).
		Scan(&agg).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if agg.N == 0 {
		return nil, nil
	}
	personal := amount - agg.Owed
	if personal < 0 || personal > amount {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidShares, "shares must not exceed the expense amount")
	}
	return &personal, nil
}

// recomputePersonalAmount writes transactions.personal_amount from
// personalAmountFor (plan 021, D6). It must run in the same DB transaction as
// every receivable mutation; when ErrInvalidShares is returned the surrounding
// transaction should roll back. UpdateTransaction, which changes the amount
// itself, computes the value with personalAmountFor and writes it in the same
// UPDATE instead (the CHECK constraint is evaluated per statement).
func recomputePersonalAmount(tx *gorm.DB, userID, transactionID string) error {
	var txn models.Transaction
	if err := tx.Select("id", "amount").
		Where("id = ? AND user_id = ?", transactionID, userID).
		First(&txn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrTransactionNotFound
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	personal, err := personalAmountFor(tx, userID, transactionID, txn.Amount)
	if err != nil {
		return err
	}
	var value interface{} = gorm.Expr("NULL")
	if personal != nil {
		value = *personal
	}
	if err := tx.Model(&models.Transaction{}).
		Where("id = ? AND user_id = ?", transactionID, userID).
		UpdateColumn("personal_amount", value).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// receivablesByTransaction loads the non-deleted receivables (without
// settlements, with Outstanding computed) of the given transactions, grouped by
// transaction id.
func receivablesByTransaction(db *gorm.DB, userID string, transactionIDs []string) (map[string][]models.Receivable, error) {
	var recs []models.Receivable
	if err := db.Where("user_id = ? AND transaction_id IN ?", userID, transactionIDs).
		Order("created_at ASC, id ASC").
		Find(&recs).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	byTxn := make(map[string][]models.Receivable, len(transactionIDs))
	for i := range recs {
		recs[i].ComputeOutstanding()
		byTxn[recs[i].TransactionID] = append(byTxn[recs[i].TransactionID], recs[i])
	}
	return byTxn, nil
}

// attachReceivables fills Receivables (without settlements, with Outstanding
// computed) on the expense rows of txns using one query. Non-expense rows are
// left untouched.
func attachReceivables(db *gorm.DB, userID string, txns []models.Transaction) error {
	ids := make([]string, 0, len(txns))
	for i := range txns {
		if txns[i].Type == models.TransactionTypeExpense {
			ids = append(ids, txns[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	byTxn, err := receivablesByTransaction(db, userID, ids)
	if err != nil {
		return err
	}
	for i := range txns {
		if rs, ok := byTxn[txns[i].ID]; ok {
			txns[i].Receivables = rs
		}
	}
	return nil
}

// loadExpenseWithReceivables reads a user's transaction by id and, when it is an
// expense, attaches its receivables (without settlements). It is the single
// "read one transaction back" path for the transaction and receivable services.
func loadExpenseWithReceivables(db *gorm.DB, userID, transactionID string) (*models.Transaction, error) {
	var txn models.Transaction
	if err := db.Where("id = ? AND user_id = ?", transactionID, userID).First(&txn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTransactionNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if txn.Type == models.TransactionTypeExpense {
		byTxn, err := receivablesByTransaction(db, userID, []string{txn.ID})
		if err != nil {
			return nil, err
		}
		txn.Receivables = byTxn[txn.ID]
	}
	return &txn, nil
}

// validateTransactionInput checks the fields shared by every user-created
// income/expense: a positive amount, an account id and a clean description.
func validateTransactionInput(accountID string, amount int64, description string) error {
	if amount <= 0 {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "amount must be greater than zero")
	}
	if accountID == "" {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "account ID is required")
	}
	return validateDescription(description)
}

// hasControlChars reports whether s contains a Unicode control character other
// than those in allowed, or is not valid UTF-8. PostgreSQL rejects NUL and
// invalid UTF-8 in text columns, which would otherwise surface as a 500.
func hasControlChars(s, allowed string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) && !strings.ContainsRune(allowed, r)
	}) >= 0
}

// validateDescription rejects control characters other than newline and tab in
// a transaction description (INVALID_INPUT).
func validateDescription(description string) error {
	if hasControlChars(description, "\n\t") {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "description must not contain control characters")
	}
	return nil
}

// normalizeSearchKey normalizes a user-supplied counterparty filter or search
// text to a key and rejects control characters (INVALID_INPUT). Whitespace
// controls such as tab and newline are collapsed by the normalization first.
func normalizeSearchKey(raw string) (string, error) {
	key := models.NormalizeCounterpartyKey(raw)
	if hasControlChars(key, "") {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "search text must not contain control characters")
	}
	return key, nil
}

// resolveRuleCategory returns the category a new transaction should get: the
// explicit categoryID when set, otherwise the first matching auto-categorization
// rule (plan 018) for categorizable types. It runs outside the write
// transaction so rule reads don't extend the account-row lock, and a rule error
// never blocks creation (the category is simply left unset).
func resolveRuleCategory(rules RuleServicer, userID string, categoryID *string, in RuleInput) *string {
	if categoryID != nil || rules == nil || !isCategorizableType(in.Type) {
		return categoryID
	}
	res, err := rules.ResolveForUser(userID, in)
	if err != nil {
		return nil
	}
	return res.CategoryID
}

// insertTransaction creates a user transaction row and applies its balance
// effect to account with a relative update (plan 021, D10). The caller must
// already have locked the account inside tx (lockAccounts).
func insertTransaction(
	tx *gorm.DB,
	accounts AccountServicer,
	userID string,
	account *models.Account,
	categoryID *string,
	txnType models.TransactionType,
	amount int64,
	description string,
	date time.Time,
) (*models.Transaction, error) {
	txn := &models.Transaction{
		UserID:      userID,
		AccountID:   account.ID,
		CategoryID:  categoryID,
		Type:        txnType,
		Amount:      amount,
		Description: description,
		Date:        date,
	}
	if err := tx.Create(txn).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if err := accounts.ApplyBalanceDelta(tx, account.ID, account.Type, txnType, amount); err != nil {
		return nil, err
	}
	return txn, nil
}

// insertReceivable creates one receivable for an expense with its derived
// status. createdAt is set explicitly so a batch keeps its request order.
func insertReceivable(tx *gorm.DB, userID, transactionID, name, key string, kind models.ReceivableKind, amount int64, createdAt time.Time) error {
	rec := &models.Receivable{
		UserID:           userID,
		TransactionID:    transactionID,
		CounterpartyName: name,
		CounterpartyKey:  key,
		Kind:             kind,
		Amount:           amount,
	}
	rec.CreatedAt = createdAt
	rec.Status = rec.DeriveStatus()
	if err := tx.Create(rec).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// truncateRunes returns s cut to at most limit runes.
func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
