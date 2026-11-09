package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// MaxBulkTransactions is the most transactions one bulk request may touch.
const MaxBulkTransactions = 200

// Audit actions written per transaction by a bulk request.
const (
	auditBulkUpdate = "BULK_UPDATE_TRANSACTION"
	auditBulkDelete = "BULK_DELETE_TRANSACTION"
)

// normalizeMeta validates notes and source and fills in the default source.
func normalizeMeta(meta TransactionMeta) (TransactionMeta, error) {
	if err := validateNotes(meta.Notes); err != nil {
		return meta, err
	}
	if meta.Source == "" {
		meta.Source = models.TransactionSourceWeb
	}
	if !meta.Source.Valid() {
		return meta, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid transaction source")
	}
	return meta, nil
}

// validateNotes enforces the notes length (in characters) and rejects control
// characters other than newline, carriage return and tab.
func validateNotes(notes string) error {
	if err := CheckTextLength("notes", notes, MaxNotesLen); err != nil {
		return err
	}
	if hasControlChars(notes, "\n\r\t") {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "notes must not contain control characters")
	}
	return nil
}

// sourceClientPtr returns the client id as a nullable column value.
func sourceClientPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// BulkTransactionSet is the set of changes a bulk request applies to every
// selected transaction.
type BulkTransactionSet struct {
	// CategoryID assigns this category (must match each transaction's type).
	CategoryID *string
	// NotesAppend is appended to the existing notes on a new line.
	NotesAppend *string
	// Shared may only be false: it removes the receivables of the selected
	// expenses (refused while any has a settlement) and restores the full amount.
	Shared *bool
}

// BulkTransactionInput is a bulk edit or delete of up to MaxBulkTransactions
// transactions.
type BulkTransactionInput struct {
	IDs    []string
	Set    BulkTransactionSet
	Delete bool
	// IPAddress is recorded on the audit rows.
	IPAddress string
}

// BulkTransactionResult reports what a bulk request did.
type BulkTransactionResult struct {
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}

// validateBulkInput checks the request shape and returns the de-duplicated ids.
func validateBulkInput(in BulkTransactionInput) ([]string, error) {
	if len(in.IDs) == 0 || len(in.IDs) > MaxBulkTransactions {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("ids must contain between 1 and %d transactions", MaxBulkTransactions))
	}
	seen := make(map[string]struct{}, len(in.IDs))
	ids := make([]string, 0, len(in.IDs))
	for _, id := range in.IDs {
		if id == "" {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "ids must not contain empty values")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	hasSet := in.Set.CategoryID != nil || in.Set.NotesAppend != nil || in.Set.Shared != nil
	switch {
	case in.Delete && hasSet:
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "delete cannot be combined with set")
	case !in.Delete && !hasSet:
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "set must contain at least one change, or delete must be true")
	}
	if in.Set.Shared != nil && *in.Set.Shared {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "shared can only be set to false in bulk")
	}
	if in.Set.CategoryID != nil && *in.Set.CategoryID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "category_id must not be empty")
	}
	if in.Set.NotesAppend != nil {
		if strings.TrimSpace(*in.Set.NotesAppend) == "" {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "notes_append must not be empty")
		}
		if err := validateNotes(*in.Set.NotesAppend); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// BulkUpdateTransactions applies a bulk edit (category, notes append, unshare)
// or a bulk delete to the user's transactions in one DB transaction, writing
// one audit row per changed transaction. Every id must belong to the user
// (TRANSACTION_NOT_FOUND otherwise) and any failure rolls the whole request
// back, audit rows included. Opening balances are rejected, and a category
// must match each transaction's type (CATEGORY_TYPE_MISMATCH).
func (s *transactionService) BulkUpdateTransactions(userID string, in BulkTransactionInput) (*BulkTransactionResult, error) {
	ids, err := validateBulkInput(in)
	if err != nil {
		return nil, err
	}

	result := &BulkTransactionResult{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var category *models.Category
		if in.Set.CategoryID != nil {
			category = &models.Category{}
			if err := tx.Where("id = ? AND user_id = ?", *in.Set.CategoryID, userID).First(category).Error; err != nil {
				return apperrors.ErrCategoryNotFound
			}
		}

		// Lock all rows in id order so concurrent bulk requests cannot deadlock.
		var rows []models.Transaction
		if err := lockUpdate(tx).Where("user_id = ? AND id IN ?", userID, ids).
			Order("id ASC").Find(&rows).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if len(rows) != len(ids) {
			return apperrors.ErrTransactionNotFound
		}

		for i := range rows {
			row := &rows[i]
			if row.Type == models.TransactionTypeOpeningBalance {
				return apperrors.WithMessage(apperrors.ErrTransactionNotEditable, "An opening balance cannot be changed in bulk")
			}
			if in.Delete {
				if err := s.deleteTransactionTx(tx, userID, row.ID); err != nil {
					return err
				}
				if err := writeAuditTx(tx, userID, auditBulkDelete, row.ID, in.IPAddress,
					map[string]any{"type": row.Type, "amount": row.Amount}); err != nil {
					return err
				}
				result.Deleted++
				continue
			}
			changes, err := applyBulkSet(tx, userID, row, in.Set, category)
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				if err := writeAuditTx(tx, userID, auditBulkUpdate, row.ID, in.IPAddress, changes); err != nil {
					return err
				}
			}
			result.Updated++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// applyBulkSet applies the set to one locked row and returns the audit changes.
func applyBulkSet(tx *gorm.DB, userID string, row *models.Transaction, set BulkTransactionSet, category *models.Category) (map[string]any, error) {
	changes := map[string]any{}
	updates := map[string]any{}

	if category != nil {
		if !isCategorizableType(row.Type) {
			return nil, apperrors.WithMessage(apperrors.ErrTransactionNotEditable,
				fmt.Sprintf("A %s transaction cannot have a category", row.Type))
		}
		if err := ensureCategoryMatchesType(category, row.Type); err != nil {
			return nil, err
		}
		updates["category_id"] = category.ID
		changes["category_id"] = map[string]any{"from": row.CategoryID, "to": category.ID}
	}

	if set.NotesAppend != nil {
		appended := strings.TrimSpace(*set.NotesAppend)
		newNotes := appended
		if row.Notes != "" {
			newNotes = row.Notes + "\n" + appended
		}
		if err := validateNotes(newNotes); err != nil {
			return nil, err
		}
		updates["notes"] = newNotes
		changes["notes_append"] = appended
	}

	if set.Shared != nil && !*set.Shared && row.Type == models.TransactionTypeExpense {
		var n int64
		if err := tx.Model(&models.Receivable{}).
			Where("transaction_id = ? AND user_id = ?", row.ID, userID).Count(&n).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if n > 0 {
			if err := deleteReceivablesOfExpense(tx, userID, row.ID); err != nil {
				return nil, err
			}
			updates["personal_amount"] = gorm.Expr("NULL")
			changes["shared"] = map[string]any{"from": true, "to": false}
		}
	}

	if len(updates) > 0 {
		if err := tx.Model(&models.Transaction{}).
			Where("id = ? AND user_id = ?", row.ID, userID).Updates(updates).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return changes, nil
}

// writeAuditTx writes an audit row inside an open DB transaction, so the trail
// commits or rolls back together with the change it describes.
func writeAuditTx(tx *gorm.DB, userID, action, transactionID, ip string, changes map[string]any) error {
	data, err := json.Marshal(changes)
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	userRef, _ := auditUUID(userID)
	resourceRef, _ := auditUUID(transactionID)
	entry := &models.AuditLog{
		UserID:       userRef,
		Action:       action,
		ResourceType: "transaction",
		ResourceID:   resourceRef,
		IPAddress:    ip,
		Changes:      string(data),
	}
	if err := tx.Create(entry).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}
