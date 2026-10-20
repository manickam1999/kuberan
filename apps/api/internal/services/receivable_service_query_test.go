package services

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// day returns midnight UTC, n days before today.
func day(n int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).AddDate(0, 0, -n)
}

// sharedOn creates a custom-mode shared expense dated daysAgo days back.
func (e *recEnv) sharedOn(daysAgo int, kind models.ReceivableKind, desc string, cps ...SharedCounterpartyInput) *models.Transaction {
	e.t.Helper()
	in := e.in(100000, ShareModeCustom, kind, cps...)
	in.Date = day(daysAgo)
	in.Description = desc
	return e.mustCreate(in)
}

func TestListReceivables(t *testing.T) {
	e := newRecEnv(t)
	// newest first expected: dinner (1d) < lunch (3d) < trip (10d) in age.
	trip := e.sharedOn(10, models.ReceivableKindLoan, "trip", rcpAmt("Alex", 5000))
	lunch := e.sharedOn(3, models.ReceivableKindSplit, "lunch", rcpAmt("Alex", 1000), rcpAmt("Sam", 2000))
	dinner := e.sharedOn(1, models.ReceivableKindClaim, "dinner", rcpAmt("Kim", 3000))
	e.mustSettle(lunch.Receivables[0].ID, 1000)       // Alex/lunch -> settled
	e.mustSettle(lunch.Receivables[1].ID, 500)        // Sam/lunch -> partially_paid
	_, err := e.writeOff(trip.Receivables[0].ID, nil) // Alex/trip -> written_off
	testutil.AssertNoError(t, err)

	list := func(f ReceivableFilter) *pagination.PageResponse[models.Receivable] {
		t.Helper()
		res, err := e.rec.ListReceivables(e.user.ID, pageReq(), f)
		testutil.AssertNoError(t, err)
		return res
	}
	names := func(res *pagination.PageResponse[models.Receivable]) string {
		var out []string
		for _, r := range res.Data {
			out = append(out, r.CounterpartyName+"/"+r.Transaction.Description)
		}
		return strings.Join(out, ",")
	}

	t.Run("all, ordered by expense date desc", func(t *testing.T) {
		res := list(ReceivableFilter{})
		if res.TotalItems != 4 {
			t.Fatalf("total = %d", res.TotalItems)
		}
		got := names(res)
		if !strings.HasPrefix(got, "Kim/dinner,") || !strings.HasSuffix(got, "Alex/trip") {
			t.Errorf("order = %s", got)
		}
	})
	t.Run("items carry the expense and outstanding", func(t *testing.T) {
		res := list(ReceivableFilter{CounterpartyKey: "kim"})
		r := res.Data[0]
		if r.Transaction == nil || r.Transaction.ID != dinner.ID || r.Transaction.Amount != 100000 ||
			r.Transaction.PersonalAmount == nil || *r.Transaction.PersonalAmount != 97000 ||
			r.Transaction.AccountID != e.cash.ID || r.Transaction.CategoryID == nil {
			t.Errorf("transaction = %+v", r.Transaction)
		}
		if r.Outstanding != 3000 {
			t.Errorf("outstanding = %d", r.Outstanding)
		}
	})

	filters := []struct {
		name   string
		filter ReceivableFilter
		want   int64
	}{
		{"pending", ReceivableFilter{Status: "pending"}, 1},
		{"partially_paid", ReceivableFilter{Status: "partially_paid"}, 1},
		{"settled", ReceivableFilter{Status: "settled"}, 1},
		{"written_off", ReceivableFilter{Status: "written_off"}, 1},
		{"open = pending + partially_paid", ReceivableFilter{Status: "open"}, 2},
		{"kind split", ReceivableFilter{Kind: "split"}, 2},
		{"kind loan", ReceivableFilter{Kind: "loan"}, 1},
		{"kind deposit (none)", ReceivableFilter{Kind: "deposit"}, 0},
		{"counterparty key", ReceivableFilter{CounterpartyKey: "alex"}, 2},
		{"counterparty key is normalized", ReceivableFilter{CounterpartyKey: "  ALEX "}, 2},
		{"combined", ReceivableFilter{Status: "open", Kind: "split", CounterpartyKey: "sam"}, 1},
		{"unknown counterparty", ReceivableFilter{CounterpartyKey: "nobody"}, 0},
	}
	for _, tt := range filters {
		t.Run("filter "+tt.name, func(t *testing.T) {
			res := list(tt.filter)
			if res.TotalItems != tt.want || int64(len(res.Data)) != tt.want {
				t.Errorf("got %d (%d rows), want %d: %s", res.TotalItems, len(res.Data), tt.want, names(res))
			}
		})
	}

	t.Run("invalid filters", func(t *testing.T) {
		_, err := e.rec.ListReceivables(e.user.ID, pageReq(), ReceivableFilter{Status: "done"})
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
		_, err = e.rec.ListReceivables(e.user.ID, pageReq(), ReceivableFilter{Kind: "gift"})
		assertAppCode(t, err, "INVALID_RECEIVABLE_KIND", http.StatusBadRequest)
	})

	t.Run("pagination", func(t *testing.T) {
		res, err := e.rec.ListReceivables(e.user.ID, pagination.PageRequest{Page: 2, PageSize: 3}, ReceivableFilter{})
		testutil.AssertNoError(t, err)
		if res.TotalItems != 4 || res.TotalPages != 2 || len(res.Data) != 1 || res.Data[0].Transaction.Description != "trip" {
			t.Errorf("page 2 = %+v", res)
		}
	})

	t.Run("deleted expenses and receivables are excluded", func(t *testing.T) {
		gone := e.sharedOn(0, models.ReceivableKindSplit, "gone", rcpAmt("Zed", 10))
		testutil.AssertNoError(t, e.deleteTxn(gone.ID))
		res := list(ReceivableFilter{CounterpartyKey: "zed"})
		if res.TotalItems != 0 {
			t.Errorf("deleted expense's receivables listed: %d", res.TotalItems)
		}
	})

	t.Run("empty list is a non-nil array", func(t *testing.T) {
		e2 := newRecEnv(t)
		res, err := e2.rec.ListReceivables(e2.user.ID, pageReq(), ReceivableFilter{})
		testutil.AssertNoError(t, err)
		if res.Data == nil || len(res.Data) != 0 {
			t.Errorf("data = %#v", res.Data)
		}
	})
}

func TestGetReceivable(t *testing.T) {
	e := newRecEnv(t)
	x := e.split(6000, "Alex", "Sam")
	id := x.Receivables[0].ID
	e.mustSettle(id, 500)
	e.mustSettle(id, 700)

	r := e.get(id)
	if len(r.Settlements) != 2 || r.Settlements[0].Amount != 500 || r.Settlements[1].Amount != 700 {
		t.Errorf("settlements = %+v", r.Settlements)
	}
	if r.Transaction == nil || r.Transaction.ID != x.ID || r.Outstanding != 800 || r.Settlements[0].AccountID != e.cash.ID {
		t.Errorf("receivable = %+v", r)
	}
	// Deleted settlements disappear from the detail.
	_, err := e.delSettlement(r.Settlements[0].ID)
	testutil.AssertNoError(t, err)
	if got := e.get(id); len(got.Settlements) != 1 || got.Settlements[0].Amount != 700 {
		t.Errorf("settlements after delete = %+v", got.Settlements)
	}
	_, err = e.rec.GetReceivable(e.user.ID, "missing")
	assertAppCode(t, err, "RECEIVABLE_NOT_FOUND", http.StatusNotFound)
}

func TestSummary(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		e := newRecEnv(t)
		s, err := e.rec.Summary(e.user.ID)
		testutil.AssertNoError(t, err)
		if s.TotalOutstanding != 0 || s.ByKind == nil || s.ByCounterparty == nil || len(s.ByKind) != 0 || len(s.ByCounterparty) != 0 {
			t.Errorf("summary = %#v", s)
		}
	})

	e := newRecEnv(t)
	e.sharedOn(20, models.ReceivableKindLoan, "old loan", rcpAmt("Alex", 7000))
	mid := e.sharedOn(10, models.ReceivableKindSplit, "lunch", rcpAmt("alex", 1000), rcpAmt("Sam", 2000))
	e.sharedOn(2, models.ReceivableKindSplit, "settled one", rcpAmt("Kim", 900))
	deleted := e.sharedOn(1, models.ReceivableKindSplit, "deleted", rcpAmt("Zed", 5000))
	dropped := e.sharedOn(1, models.ReceivableKindClaim, "dropped", rcpAmt("Dee", 400), rcpAmt("Dan", 600))

	// Kim: fully repaid -> excluded. Sam: part paid -> counts the remainder.
	kim, err := e.rec.ListReceivables(e.user.ID, pageReq(), ReceivableFilter{CounterpartyKey: "kim"})
	testutil.AssertNoError(t, err)
	e.mustSettle(kim.Data[0].ID, 900)
	e.mustSettle(mid.Receivables[1].ID, 500)
	// Soft-deleted expense and a soft-deleted receivable are excluded.
	testutil.AssertNoError(t, e.deleteTxn(deleted.ID))
	_, err = e.update(dropped.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{shareIn(&dropped.Receivables[1].ID, "Dan", 600)}})
	testutil.AssertNoError(t, err)
	// A write-off removes the receivable from the open totals too.
	_, err = e.writeOff(dropped.Receivables[1].ID, nil)
	testutil.AssertNoError(t, err)

	s, err := e.rec.Summary(e.user.ID)
	testutil.AssertNoError(t, err)

	// Open: Alex loan 7000, alex split 1000, Sam 1500.
	if s.TotalOutstanding != 9500 {
		t.Errorf("total = %d, want 9500", s.TotalOutstanding)
	}
	if len(s.ByCounterparty) != 2 {
		t.Fatalf("by_counterparty = %+v", s.ByCounterparty)
	}
	a, sam := s.ByCounterparty[0], s.ByCounterparty[1]
	if a.CounterpartyKey != "alex" || a.Outstanding != 8000 || a.OpenCount != 2 {
		t.Errorf("alex = %+v", a)
	}
	// Name comes from the most recently created receivable (the lowercase "alex").
	if a.CounterpartyName != "alex" {
		t.Errorf("alex name = %q, want the newest receivable's name", a.CounterpartyName)
	}
	if !a.OldestDate.Equal(day(20)) {
		t.Errorf("alex oldest = %v, want %v", a.OldestDate, day(20))
	}
	if sam.CounterpartyKey != "sam" || sam.Outstanding != 1500 || sam.OpenCount != 1 || !sam.OldestDate.Equal(day(10)) {
		t.Errorf("sam = %+v", sam)
	}

	if len(s.ByKind) != 2 {
		t.Fatalf("by_kind = %+v", s.ByKind)
	}
	if k := s.ByKind[0]; k.Kind != models.ReceivableKindLoan || k.Outstanding != 7000 || k.OpenCount != 1 {
		t.Errorf("loan = %+v", k)
	}
	if k := s.ByKind[1]; k.Kind != models.ReceivableKindSplit || k.Outstanding != 2500 || k.OpenCount != 2 {
		t.Errorf("split = %+v", k)
	}
}

func TestListCounterparties(t *testing.T) {
	e := newRecEnv(t)
	e.sharedOn(5, models.ReceivableKindSplit, "a", rcpAmt("Alex Tan", 10))
	e.sharedOn(4, models.ReceivableKindSplit, "b", rcpAmt("Sam", 10), rcpAmt("Alexa", 10))
	e.sharedOn(3, models.ReceivableKindSplit, "c", rcpAmt("ALEX TAN", 10))
	e.sharedOn(2, models.ReceivableKindSplit, "d", rcpAmt("Bob", 10))

	keys := func(q string) string {
		t.Helper()
		res, err := e.rec.ListCounterparties(e.user.ID, q)
		testutil.AssertNoError(t, err)
		var out []string
		for _, c := range res {
			out = append(out, c.Key+"="+c.Name)
		}
		return strings.Join(out, "|")
	}

	t.Run("most recent first, distinct by key, newest display name", func(t *testing.T) {
		got := keys("")
		if got != "bob=Bob|alex tan=ALEX TAN|alexa=Alexa|sam=Sam" {
			t.Errorf("all = %s", got)
		}
	})
	t.Run("contains match, case and whitespace insensitive", func(t *testing.T) {
		if got := keys("  ALEX "); got != "alex tan=ALEX TAN|alexa=Alexa" {
			t.Errorf("alex = %s", got)
		}
		if got := keys("tan"); got != "alex tan=ALEX TAN" {
			t.Errorf("tan = %s (contains must match anywhere)", got)
		}
		if got := keys("zzz"); got != "" {
			t.Errorf("zzz = %s", got)
		}
	})
	t.Run("LIKE wildcards are literal", func(t *testing.T) {
		if got := keys("%"); got != "" {
			t.Errorf("%% = %s", got)
		}
		if got := keys("_"); got != "" {
			t.Errorf("_ = %s", got)
		}
	})
	t.Run("capped at 20", func(t *testing.T) {
		e2 := newRecEnv(t)
		for i := 0; i < 25; i++ {
			e2.sharedOn(0, models.ReceivableKindSplit, "x", rcpAmt(fmt.Sprintf("Person %02d", i), 10))
		}
		res, err := e2.rec.ListCounterparties(e2.user.ID, "person")
		testutil.AssertNoError(t, err)
		if len(res) != 20 || res[0].Key != "person 24" {
			t.Errorf("got %d, first %v", len(res), res[0])
		}
	})
	t.Run("never returns an empty non-nil slice as null", func(t *testing.T) {
		res, err := e.rec.ListCounterparties(e.user.ID, "nothing-matches")
		testutil.AssertNoError(t, err)
		if res == nil {
			t.Error("want empty slice, got nil")
		}
	})
}

func TestRenameCounterparty(t *testing.T) {
	t.Run("rename updates name and key on all receivables", func(t *testing.T) {
		e := newRecEnv(t)
		a := e.sharedOn(3, models.ReceivableKindSplit, "a", rcpAmt("Alx", 10))
		b := e.sharedOn(2, models.ReceivableKindLoan, "b", rcpAmt("alx", 20))
		e.sharedOn(1, models.ReceivableKindSplit, "c", rcpAmt("Sam", 30))
		n, err := e.rec.RenameCounterparty(e.user.ID, "  ALX ", "  Alex   Tan ")
		testutil.AssertNoError(t, err)
		e.check()
		if n != 2 {
			t.Errorf("updated = %d, want 2", n)
		}
		for _, id := range []string{a.Receivables[0].ID, b.Receivables[0].ID} {
			if r := e.get(id); r.CounterpartyName != "Alex Tan" || r.CounterpartyKey != "alex tan" {
				t.Errorf("receivable = %+v", r)
			}
		}
		res, _ := e.rec.ListCounterparties(e.user.ID, "alx")
		if len(res) != 0 {
			t.Errorf("old key still suggested: %+v", res)
		}
	})
	t.Run("renaming onto an existing key merges", func(t *testing.T) {
		e := newRecEnv(t)
		e.sharedOn(3, models.ReceivableKindSplit, "a", rcpAmt("Alex", 1000))
		e.sharedOn(2, models.ReceivableKindSplit, "b", rcpAmt("Alexander", 2000))
		n, err := e.rec.RenameCounterparty(e.user.ID, "alexander", "alex")
		testutil.AssertNoError(t, err)
		if n != 1 {
			t.Errorf("updated = %d", n)
		}
		s, err := e.rec.Summary(e.user.ID)
		testutil.AssertNoError(t, err)
		if len(s.ByCounterparty) != 1 || s.ByCounterparty[0].Outstanding != 3000 || s.ByCounterparty[0].OpenCount != 2 {
			t.Errorf("summary = %+v", s.ByCounterparty)
		}
	})
	t.Run("an expense holding merged duplicates stays editable", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.sharedOn(1, models.ReceivableKindSplit, "dup", rcpAmt("Alex", 1000), rcpAmt("Alexander", 2000))
		_, err := e.rec.RenameCounterparty(e.user.ID, "alexander", "Alex")
		testutil.AssertNoError(t, err)
		_, err = e.update(x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{
			shareIn(&x.Receivables[0].ID, "Alex", 1000), shareIn(&x.Receivables[1].ID, "Alex", 2500),
		}})
		testutil.AssertNoError(t, err)
	})
	t.Run("unknown key updates nothing", func(t *testing.T) {
		e := newRecEnv(t)
		n, err := e.rec.RenameCounterparty(e.user.ID, "ghost", "Boo")
		testutil.AssertNoError(t, err)
		if n != 0 {
			t.Errorf("updated = %d", n)
		}
	})
	t.Run("validation", func(t *testing.T) {
		e := newRecEnv(t)
		_, err := e.rec.RenameCounterparty(e.user.ID, "alex", "   ")
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		_, err = e.rec.RenameCounterparty(e.user.ID, "alex", strings.Repeat("x", 101))
		assertAppCode(t, err, "INVALID_SHARES", http.StatusBadRequest)
		_, err = e.rec.RenameCounterparty(e.user.ID, "  ", "Alex")
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest)
	})
	t.Run("other users' receivables are untouched", func(t *testing.T) {
		e := newRecEnv(t)
		e.sharedOn(1, models.ReceivableKindSplit, "mine", rcpAmt("Alex", 10))
		other := NewReceivableService(e.db, e.acct, nil)
		u, a, c := e.otherUser()
		_, err := other.CreateSharedExpense(u.ID, CreateSharedExpenseInput{
			AccountID: a.ID, CategoryID: &c.ID, Amount: 100, Kind: models.ReceivableKindSplit,
			Mode: ShareModeCustom, Counterparties: []SharedCounterpartyInput{rcpAmt("Alex", 10)},
		})
		testutil.AssertNoError(t, err)
		n, err := e.rec.RenameCounterparty(e.user.ID, "alex", "Alexis")
		testutil.AssertNoError(t, err)
		if n != 1 {
			t.Errorf("updated = %d, want only my own", n)
		}
		res, err := other.ListCounterparties(u.ID, "")
		testutil.AssertNoError(t, err)
		if len(res) != 1 || res[0].Name != "Alex" {
			t.Errorf("other user's suggestions = %+v", res)
		}
	})
}

// TestReceivables_UserIsolation proves every method is scoped to the calling user.
func TestReceivables_UserIsolation(t *testing.T) {
	e := newRecEnv(t)
	x := e.split(6000, "Alex", "Sam")
	_, settlement := e.mustSettle(x.Receivables[0].ID, 500)
	intruder, theirAcct, _ := e.otherUser()
	uid := intruder.ID

	t.Run("create into someone else's account", func(t *testing.T) {
		in := e.in(100, ShareModeFull, models.ReceivableKindLoan, rcpEqual("A")...)
		in.CategoryID = nil
		_, err := e.rec.CreateSharedExpense(uid, in)
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
	t.Run("create with someone else's category", func(t *testing.T) {
		in := e.in(100, ShareModeFull, models.ReceivableKindLoan, rcpEqual("A")...)
		in.AccountID = theirAcct.ID
		_, err := e.rec.CreateSharedExpense(uid, in)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})
	t.Run("update receivables", func(t *testing.T) {
		_, err := e.rec.UpdateReceivables(uid, x.ID, UpdateReceivablesInput{Receivables: []ReceivableShareInput{}})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})
	t.Run("settle", func(t *testing.T) {
		_, _, err := e.rec.Settle(uid, x.Receivables[1].ID, SettleInput{AccountID: theirAcct.ID, Amount: 100})
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
	})
	t.Run("settle my receivable into their account", func(t *testing.T) {
		_, _, err := e.rec.Settle(e.user.ID, x.Receivables[1].ID, SettleInput{AccountID: theirAcct.ID, Amount: 100})
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
	t.Run("delete settlement", func(t *testing.T) {
		_, err := e.rec.DeleteSettlement(uid, settlement.ID)
		testutil.AssertAppError(t, err, "SETTLEMENT_NOT_FOUND")
	})
	t.Run("write off and undo", func(t *testing.T) {
		_, err := e.rec.WriteOff(uid, x.Receivables[1].ID, nil)
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
		_, err = e.rec.UndoWriteOff(uid, x.Receivables[1].ID)
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
	})
	t.Run("get", func(t *testing.T) {
		_, err := e.rec.GetReceivable(uid, x.Receivables[0].ID)
		testutil.AssertAppError(t, err, "RECEIVABLE_NOT_FOUND")
	})
	t.Run("list, summary and counterparties are empty", func(t *testing.T) {
		list, err := e.rec.ListReceivables(uid, pageReq(), ReceivableFilter{})
		testutil.AssertNoError(t, err)
		if list.TotalItems != 0 {
			t.Errorf("list total = %d", list.TotalItems)
		}
		s, err := e.rec.Summary(uid)
		testutil.AssertNoError(t, err)
		if s.TotalOutstanding != 0 || len(s.ByCounterparty) != 0 {
			t.Errorf("summary = %+v", s)
		}
		cps, err := e.rec.ListCounterparties(uid, "")
		testutil.AssertNoError(t, err)
		if len(cps) != 0 {
			t.Errorf("counterparties = %+v", cps)
		}
	})
	t.Run("rename", func(t *testing.T) {
		n, err := e.rec.RenameCounterparty(uid, "alex", "Mallory")
		testutil.AssertNoError(t, err)
		if n != 0 {
			t.Errorf("updated = %d", n)
		}
	})
	t.Run("transaction update and delete", func(t *testing.T) {
		_, err := e.txn.UpdateTransaction(uid, x.ID, TransactionUpdateFields{Amount: i64(7000)})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
		testutil.AssertAppError(t, e.txn.DeleteTransaction(uid, x.ID), "TRANSACTION_NOT_FOUND")
		testutil.AssertAppError(t, e.txn.DeleteTransaction(uid, settlement.TransactionID), "TRANSACTION_NOT_FOUND")
	})

	// Nothing of the above changed my data.
	e.check()
	e.wantBalance(e.cash.ID, recCashStart-6000+500)
	e.wantPersonal(x.ID, 2000)
	if got := e.balance(theirAcct.ID); got != 500000 {
		t.Errorf("their balance changed to %d", got)
	}
}
