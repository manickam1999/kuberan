package services

import (
	"errors"
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

type transferFXEnv struct {
	svc      TransactionServicer
	acctSvc  AccountServicer
	userID   string
	usd, myr *models.Account
	eur      *models.Account
}

func newTransferFXEnv(t *testing.T) (*transferFXEnv, func(id string) int64, TransactionServicer) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	fxs := NewFXService(db, 24*time.Hour)
	acctSvc := NewAccountService(db, WithFXService(fxs))
	svc := NewTransactionService(db, acctSvc, nil, WithTransactionFXService(fxs))
	user := testutil.CreateTestUser(t, db)

	rate, err := fx.ParseRate("4.4712")
	testutil.AssertNoError(t, err)
	testutil.AssertNoError(t, db.Create(&models.FXRate{
		Base: "USD", Quote: "MYR", Rate: rate, RecordedAt: time.Now().Add(-time.Hour), Source: "test",
	}).Error)

	usd := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 200000)
	myr := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
	eur := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
	testutil.AssertNoError(t, db.Model(&models.Account{}).Where("id = ?", myr.ID).Update("currency", "MYR").Error)
	testutil.AssertNoError(t, db.Model(&models.Account{}).Where("id = ?", eur.ID).Update("currency", "USD").Error)
	balance := func(id string) int64 {
		var a models.Account
		testutil.AssertNoError(t, db.First(&a, "id = ?", id).Error)
		return a.Balance
	}
	return &transferFXEnv{svc: svc, acctSvc: acctSvc, userID: user.ID, usd: usd, myr: myr, eur: eur}, balance, svc
}

func ptr64(v int64) *int64 { return &v }

func TestCreateTransfer_SameCurrencyToAmount(t *testing.T) {
	e, balance, svc := newTransferFXEnv(t)

	t.Run("omitted stores nothing", func(t *testing.T) {
		tx, err := svc.CreateTransfer(e.userID, e.usd.ID, e.eur.ID, 1000, nil, "same", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)
		if tx.ToAmount != nil || tx.ImpliedRate != "" {
			t.Errorf("same-currency transfer stored to_amount=%v implied=%q", tx.ToAmount, tx.ImpliedRate)
		}
	})
	t.Run("equal is accepted and not stored", func(t *testing.T) {
		tx, err := svc.CreateTransfer(e.userID, e.usd.ID, e.eur.ID, 1000, ptr64(1000), "same", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)
		if tx.ToAmount != nil {
			t.Errorf("to_amount = %d, want NULL", *tx.ToAmount)
		}
	})
	t.Run("unequal is rejected", func(t *testing.T) {
		before := balance(e.usd.ID)
		_, err := svc.CreateTransfer(e.userID, e.usd.ID, e.eur.ID, 1000, ptr64(999), "bad", time.Now(), TransactionMeta{})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
		if balance(e.usd.ID) != before {
			t.Error("a rejected transfer moved money")
		}
	})
	if balance(e.usd.ID) != 198000 || balance(e.eur.ID) != 2000 {
		t.Errorf("balances = %d / %d, want 198000 / 2000", balance(e.usd.ID), balance(e.eur.ID))
	}
}

func TestCreateTransfer_CrossCurrency(t *testing.T) {
	e, balance, svc := newTransferFXEnv(t)

	t.Run("missing to_amount returns the suggestion", func(t *testing.T) {
		_, err := svc.CreateTransfer(e.userID, e.myr.ID, e.usd.ID, 894240, nil, "fund", time.Now(), TransactionMeta{})
		testutil.AssertAppError(t, err, apperrors.ErrFXAmountRequired.Code)
		var ae *apperrors.AppError
		if !errors.As(err, &ae) {
			t.Fatalf("not an AppError: %v", err)
		}
		if ae.Details["currency"] != "USD" || ae.Details["field"] != "to_amount" {
			t.Errorf("details = %v", ae.Details)
		}
		if got, ok := ae.Details["suggested_amount"].(int64); !ok || got < 199999 || got > 200001 {
			t.Errorf("suggested_amount = %v", ae.Details["suggested_amount"])
		}
		if balance(e.myr.ID) != 1000000 || balance(e.usd.ID) != 200000 {
			t.Error("a refused transfer moved money")
		}
	})

	t.Run("given to_amount moves both balances", func(t *testing.T) {
		tx, err := svc.CreateTransfer(e.userID, e.myr.ID, e.usd.ID, 894240, ptr64(200000), "fund", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)
		if tx.ToAmount == nil || *tx.ToAmount != 200000 || tx.ImpliedRate != "0.2236536053" {
			t.Errorf("transfer = to_amount %v implied %q", tx.ToAmount, tx.ImpliedRate)
		}
		if balance(e.myr.ID) != 105760 || balance(e.usd.ID) != 400000 {
			t.Errorf("balances = %d / %d, want 105760 / 400000", balance(e.myr.ID), balance(e.usd.ID))
		}

		got, err := svc.GetTransactionByID(e.userID, tx.ID)
		testutil.AssertNoError(t, err)
		if got.ImpliedRate != "0.2236536053" {
			t.Errorf("loaded implied_rate = %q", got.ImpliedRate)
		}

		// Delete restores both balances exactly.
		testutil.AssertNoError(t, svc.DeleteTransaction(e.userID, tx.ID))
		if balance(e.myr.ID) != 1000000 || balance(e.usd.ID) != 200000 {
			t.Errorf("after delete balances = %d / %d, want 1000000 / 200000", balance(e.myr.ID), balance(e.usd.ID))
		}
	})

	t.Run("non-positive to_amount is rejected", func(t *testing.T) {
		_, err := svc.CreateTransfer(e.userID, e.myr.ID, e.usd.ID, 100, ptr64(0), "x", time.Now(), TransactionMeta{})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})

	t.Run("a transfer cannot be edited, so to_amount cannot drift", func(t *testing.T) {
		tx, err := svc.CreateTransfer(e.userID, e.myr.ID, e.usd.ID, 894240, ptr64(200000), "fund", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)
		amount := int64(1)
		_, err = svc.UpdateTransaction(e.userID, tx.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertAppError(t, err, apperrors.ErrTransactionNotEditable.Code)
	})
}
