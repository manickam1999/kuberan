package services

import (
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// This file holds package-level helpers shared by the receivable service and
// the transaction service. Keeping them free functions (rather than methods on
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

// recomputePersonalAmount is the only writer of transactions.personal_amount
// (plan 021, D6). It sets
//
//	personal_amount = amount - SUM(receivable.amount - receivable.written_off_amount)
//
// over the expense's non-deleted receivables, or NULL when it has none. It must
// run in the same DB transaction as every receivable mutation. The result is
// guaranteed to be within [0, amount]; otherwise ErrInvalidShares is returned
// and the surrounding transaction should roll back.
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

	var agg struct {
		N    int64
		Owed int64
	}
	if err := tx.Model(&models.Receivable{}).
		Select("COUNT(*) AS n, COALESCE(SUM(amount - written_off_amount), 0) AS owed").
		Where("transaction_id = ? AND user_id = ?", transactionID, userID).
		Scan(&agg).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var value interface{} = gorm.Expr("NULL")
	if agg.N > 0 {
		personal := txn.Amount - agg.Owed
		if personal < 0 || personal > txn.Amount {
			return apperrors.WithMessage(apperrors.ErrInvalidShares, "shares must not exceed the expense amount")
		}
		value = personal
	}

	if err := tx.Model(&models.Transaction{}).
		Where("id = ? AND user_id = ?", transactionID, userID).
		UpdateColumn("personal_amount", value).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
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

	var recs []models.Receivable
	if err := db.Where("user_id = ? AND transaction_id IN ?", userID, ids).
		Order("created_at ASC, id ASC").
		Find(&recs).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	byTxn := make(map[string][]models.Receivable, len(ids))
	for i := range recs {
		recs[i].ComputeOutstanding()
		byTxn[recs[i].TransactionID] = append(byTxn[recs[i].TransactionID], recs[i])
	}
	for i := range txns {
		if rs, ok := byTxn[txns[i].ID]; ok {
			txns[i].Receivables = rs
		}
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
