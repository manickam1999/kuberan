package services

import (
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// The SQLite test schema comes from AutoMigrate, so the SQL CHECKs of migration
// 000030 only exist there because the models carry `check:` tags. These tests
// pin that mirror: a violation must fail in the test database too.
func TestModelCheckConstraintsMirrorMigration(t *testing.T) {
	e := newRecEnv(t)
	x := e.split(1000, "Alice")
	rec := x.Receivables[0]
	var settlementTxnID string
	{
		_, s := e.mustSettle(rec.ID, 100)
		settlementTxnID = s.TransactionID
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{"personal_amount above amount", func() error {
			return e.db.Model(&models.Transaction{}).Where("id = ?", x.ID).UpdateColumn("personal_amount", 1001).Error
		}},
		{"negative personal_amount", func() error {
			return e.db.Model(&models.Transaction{}).Where("id = ?", x.ID).UpdateColumn("personal_amount", -1).Error
		}},
		{"amount lowered below personal_amount", func() error {
			return e.db.Model(&models.Transaction{}).Where("id = ?", x.ID).UpdateColumn("amount", 100).Error
		}},
		{"receivable amount zero", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("amount", 0).Error
		}},
		{"receivable negative paid_amount", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("paid_amount", -1).Error
		}},
		{"receivable negative written_off_amount", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("written_off_amount", -1).Error
		}},
		{"receivable paid plus written off above amount", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("written_off_amount", rec.Amount).Error
		}},
		{"receivable unknown kind", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("kind", "gift").Error
		}},
		{"receivable unknown status", func() error {
			return e.db.Model(&models.Receivable{}).Where("id = ?", rec.ID).UpdateColumn("status", "lost").Error
		}},
		{"settlement amount zero", func() error {
			return e.db.Model(&models.ReceivableSettlement{}).Where("transaction_id = ?", settlementTxnID).UpdateColumn("amount", 0).Error
		}},
		{"settlement negative written_off_amount", func() error {
			return e.db.Model(&models.ReceivableSettlement{}).Where("transaction_id = ?", settlementTxnID).UpdateColumn("written_off_amount", -5).Error
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Error("expected a CHECK constraint violation, got none")
			}
		})
	}
}

// Regression: lowering the amount of a shared expense wrote the new amount next
// to the old personal_amount, violating transactions_personal_amount_check on
// PostgreSQL (a 500). The CHECK now exists in the SQLite schema too.
func TestUpdateTransactionLowersSharedExpenseAmount(t *testing.T) {
	t.Run("custom split of 1000 with Alice owing 100, amount lowered to 500", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 100)))
		e.wantPersonal(x.ID, 900)

		amount := int64(500)
		got, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertNoError(t, err)
		if got.Amount != 500 || got.PersonalAmount == nil || *got.PersonalAmount != 400 {
			t.Errorf("result = amount %d personal %v, want 500 / 400", got.Amount, got.PersonalAmount)
		}
		e.wantPersonal(x.ID, 400)
		e.wantBalance(e.cash.ID, recCashStart-500)
	})

	t.Run("raising the amount keeps the owed shares", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 100)))
		amount := int64(3000)
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 2900)
		e.wantBalance(e.cash.ID, recCashStart-3000)
	})

	t.Run("lowering to exactly what is owed leaves a zero personal share", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 100)))
		amount := int64(100)
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 0)
	})

	t.Run("lowering below what is owed is refused and changes nothing", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 100)))
		amount := int64(99)
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		e.wantPersonal(x.ID, 900)
		e.wantBalance(e.cash.ID, recCashStart-1000)
	})

	t.Run("write-offs are honoured when the amount changes", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 300)))
		_, err := e.writeOff(x.Receivables[0].ID, i64(100)) // owes 200 net, personal 800
		testutil.AssertNoError(t, err)
		amount := int64(320) // must stay >= the gross 300 owed
		_, err = e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 120)
	})
}

// Bug 7: editing an expense whose account is deactivated is refused with
// ACCOUNT_NOT_FOUND, exactly as before plan 021 (UpdateTransaction used
// GetAccountByID, which only returns active accounts).
func TestUpdateAndDeleteOnDeactivatedAccountMatchMain(t *testing.T) {
	e := newRecEnv(t)
	x, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 300, "coffee", time.Now())
	testutil.AssertNoError(t, err)
	testutil.AssertNoError(t, e.db.Model(e.cash).Update("is_active", false).Error)

	desc := "latte"
	_, err = e.updateTxn(x.ID, TransactionUpdateFields{Description: &desc})
	assertAppCode(t, err, "ACCOUNT_NOT_FOUND", http.StatusNotFound)
	assertAppCode(t, e.deleteTxn(x.ID), "ACCOUNT_NOT_FOUND", http.StatusNotFound)
	_, err = e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 100, "x", time.Now())
	assertAppCode(t, err, "ACCOUNT_NOT_FOUND", http.StatusNotFound)
}

func TestShareSumsCannotOverflow(t *testing.T) {
	const half = math.MaxInt64/2 + 1 // two of these wrap an int64

	t.Run("create custom", func(t *testing.T) {
		e := newRecEnv(t)
		for _, shares := range [][]SharedCounterpartyInput{
			{rcpAmt("A", half), rcpAmt("B", half)},
			{rcpAmt("A", math.MaxInt64), rcpAmt("B", math.MaxInt64)},
			{rcpAmt("A", math.MaxInt64), rcpAmt("B", 1)},
		} {
			in := e.in(math.MaxInt64, ShareModeCustom, models.ReceivableKindSplit, shares...)
			_, err := e.create(in)
			assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		}
		// A single share of exactly the full amount is fine.
		x := e.mustCreate(e.in(math.MaxInt64, ShareModeCustom, models.ReceivableKindLoan, rcpAmt("A", math.MaxInt64)))
		e.wantPersonal(x.ID, 0)
	})

	t.Run("update diff", func(t *testing.T) {
		e := newRecEnv(t)
		x, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, math.MaxInt64, "huge", time.Now())
		testutil.AssertNoError(t, err)
		_, err = e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			{Name: "A", Amount: half}, {Name: "B", Amount: half},
		}})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		_, err = e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			{Name: "A", Amount: math.MaxInt64}, {Name: "B", Amount: math.MaxInt64}, {Name: "C", Amount: 10},
		}})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		if got := e.personal(x.ID); got != nil {
			t.Errorf("personal_amount = %d, want NULL (nothing written)", *got)
		}
	})
}

func TestControlCharactersAreRejected(t *testing.T) {
	t.Run("counterparty names", func(t *testing.T) {
		e := newRecEnv(t)
		for _, name := range []string{"Al\x00ice", "Alice\x07", "A\u0000", "bad\xffutf8"} {
			_, err := e.create(e.in(900, ShareModeEqual, models.ReceivableKindSplit, rcpEqual(name)...))
			assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		}
		x := e.split(900, "Alice")
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{{ID: &x.Receivables[0].ID, Name: "Al\x00", Amount: 450}}})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		_, err = e.rec.RenameCounterparty(e.user.ID, "alice", "Al\x00ice")
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)

		// Tabs and newlines are whitespace and are collapsed, not rejected.
		y := e.mustCreate(e.in(900, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Bob\tJones\n")...))
		if y.Receivables[0].CounterpartyName != "Bob Jones" {
			t.Errorf("name = %q, want %q", y.Receivables[0].CounterpartyName, "Bob Jones")
		}
	})

	t.Run("search filters", func(t *testing.T) {
		e := newRecEnv(t)
		e.split(900, "Alice")
		_, err := e.rec.ListCounterparties(e.user.ID, "al\x00")
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		_, err = e.rec.ListReceivables(e.user.ID, pagination.PageRequest{Page: 1, PageSize: 10}, ReceivableFilter{CounterpartyKey: "al\x00"})
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		_, err = e.rec.RenameCounterparty(e.user.ID, "al\x00", "Alicia")
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		res, err := e.rec.ListCounterparties(e.user.ID, "\tAL ")
		testutil.AssertNoError(t, err)
		if len(res) != 1 {
			t.Errorf("whitespace-padded query should still match, got %d", len(res))
		}
	})

	t.Run("transaction descriptions", func(t *testing.T) {
		e := newRecEnv(t)
		bad := "co\x00ffee"
		_, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 100, bad, time.Now())
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		in := e.in(900, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alice")...)
		in.Description = bad
		_, err = e.create(in)
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)

		to := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 0)
		_, err = e.txn.CreateTransfer(e.user.ID, e.cash.ID, to.ID, 100, bad, time.Now())
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)

		ok, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 100, "line one\n\tline two", time.Now())
		testutil.AssertNoError(t, err)
		_, err = e.updateTxn(ok.ID, TransactionUpdateFields{Description: &bad})
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)

		x := e.split(900, "Alice")
		_, _, err = e.rec.Settle(e.user.ID, x.Receivables[0].ID, SettleInput{AccountID: e.cash.ID, Amount: 100, Description: bad})
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		e.check()
		if strings.Contains(e.mustDescription(ok.ID), "\x00") {
			t.Error("a rejected update must not be persisted")
		}
	})
}

// mustDescription reloads a transaction's description.
func (e *recEnv) mustDescription(id string) string {
	e.t.Helper()
	var x models.Transaction
	if err := e.db.First(&x, "id = ?", id).Error; err != nil {
		e.t.Fatalf("load txn: %v", err)
	}
	return x.Description
}
