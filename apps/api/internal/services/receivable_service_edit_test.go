package services

import (
	"net/http"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func shareIn(id *string, name string, amount int64) ReceivableShareInput {
	return ReceivableShareInput{ID: id, Name: name, Amount: amount}
}

func kindPtr(k models.ReceivableKind) *models.ReceivableKind { return &k }

func TestUpdateReceivables_Diff(t *testing.T) {
	// base: $90 expense, Alex 2000, Sam 3000, Kim 1000 (custom) -> personal 3000.
	setup := func(t *testing.T) (*recEnv, *models.Transaction) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(9000, ShareModeCustom, models.ReceivableKindSplit,
			rcpAmt("Alex", 2000), rcpAmt("Sam", 3000), rcpAmt("Kim", 1000)))
		e.wantPersonal(x.ID, 3000)
		return e, x
	}

	t.Run("update in place: rename and change amount", func(t *testing.T) {
		e, x := setup(t)
		alex, sam, kim := x.Receivables[0], x.Receivables[1], x.Receivables[2]
		res, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&alex.ID, "Alexander", 2500), shareIn(&sam.ID, "Sam", 3000), shareIn(&kim.ID, "Kim", 1000),
		}})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 2500)
		got := e.get(alex.ID)
		if got.CounterpartyName != "Alexander" || got.CounterpartyKey != "alexander" || got.Amount != 2500 || got.Outstanding != 2500 {
			t.Errorf("alex = %+v", got)
		}
		if res.PersonalAmount == nil || *res.PersonalAmount != 2500 || len(res.Receivables) != 3 {
			t.Errorf("returned expense = %+v", res)
		}
		e.wantBalance(e.cash.ID, recCashStart-9000) // editing shares never moves money
	})

	t.Run("insert a new receivable", func(t *testing.T) {
		e, x := setup(t)
		in := UpdateReceivablesInput{}
		for i := range x.Receivables {
			in.Receivables = append(in.Receivables, shareIn(&x.Receivables[i].ID, x.Receivables[i].CounterpartyName, x.Receivables[i].Amount))
		}
		in.Receivables = append(in.Receivables, shareIn(nil, "Jo", 1500))
		res, err := e.update(x.ID, in)
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 1500)
		if len(res.Receivables) != 4 || res.Receivables[3].CounterpartyName != "Jo" || res.Receivables[3].Kind != models.ReceivableKindSplit {
			t.Errorf("receivables = %+v", res.Receivables)
		}
	})

	t.Run("delete an unpaid receivable by omitting it", func(t *testing.T) {
		e, x := setup(t)
		in := UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[0].ID, "Alex", 2000), shareIn(&x.Receivables[1].ID, "Sam", 3000),
		}}
		res, err := e.update(x.ID, in)
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 4000)
		if len(res.Receivables) != 2 {
			t.Errorf("receivables = %d, want 2", len(res.Receivables))
		}
		_, err = e.rec.GetReceivable(e.user.ID, x.Receivables[2].ID)
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
	})

	t.Run("removing a paid receivable is a 409", func(t *testing.T) {
		e, x := setup(t)
		e.mustSettle(x.Receivables[0].ID, 500)
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[1].ID, "Sam", 3000), shareIn(&x.Receivables[2].ID, "Kim", 1000),
		}})
		assertAppCode(t, err, "RECEIVABLE_HAS_PAYMENTS", http.StatusConflict)
		e.wantPersonal(x.ID, 3000)
		if len(e.get(x.Receivables[0].ID).Settlements) != 1 {
			t.Error("the paid receivable must survive a rejected removal")
		}
	})

	t.Run("amount below paid plus written off is rejected", func(t *testing.T) {
		e, x := setup(t)
		alex := x.Receivables[0]
		e.mustSettle(alex.ID, 1500)
		_, err := e.writeOff(alex.ID, i64(200))
		testutil.AssertNoError(t, err)
		build := func(alexAmount int64) UpdateReceivablesInput {
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{
				shareIn(&alex.ID, "Alex", alexAmount), shareIn(&x.Receivables[1].ID, "Sam", 3000), shareIn(&x.Receivables[2].ID, "Kim", 1000),
			}}
		}
		_, err = e.update(x.ID, build(1699))
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		res, err := e.update(x.ID, build(1700)) // exactly paid + written off -> settled-ish
		testutil.AssertNoError(t, err)
		if r := res.Receivables[0]; r.Status != models.ReceivableStatusWrittenOff || r.Outstanding != 0 {
			t.Errorf("alex = %+v", r)
		}
	})

	t.Run("raising a settled receivable reopens it", func(t *testing.T) {
		e, x := setup(t)
		alex := x.Receivables[0]
		e.mustSettle(alex.ID, 2000)
		res, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&alex.ID, "Alex", 2600), shareIn(&x.Receivables[1].ID, "Sam", 3000), shareIn(&x.Receivables[2].ID, "Kim", 1000),
		}})
		testutil.AssertNoError(t, err)
		if r := res.Receivables[0]; r.Status != models.ReceivableStatusPartiallyPaid || r.Outstanding != 600 {
			t.Errorf("alex = %+v", r)
		}
	})

	t.Run("sum above the expense amount is rejected", func(t *testing.T) {
		e, x := setup(t)
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[0].ID, "Alex", 2000), shareIn(&x.Receivables[1].ID, "Sam", 3000),
			shareIn(&x.Receivables[2].ID, "Kim", 1000), shareIn(nil, "Jo", 3001),
		}})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		e.wantPersonal(x.ID, 3000)
		// A sum exactly equal to the amount is fine (personal 0).
		_, err = e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[0].ID, "Alex", 2000), shareIn(&x.Receivables[1].ID, "Sam", 3000),
			shareIn(&x.Receivables[2].ID, "Kim", 1000), shareIn(nil, "Jo", 3000),
		}})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 0)
	})

	t.Run("kind applies to all receivables", func(t *testing.T) {
		e, x := setup(t)
		in := UpdateReceivablesInput{Kind: kindPtr(models.ReceivableKindClaim)}
		for _, r := range x.Receivables {
			in.Receivables = append(in.Receivables, shareIn(&r.ID, r.CounterpartyName, r.Amount))
		}
		in.Receivables = append(in.Receivables, shareIn(nil, "Jo", 100))
		res, err := e.update(x.ID, in)
		testutil.AssertNoError(t, err)
		for _, r := range res.Receivables {
			if r.Kind != models.ReceivableKindClaim {
				t.Errorf("%s kind = %s, want claim", r.CounterpartyName, r.Kind)
			}
		}
		_, err = e.update(x.ID, UpdateReceivablesInput{Kind: kindPtr("bogus"), Receivables: in.Receivables[:1]})
		assertAppCode(t, err, "INVALID_RECEIVABLE_KIND", http.StatusBadRequest)
	})

	t.Run("an empty list removes all and clears personal_amount", func(t *testing.T) {
		e, x := setup(t)
		res, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{}})
		testutil.AssertNoError(t, err)
		if e.personal(x.ID) != nil || res.PersonalAmount != nil || len(res.Receivables) != 0 {
			t.Errorf("expense = %+v", res)
		}
		spend, err := e.txn.GetSpendingByCategory(e.user.ID, time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 1, 0))
		testutil.AssertNoError(t, err)
		if spend.TotalSpent != 9000 {
			t.Errorf("spending = %d, want the full 9000 once nothing is shared", spend.TotalSpent)
		}
	})

	t.Run("an empty list with a paid receivable is a 409", func(t *testing.T) {
		e, x := setup(t)
		e.mustSettle(x.Receivables[0].ID, 100)
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{}})
		assertAppCode(t, err, "RECEIVABLE_HAS_PAYMENTS", http.StatusConflict)
	})

	t.Run("receivables can be added to a plain expense", func(t *testing.T) {
		e := newRecEnv(t)
		plain, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, &e.cat.ID, models.TransactionTypeExpense, 4000, "plain", time.Now())
		testutil.AssertNoError(t, err)
		res, err := e.update(plain.ID, UpdateReceivablesInput{Kind: kindPtr(models.ReceivableKindLoan), Receivables: []ReceivableShareInput{shareIn(nil, "Alex", 4000)}})
		testutil.AssertNoError(t, err)
		e.wantPersonal(plain.ID, 0)
		if res.Receivables[0].Kind != models.ReceivableKindLoan {
			t.Errorf("kind = %s", res.Receivables[0].Kind)
		}
	})

	rejects := []struct {
		name  string
		build func(x *models.Transaction) UpdateReceivablesInput
		code  string
	}{
		{"zero amount", func(x *models.Transaction) UpdateReceivablesInput {
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&x.Receivables[0].ID, "Alex", 0)}}
		}, "INVALID_SHARES"},
		{"blank name", func(x *models.Transaction) UpdateReceivablesInput {
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&x.Receivables[0].ID, "  ", 10)}}
		}, "INVALID_SHARES"},
		{"duplicate id", func(x *models.Transaction) UpdateReceivablesInput {
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&x.Receivables[0].ID, "A", 10), shareIn(&x.Receivables[0].ID, "B", 10)}}
		}, "INVALID_SHARES"},
		{"new entry repeats a key", func(x *models.Transaction) UpdateReceivablesInput {
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&x.Receivables[0].ID, "Alex", 10), shareIn(nil, " alex ", 10)}}
		}, "INVALID_SHARES"},
		{"unknown receivable id", func(_ *models.Transaction) UpdateReceivablesInput {
			missing := "missing"
			return UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&missing, "A", 10)}}
		}, "RECEIVABLE_NOT_FOUND"},
	}
	for _, tt := range rejects {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			e, x := setup(t)
			_, err := e.update(x.ID, tt.build(x))
			testutil.AssertAppError(t, err, tt.code)
			e.wantPersonal(x.ID, 3000)
		})
	}

	t.Run("a receivable of another expense is not found", func(t *testing.T) {
		e, x := setup(t)
		y := e.split(3000, "Zed")
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&y.Receivables[0].ID, "Zed", 10)}})
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
		e.wantPersonal(y.ID, 1500)
	})

	t.Run("not an expense", func(t *testing.T) {
		e := newRecEnv(t)
		income, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeIncome, 4000, "pay", time.Now())
		testutil.AssertNoError(t, err)
		_, err = e.update(income.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(nil, "Alex", 100)}})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
	})

	t.Run("unknown transaction", func(t *testing.T) {
		e := newRecEnv(t)
		_, err := e.update("missing", UpdateReceivablesInput{Receivables: []ReceivableShareInput{}})
		assertAppCode(t, err, "TRANSACTION_NOT_FOUND", http.StatusNotFound)
	})
}

func TestUpdateTransaction_SharedExpense(t *testing.T) {
	t.Run("raising the amount recomputes the personal share and moves the balance", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam") // owed 4000
		res, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: i64(9000)})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 5000)
		e.wantBalance(e.cash.ID, recCashStart-9000)
		if res.PersonalAmount == nil || *res.PersonalAmount != 5000 || len(res.Receivables) != 2 {
			t.Errorf("returned = %+v", res)
		}
	})
	t.Run("lowering the amount to exactly the total owed is allowed", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: i64(4000)})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 0)
		e.wantBalance(e.cash.ID, recCashStart-4000)
	})
	t.Run("an amount below the total owed is INVALID_SHARES", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Amount: i64(3999)})
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		e.wantPersonal(x.ID, 2000)
		e.wantBalance(e.cash.ID, recCashStart-6000)
	})
	t.Run("changing the type away from expense is INVALID_TYPE_CHANGE", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		income := models.TransactionTypeIncome
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Type: &income})
		assertAppCode(t, err, "INVALID_TYPE_CHANGE", http.StatusBadRequest)
		e.wantBalance(e.cash.ID, recCashStart-6000)
	})
	t.Run("a plain expense can still change type", func(t *testing.T) {
		e := newRecEnv(t)
		plain, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 1000, "x", time.Now())
		testutil.AssertNoError(t, err)
		income := models.TransactionTypeIncome
		_, err = e.updateTxn(plain.ID, TransactionUpdateFields{Type: &income})
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart+1000)
	})
	t.Run("changing the account moves the full amount; settlements keep their own account", func(t *testing.T) {
		e := newRecEnv(t)
		other := e.track(testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 20000))
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 2000)
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{AccountID: &other.ID})
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart+2000)
		e.wantBalance(other.ID, 20000-6000)
		if s.AccountID != e.cash.ID {
			t.Errorf("settlement account = %s", s.AccountID)
		}
	})
	t.Run("description and date edits keep the share", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		desc := "renamed"
		when := time.Now().AddDate(0, 0, -3)
		res, err := e.updateTxn(x.ID, TransactionUpdateFields{Description: &desc, Date: &when})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 2000)
		if res.Description != "renamed" {
			t.Errorf("description = %q", res.Description)
		}
	})
	t.Run("a write-off is respected when the amount changes", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, err := e.writeOff(x.Receivables[0].ID, i64(500))
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 2500)
		_, err = e.updateTxn(x.ID, TransactionUpdateFields{Amount: i64(7000)})
		testutil.AssertNoError(t, err)
		e.wantPersonal(x.ID, 3500)
	})
	t.Run("reimbursement type cannot be set", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		r := models.TransactionTypeReimbursement
		_, err := e.updateTxn(x.ID, TransactionUpdateFields{Type: &r})
		assertAppCode(t, err, "INVALID_TYPE_CHANGE", http.StatusBadRequest)
	})
	t.Run("other user's transaction", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		u, _, _ := e.otherUser()
		_, err := e.txn.UpdateTransaction(u.ID, x.ID, TransactionUpdateFields{Amount: i64(9000)})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
		e.wantPersonal(x.ID, 2000)
	})
}

func TestDeleteTransaction_SharedExpense(t *testing.T) {
	t.Run("cascades to the receivables when nothing was repaid", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		testutil.AssertNoError(t, e.deleteTxn(x.ID))
		e.wantBalance(e.cash.ID, recCashStart)
		for _, r := range x.Receivables {
			_, err := e.rec.GetReceivable(e.user.ID, r.ID)
			testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
		}
		var n int64
		e.db.Unscoped().Model(&models.Receivable{}).Where("transaction_id = ? AND deleted_at IS NOT NULL", x.ID).Count(&n)
		if n != 2 {
			t.Errorf("soft-deleted receivables = %d, want 2", n)
		}
		sum, err := e.rec.Summary(e.user.ID)
		testutil.AssertNoError(t, err)
		if sum.TotalOutstanding != 0 {
			t.Errorf("outstanding = %d", sum.TotalOutstanding)
		}
	})
	t.Run("409 with a settlement, then allowed once it is deleted", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[1].ID, 500)
		assertAppCode(t, e.deleteTxn(x.ID), "RECEIVABLE_HAS_SETTLEMENTS", http.StatusConflict)
		e.wantBalance(e.cash.ID, recCashStart-6000+500)
		e.wantPersonal(x.ID, 2000)

		_, err := e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
		testutil.AssertNoError(t, e.deleteTxn(x.ID))
		e.wantBalance(e.cash.ID, recCashStart)
	})
	t.Run("a plain expense is unaffected", func(t *testing.T) {
		e := newRecEnv(t)
		plain, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 1234, "p", time.Now())
		testutil.AssertNoError(t, err)
		testutil.AssertNoError(t, e.deleteTxn(plain.ID))
		e.wantBalance(e.cash.ID, recCashStart)
	})
	t.Run("a failure while deleting leaves everything in place", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		restore := failOnDelete(t, e.db, "transactions")
		err := e.deleteTxn(x.ID)
		restore()
		if err == nil {
			t.Fatal("expected a forced failure")
		}
		e.wantBalance(e.cash.ID, recCashStart-6000)
		if got := e.get(x.Receivables[0].ID); got.Status != models.ReceivableStatusPending {
			t.Errorf("receivable should survive a rolled-back delete: %+v", got)
		}
	})
}

func TestReceivables_Atomicity(t *testing.T) {
	t.Run("settle: no partial rows when the settlement insert fails", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		restore := failOnCreate(t, e.db, "receivable_settlements")
		_, _, err := e.settle(x.Receivables[0].ID, e.cash.ID, 500, false)
		restore()
		if err == nil {
			t.Fatal("expected a forced failure")
		}
		e.wantBalance(e.cash.ID, recCashStart-6000)
		var n int64
		e.db.Model(&models.Transaction{}).Where("type = ?", models.TransactionTypeReimbursement).Count(&n)
		if n != 0 {
			t.Errorf("reimbursement rows = %d, want 0", n)
		}
		if r := e.get(x.Receivables[0].ID); r.PaidAmount != 0 || r.Status != models.ReceivableStatusPending {
			t.Errorf("receivable = %+v", r)
		}
		// And the operation works once the fault is gone.
		e.mustSettle(x.Receivables[0].ID, 500)
	})
	t.Run("create: no expense, receivable or balance change when a receivable insert fails", func(t *testing.T) {
		e := newRecEnv(t)
		restore := failOnCreate(t, e.db, "receivables")
		_, err := e.create(e.in(6000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alex", "Sam")...))
		restore()
		if err == nil {
			t.Fatal("expected a forced failure")
		}
		e.wantBalance(e.cash.ID, recCashStart)
		var n int64
		e.db.Model(&models.Transaction{}).Where("user_id = ?", e.user.ID).Count(&n)
		if n != 0 {
			t.Errorf("transactions = %d, want 0", n)
		}
	})
	t.Run("delete settlement: the reversal is rolled back as a whole", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		restore := failOnDelete(t, e.db, "receivable_settlements")
		_, err := e.delSettlement(s.ID)
		restore()
		if err == nil {
			t.Fatal("expected a forced failure")
		}
		e.wantBalance(e.cash.ID, recCashStart-6000+500)
		if r := e.get(x.Receivables[0].ID); r.PaidAmount != 500 {
			t.Errorf("paid = %d", r.PaidAmount)
		}
		_, err = e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
	})
	t.Run("update receivables: a failed insert keeps the old shares", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		restore := failOnCreate(t, e.db, "receivables")
		_, err := e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[0].ID, "Alex", 1000), shareIn(nil, "New", 500),
		}})
		restore()
		if err == nil {
			t.Fatal("expected a forced failure")
		}
		e.wantPersonal(x.ID, 2000)
		if got := e.get(x.Receivables[0].ID); got.Amount != 2000 {
			t.Errorf("alex amount = %d, want the original 2000", got.Amount)
		}
		if _, err := e.rec.GetReceivable(e.user.ID, x.Receivables[1].ID); err != nil {
			t.Errorf("sam must survive: %v", err)
		}
	})
}

func TestReceivables_PreloadedOnTransactionReads(t *testing.T) {
	e := newRecEnv(t)
	x := e.split(6000, "Alex", "Sam")
	_, _ = e.mustSettle(x.Receivables[0].ID, 500)
	plain, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 700, "plain", time.Now())
	testutil.AssertNoError(t, err)

	check := func(t *testing.T, txns []models.Transaction) {
		t.Helper()
		var shared, other *models.Transaction
		for i := range txns {
			switch txns[i].ID {
			case x.ID:
				shared = &txns[i]
			case plain.ID:
				other = &txns[i]
			}
		}
		if shared == nil || other == nil {
			t.Fatalf("missing rows in %d transactions", len(txns))
		}
		if len(shared.Receivables) != 2 || shared.Receivables[0].Outstanding != 1500 || shared.Receivables[1].Outstanding != 2000 {
			t.Errorf("receivables = %+v", shared.Receivables)
		}
		if len(shared.Receivables[0].Settlements) != 0 {
			t.Error("settlements must not be loaded on transaction reads")
		}
		if shared.PersonalAmount == nil || *shared.PersonalAmount != 2000 {
			t.Errorf("personal_amount = %v", shared.PersonalAmount)
		}
		if len(other.Receivables) != 0 || other.PersonalAmount != nil {
			t.Errorf("plain expense = %+v", other)
		}
	}

	t.Run("user list", func(t *testing.T) {
		page, err := e.txn.GetUserTransactions(e.user.ID, pageReq(), TransactionFilter{})
		testutil.AssertNoError(t, err)
		check(t, page.Data)
	})
	t.Run("account list", func(t *testing.T) {
		page, err := e.txn.GetAccountTransactions(e.user.ID, e.cash.ID, pageReq(), TransactionFilter{})
		testutil.AssertNoError(t, err)
		check(t, page.Data)
	})
	t.Run("detail", func(t *testing.T) {
		got, err := e.txn.GetTransactionByID(e.user.ID, x.ID)
		testutil.AssertNoError(t, err)
		if len(got.Receivables) != 2 || got.Receivables[0].Outstanding != 1500 {
			t.Errorf("detail receivables = %+v", got.Receivables)
		}
		p, err := e.txn.GetTransactionByID(e.user.ID, plain.ID)
		testutil.AssertNoError(t, err)
		if len(p.Receivables) != 0 {
			t.Errorf("plain detail receivables = %+v", p.Receivables)
		}
	})
	t.Run("reimbursement rows carry no receivables", func(t *testing.T) {
		typ := models.TransactionTypeReimbursement
		page, err := e.txn.GetUserTransactions(e.user.ID, pageReq(), TransactionFilter{Type: &typ})
		testutil.AssertNoError(t, err)
		if len(page.Data) != 1 || len(page.Data[0].Receivables) != 0 {
			t.Errorf("reimbursements = %+v", page.Data)
		}
	})
}
