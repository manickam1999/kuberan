package services

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// write applies a validated plan inside the caller's DB transaction.
func (s *importService) write(tx *gorm.DB, pl *importPlan) error {
	if err := s.writeHoldings(tx, pl); err != nil {
		return err
	}
	if err := s.writeCash(tx, pl); err != nil {
		return err
	}
	if pl.legacy != nil {
		var bal int64
		if err := tx.Model(&models.Account{}).Where("id = ?", pl.legacy.ID).Select("balance").Scan(&bal).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		pl.legacyAfter = &bal
		if pl.params.DeactivateLegacyCashAccount {
			if err := tx.Model(&models.Account{}).Where("id = ?", pl.legacy.ID).Update("is_active", false).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
	}
	return nil
}

// writeHoldings replaces each symbol's investment transactions and rebuilds the
// holding from them.
func (s *importService) writeHoldings(tx *gorm.DB, pl *importPlan) error {
	stamp := time.Now()
	for _, sp := range pl.symbols {
		if sp.inv == nil && len(sp.rows) == 0 {
			continue // flat and never held here: nothing to create
		}
		if sp.inv == nil {
			inv := &models.Investment{AccountID: pl.invAcct.ID, SecurityID: sp.sec.ID}
			if err := tx.Create(inv).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			sp.inv = inv
		} else if _, err := lockInvestment(tx, sp.inv.ID); err != nil {
			return err
		}
		for lo := 0; lo < len(sp.replaceIDs); lo += 500 {
			hi := min(lo+500, len(sp.replaceIDs))
			if err := tx.Where("investment_id = ? AND id IN ?", sp.inv.ID, sp.replaceIDs[lo:hi]).
				Delete(&models.InvestmentTransaction{}).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		for i := range sp.rows {
			sp.rows[i].InvestmentID = sp.inv.ID
			// Strictly increasing created_at keeps same-instant events in
			// statement order when the holding is rebuilt (date, created_at, id).
			sp.rows[i].CreatedAt = stamp.Add(time.Duration(i) * time.Millisecond)
		}
		if len(sp.rows) > 0 {
			if err := tx.CreateInBatches(&sp.rows, 100).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		if err := rebuildInvestment(tx, sp.inv.ID); err != nil {
			return err
		}
		var got models.Investment
		if err := tx.Where("id = ?", sp.inv.ID).First(&got).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if math.Abs(got.Quantity-sp.final.Quantity) > quantityEpsilon || got.CostBasis != sp.final.CostBasis {
			return apperrors.Wrap(apperrors.ErrInternalServer,
				fmt.Errorf("rebuilt holding for %s differs from the planned replay", sp.symbol))
		}
	}
	return nil
}

// reversalFor returns the balance effect that undoes a row. For expense and
// income rows the current type decides (it is what the balance reflects); the
// import_ref direction is used only for investment rows, whose type does not
// encode it (never the editable description). A
// hand-entered opening balance (no import_ref) is reversed by its type.
func reversalFor(t *models.Transaction) (models.TransactionType, bool) {
	if t.ImportRef == nil {
		if t.Type == models.TransactionTypeOpeningBalance {
			return openingBalanceReversal, true
		}
		return "", false
	}
	credit, ok := importRefCredit(t.ImportRef)
	switch {
	case !ok:
		return "", false
	case t.Type == models.TransactionTypeOpeningBalance:
		return openingBalanceReversal, true
	case t.Type == models.TransactionTypeExpense:
		// The row's current type is authoritative; the ref direction only
		// applies where the type does not encode it.
		return models.TransactionTypeIncome, true
	case t.Type == models.TransactionTypeIncome:
		return models.TransactionTypeExpense, true
	case credit:
		return models.TransactionTypeExpense, true
	default:
		return models.TransactionTypeIncome, true
	}
}

// writeCash replaces the cash ledger: it removes the previous import's rows and
// the cash account's opening balance (reversing their balance effects), then
// writes the opening balance and the cash rows, and re-points the matched
// transfers.
func (s *importService) writeCash(tx *gorm.DB, pl *importPlan) error {
	prefix := importRefPrefix(pl.set.Account)
	ids := []string{pl.cash.ID}
	if pl.legacy != nil {
		ids = append(ids, pl.legacy.ID)
	}
	if _, err := lockAccounts(tx, pl.userID, ids, false); err != nil {
		return err
	}

	var old []models.Transaction
	if err := tx.Where("user_id = ? AND type <> ? AND import_ref LIKE ?", pl.userID, models.TransactionTypeTransfer, prefix+"%").
		Find(&old).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	var openings []models.Transaction
	if err := tx.Where("user_id = ? AND account_id = ? AND type = ?", pl.userID, pl.cash.ID, models.TransactionTypeOpeningBalance).
		Find(&openings).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	seen := map[string]bool{}
	var remove []models.Transaction
	all := make([]models.Transaction, 0, len(old)+len(openings))
	all = append(append(all, old...), openings...)
	for i := range all {
		if !seen[all[i].ID] {
			seen[all[i].ID] = true
			remove = append(remove, all[i])
		}
	}
	if len(remove) > 0 {
		acctIDs := make([]string, 0, len(remove))
		for i := range remove {
			acctIDs = append(acctIDs, remove[i].AccountID)
		}
		accts, err := lockAccounts(tx, pl.userID, acctIDs, false)
		if err != nil {
			return err
		}
		delIDs := make([]string, 0, len(remove))
		for i := range remove {
			t := &remove[i]
			rev, ok := reversalFor(t)
			if !ok {
				return apperrors.Wrap(apperrors.ErrInternalServer, fmt.Errorf("cannot reverse import row %s", t.ID))
			}
			a := accts[t.AccountID]
			if err := s.accountService.ApplyBalanceDelta(tx, a.ID, a.Type, rev, t.Amount); err != nil {
				return err
			}
			delIDs = append(delIDs, t.ID)
		}
		if err := tx.Where("id IN ?", delIDs).Delete(&models.Transaction{}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	start := pl.params.CashStartDate.UTC()
	rows := make([]cashRow, 0, len(pl.cashRows)+1)
	if pl.opening > 0 {
		rows = append(rows, cashRow{
			ref: prefix + "opening" + importDirSuffix(models.TransactionTypeOpeningBalance), typ: models.TransactionTypeOpeningBalance, effect: models.TransactionTypeOpeningBalance,
			amount: pl.opening, desc: importDescPrefix + "Opening balance", date: start,
		})
	}
	rows = append(rows, pl.cashRows...)
	for i := range rows {
		r := rows[i]
		ref := r.ref
		t := models.Transaction{
			UserID: pl.userID, AccountID: pl.cash.ID, Type: r.typ, Amount: r.amount,
			Description: r.desc, Date: r.date, ImportRef: &ref, Source: models.TransactionSourceImport,
		}
		if err := tx.Create(&t).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if err := s.accountService.ApplyBalanceDelta(tx, pl.cash.ID, pl.cash.Type, r.effect, r.amount); err != nil {
			return err
		}
	}

	for i := range pl.matches {
		m := &pl.matches[i]
		ref := prefix + m.ev.Key
		if !m.changed {
			continue
		}
		if err := s.repoint(tx, pl, m, ref); err != nil {
			return err
		}
	}
	return nil
}

// repoint moves a transfer's destination to the new cash account with the USD
// amount as its to_amount: the old destination's credit is reversed and the new
// one applied through the same balance helper every other writer uses.
func (s *importService) repoint(tx *gorm.DB, pl *importPlan, m *depositMatch, ref string) error {
	var tr models.Transaction
	if err := lockUpdate(tx).Where("id = ? AND user_id = ?", m.tr.ID, pl.userID).First(&tr).Error; err != nil {
		return apperrors.ErrTransactionNotFound
	}
	if tr.ToAccountID == nil {
		return apperrors.ErrInvalidTransactionType
	}
	accts, err := lockAccounts(tx, pl.userID, []string{*tr.ToAccountID, pl.cash.ID, tr.AccountID}, false)
	if err != nil {
		return err
	}
	oldTo := accts[*tr.ToAccountID]
	if err := s.accountService.ApplyBalanceDelta(tx, oldTo.ID, oldTo.Type, models.TransactionTypeExpense, tr.EffectiveToAmount()); err != nil {
		return err
	}
	if err := s.accountService.ApplyBalanceDelta(tx, pl.cash.ID, pl.cash.Type, models.TransactionTypeIncome, m.toAmount); err != nil {
		return err
	}
	var toAmount any = m.toAmount
	if strings.EqualFold(accts[tr.AccountID].Currency, pl.cash.Currency) && m.toAmount == tr.Amount {
		toAmount = nil // same currency, same amount: nothing to store
	}
	if err := tx.Model(&models.Transaction{}).Where("id = ?", tr.ID).Updates(map[string]any{
		"to_account_id": pl.cash.ID, "to_amount": toAmount, "import_ref": ref,
	}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}
