package services

import (
	"net/http"
	"reflect"
	"sort"
	"testing"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestRecomputePersonalAmount(t *testing.T) {
	t.Run("sets amount minus owed net of write-offs", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 300), rcpAmt("Bob", 200)))
		e.wantPersonal(x.ID, 500)

		// Write-offs move money back to the user.
		_, err := e.writeOff(x.Receivables[0].ID, i64(100))
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 600)

		// Calling the helper directly is idempotent.
		testutil.AssertNoError(t, recomputePersonalAmount(e.db, e.user.ID, x.ID))
		e.wantPersonal(x.ID, 600)
	})

	t.Run("is NULL again once the last receivable is gone", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(900, "Alice")
		if e.personal(x.ID) == nil {
			t.Fatal("expected a personal amount on a shared expense")
		}
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{}})
		testutil.AssertNoError(t, err)
		if got := e.personal(x.ID); got != nil {
			t.Errorf("personal_amount = %d, want NULL", *got)
		}
	})

	t.Run("a zero personal share is valid", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(5000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
		e.wantPersonal(x.ID, 0)
	})

	t.Run("rejects shares above the amount and leaves the column untouched", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(900, "Alice") // personal 450
		// Corrupt the data behind the service's back: a share larger than the expense.
		testutil.AssertNoError(t, e.db.Model(&models.Receivable{}).
			Where("transaction_id = ?", x.ID).UpdateColumn("amount", 5000).Error)
		err := recomputePersonalAmount(e.db, e.user.ID, x.ID)
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		e.wantPersonal(x.ID, 450)
	})

	t.Run("is user scoped and reports a missing transaction", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(900, "Alice")
		other, _, _ := e.otherUser()
		err := recomputePersonalAmount(e.db, other.ID, x.ID)
		assertAppCode(t, err, "TRANSACTION_NOT_FOUND", http.StatusNotFound)
		err = recomputePersonalAmount(e.db, e.user.ID, "00000000-0000-0000-0000-00000000dead")
		assertAppCode(t, err, "TRANSACTION_NOT_FOUND", http.StatusNotFound)
	})
}

func TestPersonalAmountFor(t *testing.T) {
	e := newRecEnv(t)
	x := e.mustCreate(e.in(1000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alice", 300)))

	// The helper evaluates a hypothetical amount without writing anything.
	got, err := personalAmountFor(e.db, e.user.ID, x.ID, 500)
	testutil.AssertNoError(t, err)
	if got == nil || *got != 200 {
		t.Errorf("personalAmountFor(500) = %v, want 200", got)
	}
	_, err = personalAmountFor(e.db, e.user.ID, x.ID, 299)
	assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
	e.wantPersonal(x.ID, 700)

	none, err := personalAmountFor(e.db, e.user.ID, "00000000-0000-0000-0000-00000000dead", 500)
	testutil.AssertNoError(t, err)
	if none != nil {
		t.Errorf("no receivables must give nil, got %d", *none)
	}
}

func TestLockAccounts(t *testing.T) {
	t.Run("returns each account once by id", func(t *testing.T) {
		e := newRecEnv(t)
		b := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 5)
		var got map[string]*models.Account
		err := e.db.Transaction(func(tx *gorm.DB) error {
			var lerr error
			got, lerr = lockAccounts(tx, e.user.ID, []string{e.cash.ID, b.ID, e.cash.ID}, true)
			return lerr
		})
		testutil.AssertNoError(t, err)
		if len(got) != 2 || got[e.cash.ID].Balance != recCashStart || got[b.ID].Balance != 5 {
			t.Errorf("unexpected result: %+v", got)
		}
	})

	t.Run("locks in ascending id order regardless of request order", func(t *testing.T) {
		e := newRecEnv(t)
		b := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 5)
		c := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 5)

		var queried []string
		const name = "test:record_account_lookups"
		testutil.AssertNoError(t, e.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table != "accounts" {
				return
			}
			for _, v := range tx.Statement.Vars {
				if s, ok := v.(string); ok && (s == e.cash.ID || s == b.ID || s == c.ID) {
					queried = append(queried, s)
				}
			}
		}))
		defer func() { _ = e.db.Callback().Query().Remove(name) }()

		err := e.db.Transaction(func(tx *gorm.DB) error {
			_, lerr := lockAccounts(tx, e.user.ID, []string{c.ID, e.cash.ID, b.ID}, true)
			return lerr
		})
		testutil.AssertNoError(t, err)

		want := []string{e.cash.ID, b.ID, c.ID}
		sort.Strings(want)
		if !reflect.DeepEqual(queried, want) {
			t.Errorf("lock order = %v, want ascending %v", queried, want)
		}
	})

	t.Run("inactive accounts depend on requireActive", func(t *testing.T) {
		e := newRecEnv(t)
		testutil.AssertNoError(t, e.db.Model(e.cash).Update("is_active", false).Error)
		err := e.db.Transaction(func(tx *gorm.DB) error {
			_, lerr := lockAccounts(tx, e.user.ID, []string{e.cash.ID}, true)
			return lerr
		})
		assertAppCode(t, err, "ACCOUNT_NOT_FOUND", http.StatusNotFound)
		err = e.db.Transaction(func(tx *gorm.DB) error {
			got, lerr := lockAccounts(tx, e.user.ID, []string{e.cash.ID}, false)
			if lerr == nil && got[e.cash.ID] == nil {
				t.Error("inactive account should be returned when requireActive is false")
			}
			return lerr
		})
		testutil.AssertNoError(t, err)
	})

	t.Run("is user scoped and rejects missing ids", func(t *testing.T) {
		e := newRecEnv(t)
		_, otherAcct, _ := e.otherUser()
		for _, id := range []string{otherAcct.ID, "00000000-0000-0000-0000-00000000dead"} {
			err := e.db.Transaction(func(tx *gorm.DB) error {
				_, lerr := lockAccounts(tx, e.user.ID, []string{e.cash.ID, id}, true)
				return lerr
			})
			if err == nil {
				t.Fatalf("expected ACCOUNT_NOT_FOUND for %s", id)
			}
			assertAppCode(t, err, apperrors.ErrAccountNotFound.Code, http.StatusNotFound)
		}
	})
}
