package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

const (
	// maxCounterparties is the maximum number of counterparties on one shared expense.
	maxCounterparties = 20
	// maxDescriptionLen is the maximum length, in runes, of a transaction description.
	maxDescriptionLen = 500
	// maxCounterpartySuggestions caps the autocomplete result.
	maxCounterpartySuggestions = 20
)

// receivableService implements ReceivableServicer. It shares helpers with
// transactionService as package-level functions (see receivable_helpers.go)
// instead of depending on it, so neither service references the other.
type receivableService struct {
	db             *gorm.DB
	accountService AccountServicer
	ruleService    RuleServicer
}

// NewReceivableService creates a new ReceivableServicer. The ruleService is
// used to auto-categorize new shared expenses (like CreateTransaction does) and
// may be nil to disable that.
func NewReceivableService(db *gorm.DB, accountService AccountServicer, ruleService RuleServicer) ReceivableServicer {
	return &receivableService{db: db, accountService: accountService, ruleService: ruleService}
}

// share is one validated counterparty share of an expense.
type share struct {
	name   string
	key    string
	amount int64
}

// invalidShares builds an INVALID_SHARES error with a specific message.
func invalidShares(msg string) error {
	return apperrors.WithMessage(apperrors.ErrInvalidShares, msg)
}

// buildCreateShares validates a shared-expense request and returns the
// counterparty shares in request order. The user's own share is always the remainder.
func buildCreateShares(in CreateSharedExpenseInput) ([]share, error) {
	n := len(in.Counterparties)
	if n < 1 || n > maxCounterparties {
		return nil, invalidShares(fmt.Sprintf("between 1 and %d counterparties are required", maxCounterparties))
	}

	shares := make([]share, 0, n)
	seen := make(map[string]bool, n)
	for _, cp := range in.Counterparties {
		name, key, err := models.NormalizeCounterpartyName(cp.Name)
		if err != nil {
			return nil, invalidShares(err.Error())
		}
		if seen[key] {
			return nil, invalidShares("duplicate counterparty: " + name)
		}
		seen[key] = true
		shares = append(shares, share{name: name, key: key})
	}

	switch in.Mode {
	case ShareModeEqual:
		for _, cp := range in.Counterparties {
			if cp.Amount != nil {
				return nil, invalidShares("amounts must be omitted for an equal split")
			}
		}
		each := in.Amount / int64(n+1)
		if each == 0 {
			return nil, invalidShares("amount is too small to split")
		}
		for i := range shares {
			shares[i].amount = each
		}
	case ShareModeCustom:
		var total int64
		for i, cp := range in.Counterparties {
			if cp.Amount == nil || *cp.Amount <= 0 {
				return nil, invalidShares("every counterparty needs an amount greater than zero")
			}
			// total <= in.Amount always holds here, so the subtraction cannot
			// overflow and the comparison rejects the entry before total wraps.
			if *cp.Amount > in.Amount-total {
				return nil, invalidShares("shares must not exceed the expense amount")
			}
			shares[i].amount = *cp.Amount
			total += *cp.Amount
		}
	case ShareModeFull:
		if n != 1 {
			return nil, invalidShares("exactly one counterparty is required for full mode")
		}
		if in.Counterparties[0].Amount != nil {
			return nil, invalidShares("amount must be omitted for full mode")
		}
		shares[0].amount = in.Amount
	default:
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "mode must be equal, custom or full")
	}
	return shares, nil
}

// CreateSharedExpense creates an expense and its receivables in one DB
// transaction: the account is locked, the expense inserted and debited by the
// full amount, the receivables inserted with their derived status, and
// personal_amount recomputed.
func (s *receivableService) CreateSharedExpense(userID string, in CreateSharedExpenseInput) (*models.Transaction, error) {
	if err := validateTransactionInput(in.AccountID, in.Amount, in.Description); err != nil {
		return nil, err
	}
	if !models.IsValidReceivableKind(in.Kind) {
		return nil, apperrors.ErrInvalidReceivableKind
	}
	shares, err := buildCreateShares(in)
	if err != nil {
		return nil, err
	}

	date := in.Date
	if date.IsZero() {
		date = time.Now()
	}

	categoryID := in.CategoryID
	if categoryID != nil {
		var n int64
		if err := s.db.Model(&models.Category{}).
			Where("id = ? AND user_id = ?", *categoryID, userID).Count(&n).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if n == 0 {
			return nil, apperrors.ErrCategoryNotFound
		}
	}

	// Auto-categorize via rules exactly like CreateTransaction, except in full
	// mode (a loan/claim/deposit is not spending, so no category is guessed).
	if in.Mode != ShareModeFull {
		categoryID = resolveRuleCategory(s.ruleService, userID, categoryID, RuleInput{
			Description: in.Description,
			Amount:      in.Amount,
			AccountID:   in.AccountID,
			Type:        models.TransactionTypeExpense,
		})
	}

	var expenseID string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		accounts, txErr := lockAccounts(tx, userID, []string{in.AccountID}, true)
		if txErr != nil {
			return txErr
		}
		expense, txErr := insertTransaction(tx, s.accountService, userID, accounts[in.AccountID],
			categoryID, models.TransactionTypeExpense, in.Amount, in.Description, date)
		if txErr != nil {
			return txErr
		}

		// Explicit, strictly increasing created_at keeps the request order stable.
		base := time.Now()
		for i, sh := range shares {
			if txErr := insertReceivable(tx, userID, expense.ID, sh.name, sh.key, in.Kind, sh.amount,
				base.Add(time.Duration(i)*time.Microsecond)); txErr != nil {
				return txErr
			}
		}

		expenseID = expense.ID
		return recomputePersonalAmount(tx, userID, expense.ID)
	})
	if err != nil {
		return nil, err
	}
	return loadExpenseWithReceivables(s.db, userID, expenseID)
}

// saveReceivable derives the status and persists the mutable columns of a receivable.
func saveReceivable(tx *gorm.DB, userID string, rec *models.Receivable) error {
	rec.Status = rec.DeriveStatus()
	if err := tx.Model(rec).
		Where("user_id = ?", userID).
		Select("CounterpartyName", "CounterpartyKey", "Kind", "Amount", "PaidAmount", "WrittenOffAmount", "Status").
		Updates(rec).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	rec.ComputeOutstanding()
	return nil
}

// UpdateReceivables applies the D7 diff to an expense's receivables under the
// D10 locks (expense, then receivables by id) and recomputes personal_amount.
func (s *receivableService) UpdateReceivables(userID, transactionID string, in UpdateReceivablesInput) (*models.Transaction, error) {
	if in.Kind != nil && !models.IsValidReceivableKind(*in.Kind) {
		return nil, apperrors.ErrInvalidReceivableKind
	}
	if in.Receivables == nil {
		in.Receivables = []ReceivableShareInput{}
	}
	if len(in.Receivables) > maxCounterparties {
		return nil, invalidShares(fmt.Sprintf("at most %d receivables are allowed", maxCounterparties))
	}

	type entry struct {
		id     string
		hasID  bool
		name   string
		key    string
		amount int64
	}
	entries := make([]entry, 0, len(in.Receivables))
	seenIDs := make(map[string]bool, len(in.Receivables))
	keyCount := make(map[string]int, len(in.Receivables))
	for _, r := range in.Receivables {
		name, key, err := models.NormalizeCounterpartyName(r.Name)
		if err != nil {
			return nil, invalidShares(err.Error())
		}
		if r.Amount <= 0 {
			return nil, invalidShares("every receivable needs an amount greater than zero")
		}
		e := entry{name: name, key: key, amount: r.Amount}
		if r.ID != nil {
			if seenIDs[*r.ID] {
				return nil, invalidShares("duplicate receivable id")
			}
			seenIDs[*r.ID] = true
			e.id, e.hasID = *r.ID, true
		}
		keyCount[key]++
		entries = append(entries, e)
	}
	// A new entry may not repeat a counterparty already on the expense. Existing
	// entries may share a key (a rename can merge counterparties).
	for _, e := range entries {
		if !e.hasID && keyCount[e.key] > 1 {
			return nil, invalidShares("duplicate counterparty: " + e.name)
		}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		expense, txErr := lockTransaction(tx, userID, transactionID)
		if txErr != nil {
			return txErr
		}
		if expense.Type != models.TransactionTypeExpense {
			return invalidShares("only expenses can have receivables")
		}
		existing, txErr := lockReceivablesForTransaction(tx, userID, expense.ID)
		if txErr != nil {
			return txErr
		}
		byID := make(map[string]*models.Receivable, len(existing))
		for i := range existing {
			byID[existing[i].ID] = &existing[i]
		}
		for _, e := range entries {
			if e.hasID && byID[e.id] == nil {
				return apperrors.ErrReceivableNotFound
			}
		}

		// New entries inherit the explicit kind, else the kind already on the expense.
		insertKind := models.ReceivableKindSplit
		if len(existing) > 0 {
			insertKind = existing[0].Kind
		}
		if in.Kind != nil {
			insertKind = *in.Kind
		}

		var total int64
		kept := make(map[string]bool, len(entries))
		now := time.Now()
		for i, e := range entries {
			// total <= expense.Amount always holds here, so this cannot overflow.
			if e.amount > expense.Amount-total {
				return invalidShares("shares must not exceed the expense amount")
			}
			total += e.amount
			if !e.hasID {
				if txErr := insertReceivable(tx, userID, expense.ID, e.name, e.key, insertKind, e.amount,
					now.Add(time.Duration(i)*time.Microsecond)); txErr != nil {
					return txErr
				}
				continue
			}

			rec := byID[e.id]
			kept[rec.ID] = true
			if e.amount < rec.PaidAmount+rec.WrittenOffAmount {
				return invalidShares("amount cannot be less than what is already paid or written off")
			}
			rec.CounterpartyName, rec.CounterpartyKey, rec.Amount = e.name, e.key, e.amount
			if in.Kind != nil {
				rec.Kind = *in.Kind
			}
			if txErr := saveReceivable(tx, userID, rec); txErr != nil {
				return txErr
			}
		}

		for i := range existing {
			rec := &existing[i]
			if kept[rec.ID] {
				continue
			}
			if rec.PaidAmount > 0 {
				return apperrors.ErrReceivableHasPayments
			}
			if txErr := tx.Where("user_id = ?", userID).Delete(rec).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}
		}

		return recomputePersonalAmount(tx, userID, expense.ID)
	})
	if err != nil {
		return nil, err
	}
	return loadExpenseWithReceivables(s.db, userID, transactionID)
}

// findReceivable reads a user's non-deleted receivable without locking (used to
// discover its expense before taking the D10 locks in order).
func (s *receivableService) findReceivable(userID, receivableID string) (*models.Receivable, error) {
	var rec models.Receivable
	if err := s.db.Where("id = ? AND user_id = ?", receivableID, userID).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrReceivableNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &rec, nil
}

// lockReceivable runs the D10 lock sequence for an operation on one receivable:
// it locks the receivable's expense, then the expense's receivables (by id), and
// returns the freshly re-read target. The receivable may have been removed since
// it was first read, in which case ErrReceivableNotFound is returned.
func lockReceivable(tx *gorm.DB, userID, transactionID, receivableID string) (*models.Receivable, error) {
	if _, err := lockTransaction(tx, userID, transactionID); err != nil {
		if errors.Is(err, apperrors.ErrTransactionNotFound) {
			return nil, apperrors.ErrReceivableNotFound
		}
		return nil, err
	}
	recs, err := lockReceivablesForTransaction(tx, userID, transactionID)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if recs[i].ID == receivableID {
			return &recs[i], nil
		}
	}
	return nil, apperrors.ErrReceivableNotFound
}

// Settle records a repayment against a receivable under the D10 locks (expense,
// receivable, account). It creates the reimbursement transaction, credits the
// account with a relative update, inserts the settlement, updates paid_amount
// and status (optionally writing off the remainder, which is recorded on the
// settlement) and recomputes personal_amount, all in one DB transaction.
func (s *receivableService) Settle(userID, receivableID string, in SettleInput) (*models.Receivable, *models.ReceivableSettlement, error) {
	if in.AccountID == "" {
		return nil, nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account ID is required")
	}
	if in.Amount <= 0 {
		return nil, nil, apperrors.ErrSettlementExceedsOutstanding
	}
	if err := validateDescription(in.Description); err != nil {
		return nil, nil, err
	}
	first, err := s.findReceivable(userID, receivableID)
	if err != nil {
		return nil, nil, err
	}

	var rec *models.Receivable
	var settlement *models.ReceivableSettlement
	err = s.db.Transaction(func(tx *gorm.DB) error {
		cur, txErr := lockReceivable(tx, userID, first.TransactionID, receivableID)
		if txErr != nil {
			return txErr
		}
		accounts, txErr := lockAccounts(tx, userID, []string{in.AccountID}, true)
		if txErr != nil {
			return txErr
		}
		account := accounts[in.AccountID]
		switch account.Type {
		case models.AccountTypeCash, models.AccountTypeCreditCard, models.AccountTypeDebt:
		default:
			return apperrors.ErrInvalidSettlementAccount
		}
		if in.Amount > cur.OutstandingAmount() {
			return apperrors.ErrSettlementExceedsOutstanding
		}

		var expense models.Transaction
		if txErr := tx.Where("id = ? AND user_id = ?", cur.TransactionID, userID).First(&expense).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		date := in.Date
		if date.IsZero() {
			date = time.Now()
		}
		description := strings.TrimSpace(in.Description)
		if description == "" {
			description = "Repayment from " + cur.CounterpartyName
			if expense.Description != "" {
				description += ": " + expense.Description
			}
		}
		description = truncateRunes(description, maxDescriptionLen)

		reimbursement := &models.Transaction{
			UserID:      userID,
			AccountID:   account.ID,
			Type:        models.TransactionTypeReimbursement,
			Amount:      in.Amount,
			Description: description,
			Date:        date,
		}
		if txErr := tx.Create(reimbursement).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := s.accountService.ApplyBalanceDelta(tx, account.ID, account.Type, models.TransactionTypeReimbursement, in.Amount); txErr != nil {
			return txErr
		}

		// The write-off is recorded on the settlement so that deleting the
		// settlement can reverse it together with the payment.
		cur.PaidAmount += in.Amount
		var writtenOff int64
		if in.WriteOffRemainder {
			writtenOff = cur.OutstandingAmount()
			cur.WrittenOffAmount += writtenOff
		}

		settlement = &models.ReceivableSettlement{
			UserID:           userID,
			ReceivableID:     cur.ID,
			TransactionID:    reimbursement.ID,
			AccountID:        account.ID,
			Amount:           in.Amount,
			WrittenOffAmount: writtenOff,
			Date:             date,
		}
		if txErr := tx.Create(settlement).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := saveReceivable(tx, userID, cur); txErr != nil {
			return txErr
		}
		if txErr := recomputePersonalAmount(tx, userID, cur.TransactionID); txErr != nil {
			return txErr
		}
		rec = cur
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := s.attachExpenses(userID, []*models.Receivable{rec}); err != nil {
		return nil, nil, err
	}
	return rec, settlement, nil
}

// DeleteSettlement reverses a repayment under the D10 locks: it soft-deletes the
// reimbursement transaction and the settlement, debits the credited account by
// the reverse type with a relative update (exactly once), reduces paid_amount,
// reverses the write-off made by the same settle call (if any) and recomputes
// the status and personal_amount.
func (s *receivableService) DeleteSettlement(userID, settlementID string) (*models.Receivable, error) {
	var first models.ReceivableSettlement
	if err := s.db.Where("id = ? AND user_id = ?", settlementID, userID).First(&first).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSettlementNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	parent, err := s.findReceivable(userID, first.ReceivableID)
	if err != nil {
		return nil, err
	}

	var rec *models.Receivable
	err = s.db.Transaction(func(tx *gorm.DB) error {
		cur, txErr := lockReceivable(tx, userID, parent.TransactionID, parent.ID)
		if txErr != nil {
			return txErr
		}

		// Re-read the settlement now that the receivable is locked: a concurrent
		// delete must not be reversed twice.
		var settlement models.ReceivableSettlement
		if txErr := tx.Where("id = ? AND user_id = ?", settlementID, userID).First(&settlement).Error; txErr != nil {
			if errors.Is(txErr, gorm.ErrRecordNotFound) {
				return apperrors.ErrSettlementNotFound
			}
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		accounts, txErr := lockAccounts(tx, userID, []string{settlement.AccountID}, false)
		if txErr != nil {
			return txErr
		}
		account := accounts[settlement.AccountID]

		var reimbursement models.Transaction
		if txErr := tx.Where("id = ? AND user_id = ?", settlement.TransactionID, userID).First(&reimbursement).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		reversal, txErr := reverseType(reimbursement.Type)
		if txErr != nil {
			return txErr
		}
		if txErr := s.accountService.ApplyBalanceDelta(tx, account.ID, account.Type, reversal, settlement.Amount); txErr != nil {
			return txErr
		}
		if txErr := tx.Delete(&reimbursement).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := tx.Delete(&settlement).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		cur.PaidAmount -= settlement.Amount
		if cur.PaidAmount < 0 {
			return apperrors.Wrap(apperrors.ErrInternalServer, fmt.Errorf("receivable %s paid_amount would become negative", cur.ID))
		}
		// Reverse the write-off made by this settlement. It cannot exceed the
		// current write-off (UndoWriteOff zeroes both), but clamp defensively.
		cur.WrittenOffAmount -= settlement.WrittenOffAmount
		if cur.WrittenOffAmount < 0 {
			cur.WrittenOffAmount = 0
		}
		if txErr := saveReceivable(tx, userID, cur); txErr != nil {
			return txErr
		}
		if txErr := recomputePersonalAmount(tx, userID, cur.TransactionID); txErr != nil {
			return txErr
		}
		rec = cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachExpenses(userID, []*models.Receivable{rec}); err != nil {
		return nil, err
	}
	return rec, nil
}

// WriteOff forgives amount of a receivable (nil means the whole outstanding
// balance) under the D10 locks and recomputes status and personal_amount. The
// forgiven money becomes the user's spending on the original expense (D6a).
func (s *receivableService) WriteOff(userID, receivableID string, amount *int64) (*models.Receivable, error) {
	return s.mutateWriteOff(userID, receivableID, func(_ *gorm.DB, rec *models.Receivable) error {
		out := rec.OutstandingAmount()
		amt := out
		if amount != nil {
			amt = *amount
		}
		if amt <= 0 || amt > out {
			return apperrors.ErrWriteOffExceedsOutstanding
		}
		rec.WrittenOffAmount += amt
		return nil
	})
}

// UndoWriteOff resets the receivable's written-off amount to zero. It fails
// with INVALID_INPUT when there is nothing to undo.
func (s *receivableService) UndoWriteOff(userID, receivableID string) (*models.Receivable, error) {
	return s.mutateWriteOff(userID, receivableID, func(tx *gorm.DB, rec *models.Receivable) error {
		if rec.WrittenOffAmount == 0 {
			return apperrors.WithMessage(apperrors.ErrInvalidInput, "nothing to undo: this receivable has no write-off")
		}
		rec.WrittenOffAmount = 0
		// The write-offs recorded on settlements are gone too; deleting one of
		// them later must not subtract them again.
		if err := tx.Model(&models.ReceivableSettlement{}).
			Where("receivable_id = ? AND user_id = ?", rec.ID, rec.UserID).
			UpdateColumn("written_off_amount", 0).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return nil
	})
}

// mutateWriteOff is the shared D10 sequence for WriteOff and UndoWriteOff.
func (s *receivableService) mutateWriteOff(userID, receivableID string, mutate func(tx *gorm.DB, rec *models.Receivable) error) (*models.Receivable, error) {
	first, err := s.findReceivable(userID, receivableID)
	if err != nil {
		return nil, err
	}
	var rec *models.Receivable
	err = s.db.Transaction(func(tx *gorm.DB) error {
		cur, txErr := lockReceivable(tx, userID, first.TransactionID, receivableID)
		if txErr != nil {
			return txErr
		}
		if txErr := mutate(tx, cur); txErr != nil {
			return txErr
		}
		if txErr := saveReceivable(tx, userID, cur); txErr != nil {
			return txErr
		}
		if txErr := recomputePersonalAmount(tx, userID, cur.TransactionID); txErr != nil {
			return txErr
		}
		rec = cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.attachExpenses(userID, []*models.Receivable{rec}); err != nil {
		return nil, err
	}
	return rec, nil
}

// attachExpenses computes Outstanding and fills the trimmed Transaction view of
// each receivable's expense with one query.
func (s *receivableService) attachExpenses(userID string, recs []*models.Receivable) error {
	if len(recs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		r.ComputeOutstanding()
		ids = append(ids, r.TransactionID)
	}
	var txns []models.Transaction
	if err := s.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&txns).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	views := make(map[string]*models.ReceivableExpense, len(txns))
	for i := range txns {
		t := &txns[i]
		views[t.ID] = &models.ReceivableExpense{
			ID:             t.ID,
			Description:    t.Description,
			Date:           t.Date,
			Amount:         t.Amount,
			PersonalAmount: t.PersonalAmount,
			AccountID:      t.AccountID,
			CategoryID:     t.CategoryID,
		}
	}
	for _, r := range recs {
		r.Transaction = views[r.TransactionID]
	}
	return nil
}

// ListReceivables returns a page of the user's receivables ordered by expense
// date (newest first), excluding receivables whose expense was deleted.
func (s *receivableService) ListReceivables(userID string, page pagination.PageRequest, filter ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
	page.Defaults()

	q := s.db.Model(&models.Receivable{}).
		Joins("JOIN transactions ON transactions.id = receivables.transaction_id AND transactions.deleted_at IS NULL").
		Where("receivables.user_id = ?", userID)

	switch st := filter.Status; {
	case st == "":
	case st == ReceivableStatusOpen:
		q = q.Where("receivables.status IN ?", []models.ReceivableStatus{models.ReceivableStatusPending, models.ReceivableStatusPartiallyPaid})
	case models.IsValidReceivableStatus(st):
		q = q.Where("receivables.status = ?", st)
	default:
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid status")
	}
	if filter.Kind != "" {
		if !models.IsValidReceivableKind(filter.Kind) {
			return nil, apperrors.ErrInvalidReceivableKind
		}
		q = q.Where("receivables.kind = ?", filter.Kind)
	}
	if filter.CounterpartyKey != "" {
		key, err := normalizeSearchKey(filter.CounterpartyKey)
		if err != nil {
			return nil, err
		}
		q = q.Where("receivables.counterparty_key = ?", key)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var recs []models.Receivable
	if err := q.Select("receivables.*").
		Scopes(pagination.Paginate(page)).
		Order("transactions.date DESC, receivables.created_at DESC, receivables.id DESC").
		Find(&recs).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	ptrs := make([]*models.Receivable, len(recs))
	for i := range recs {
		ptrs[i] = &recs[i]
	}
	if err := s.attachExpenses(userID, ptrs); err != nil {
		return nil, err
	}

	result := pagination.NewPageResponse(recs, page.Page, page.PageSize, total)
	return &result, nil
}

// GetReceivable returns one receivable with its settlements (oldest first) and expense.
func (s *receivableService) GetReceivable(userID, receivableID string) (*models.Receivable, error) {
	var rec models.Receivable
	if err := s.db.
		Preload("Settlements", func(db *gorm.DB) *gorm.DB {
			return db.Where("user_id = ?", userID).Order("date ASC, created_at ASC, id ASC")
		}).
		Where("id = ? AND user_id = ?", receivableID, userID).
		First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrReceivableNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if err := s.attachExpenses(userID, []*models.Receivable{&rec}); err != nil {
		return nil, err
	}
	return &rec, nil
}

// Summary aggregates the user's open receivables (outstanding > 0, with both the
// receivable and its expense non-deleted) per kind and per counterparty. The
// counterparty name is the one on its most recently created receivable;
// counterparties are sorted by outstanding amount, largest first.
func (s *receivableService) Summary(userID string) (*ReceivableSummary, error) {
	type row struct {
		ID               string
		Kind             models.ReceivableKind
		CounterpartyKey  string
		CounterpartyName string
		CreatedAt        time.Time
		Outstanding      int64
		ExpenseDate      time.Time
	}
	var rows []row
	if err := s.db.Model(&models.Receivable{}).
		Select("receivables.id, receivables.kind, receivables.counterparty_key, receivables.counterparty_name, "+
			"receivables.created_at, receivables.amount - receivables.paid_amount - receivables.written_off_amount AS outstanding, "+
			"transactions.date AS expense_date").
		Joins("JOIN transactions ON transactions.id = receivables.transaction_id AND transactions.deleted_at IS NULL").
		Where("receivables.user_id = ? AND receivables.amount - receivables.paid_amount - receivables.written_off_amount > 0", userID).
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	summary := &ReceivableSummary{
		ByKind:         []ReceivableKindTotal{},
		ByCounterparty: []ReceivableCounterpartyTotal{},
	}
	kinds := make(map[models.ReceivableKind]*ReceivableKindTotal)
	cps := make(map[string]*ReceivableCounterpartyTotal)
	latest := make(map[string]row)
	for _, r := range rows {
		summary.TotalOutstanding += r.Outstanding

		k := kinds[r.Kind]
		if k == nil {
			k = &ReceivableKindTotal{Kind: r.Kind}
			kinds[r.Kind] = k
		}
		k.Outstanding += r.Outstanding
		k.OpenCount++

		c := cps[r.CounterpartyKey]
		if c == nil {
			c = &ReceivableCounterpartyTotal{CounterpartyKey: r.CounterpartyKey, OldestDate: r.ExpenseDate}
			cps[r.CounterpartyKey] = c
		}
		c.Outstanding += r.Outstanding
		c.OpenCount++
		if r.ExpenseDate.Before(c.OldestDate) {
			c.OldestDate = r.ExpenseDate
		}
		if prev, ok := latest[r.CounterpartyKey]; !ok || r.CreatedAt.After(prev.CreatedAt) ||
			(r.CreatedAt.Equal(prev.CreatedAt) && r.ID > prev.ID) {
			latest[r.CounterpartyKey] = r
		}
	}

	for _, k := range kinds {
		summary.ByKind = append(summary.ByKind, *k)
	}
	sort.Slice(summary.ByKind, func(i, j int) bool {
		a, b := summary.ByKind[i], summary.ByKind[j]
		if a.Outstanding != b.Outstanding {
			return a.Outstanding > b.Outstanding
		}
		return a.Kind < b.Kind
	})
	for key, c := range cps {
		c.CounterpartyName = latest[key].CounterpartyName
		summary.ByCounterparty = append(summary.ByCounterparty, *c)
	}
	sort.Slice(summary.ByCounterparty, func(i, j int) bool {
		a, b := summary.ByCounterparty[i], summary.ByCounterparty[j]
		if a.Outstanding != b.Outstanding {
			return a.Outstanding > b.Outstanding
		}
		return a.CounterpartyKey < b.CounterpartyKey
	})
	return summary, nil
}

// likeEscaper escapes LIKE wildcards in user-supplied autocomplete text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ListCounterparties returns up to 20 distinct counterparty keys containing the
// lowercased q (all when q is empty), most recently used first, each with the
// display name of its newest receivable.
func (s *receivableService) ListCounterparties(userID, q string) ([]CounterpartySuggestion, error) {
	needle, err := normalizeSearchKey(q)
	if err != nil {
		return nil, err
	}
	query := s.db.Model(&models.Receivable{}).Where("user_id = ?", userID)
	if needle != "" {
		query = query.Where(`counterparty_key LIKE ? ESCAPE '\'`, "%"+likeEscaper.Replace(needle)+"%")
	}

	var keys []string
	if err := query.Group("counterparty_key").
		Order("MAX(created_at) DESC, counterparty_key ASC").
		Limit(maxCounterpartySuggestions).
		Pluck("counterparty_key", &keys).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	out := make([]CounterpartySuggestion, 0, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	var recs []models.Receivable
	if err := s.db.Select("counterparty_key", "counterparty_name").
		Where("user_id = ? AND counterparty_key IN ?", userID, keys).
		Order("created_at DESC, id DESC").
		Find(&recs).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	names := make(map[string]string, len(keys))
	for i := range recs {
		if _, ok := names[recs[i].CounterpartyKey]; !ok {
			names[recs[i].CounterpartyKey] = recs[i].CounterpartyName
		}
	}
	for _, k := range keys {
		out = append(out, CounterpartySuggestion{Key: k, Name: names[k]})
	}
	return out, nil
}

// RenameCounterparty sets the name (and derived key) of every one of the user's
// receivables whose key is fromKey with a single UPDATE (atomic by itself) and
// returns how many were updated. Renaming onto an existing key merges the two
// counterparties.
func (s *receivableService) RenameCounterparty(userID, fromKey, toName string) (int64, error) {
	from, err := normalizeSearchKey(fromKey)
	if err != nil {
		return 0, err
	}
	if from == "" {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_key is required")
	}
	name, key, err := models.NormalizeCounterpartyName(toName)
	if err != nil {
		return 0, invalidShares(err.Error())
	}

	res := s.db.Model(&models.Receivable{}).
		Where("user_id = ? AND counterparty_key = ?", userID, from).
		Updates(map[string]interface{}{"counterparty_name": name, "counterparty_key": key})
	if res.Error != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	return res.RowsAffected, nil
}
