package services

import (
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

// searchIDs runs a list query and returns the matching transaction ids.
func searchIDs(t *testing.T, svc TransactionServicer, userID string, f TransactionFilter) map[string]bool {
	t.Helper()
	page, err := svc.GetUserTransactions(userID, pagination.PageRequest{Page: 1, PageSize: 100}, f)
	testutil.AssertNoError(t, err)
	out := map[string]bool{}
	for i := range page.Data {
		out[page.Data[i].ID] = true
	}
	return out
}

func TestTransactionSearch(t *testing.T) {
	e := newRecEnv(t)
	user := e.user.ID

	create := func(amount int64, desc, notes string) *models.Transaction {
		tx, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, amount, desc, time.Now(), TransactionMeta{Notes: notes})
		testutil.AssertNoError(t, err)
		return tx
	}
	coffee := create(1250, "Starbucks Coffee", "")
	lunch := create(6473, "Lunch", "team lunch with Alex")
	other := create(900, "100%_off voucher", "")

	// A transfer carrying a to_amount, and a shared expense (personal share 60.00).
	dest := testutil.CreateTestCashAccountWithBalance(t, e.db, user, 0)
	xfer, err := e.txn.CreateTransfer(user, e.cash.ID, dest.ID, 1000, nil, "Move", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Where("id = ?", xfer.ID).Update("to_amount", 123456).Error)
	sharedTx, err := e.rec.CreateSharedExpense(user, e.in(9000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alex", 3000)))
	testutil.AssertNoError(t, err)

	cases := []struct {
		name string
		q    string
		want []string
	}{
		{"description, case-insensitive", "starbucks", []string{coffee.ID}},
		{"notes", "ALEX", []string{lunch.ID}},
		{"amount with decimals", "64.73", []string{lunch.ID}},
		{"amount whole number means dollars", "9", []string{other.ID}},
		{"transfer to_amount", "1234.56", []string{xfer.ID}},
		{"personal_amount of a shared expense", "60", []string{sharedTx.ID}},
		{"amount of a shared expense", "90.00", []string{sharedTx.ID}},
		{"wildcards are literal", "%_", []string{other.ID}},
		{"trimmed", "  coffee  ", []string{coffee.ID}},
		{"no match", "zzz", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			got := searchIDs(t, e.txn, user, TransactionFilter{Search: &q})
			if len(got) != len(tc.want) {
				t.Fatalf("q=%q: got %d rows, want %d", tc.q, len(got), len(tc.want))
			}
			for _, id := range tc.want {
				if !got[id] {
					t.Errorf("q=%q: missing %s", tc.q, id)
				}
			}
		})
	}

	t.Run("search term is capped at 100 characters", func(t *testing.T) {
		q := strings.Repeat("x", 500)
		_ = searchIDs(t, e.txn, user, TransactionFilter{Search: &q}) // must not error
	})

	t.Run("scoped to the user", func(t *testing.T) {
		stranger := testutil.CreateTestUserWithEmail(t, e.db, "stranger@example.com")
		q := "coffee"
		if got := searchIDs(t, e.txn, stranger.ID, TransactionFilter{Search: &q}); len(got) != 0 {
			t.Fatalf("another user's search returned %d rows", len(got))
		}
	})
}

func TestTransactionFilters_SharedAndAttachment(t *testing.T) {
	e := newRecEnv(t)
	user := e.user.ID

	plain, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 1000, "plain", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	shared := e.mustCreate(e.in(9000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alex", 3000)))
	att := &models.TransactionAttachment{
		UserID: user, TransactionID: plain.ID, StorageKey: "k", ContentType: "image/png", ByteSize: 1,
		ChecksumSHA256: strings.Repeat("a", 64),
	}
	testutil.AssertNoError(t, e.db.Create(att).Error)

	got := searchIDs(t, e.txn, user, TransactionFilter{Shared: boolp(true)})
	if len(got) != 1 || !got[shared.ID] {
		t.Errorf("shared=true: %v", got)
	}
	got = searchIDs(t, e.txn, user, TransactionFilter{Shared: boolp(false)})
	if got[shared.ID] || !got[plain.ID] {
		t.Errorf("shared=false: %v", got)
	}
	got = searchIDs(t, e.txn, user, TransactionFilter{HasAttachment: boolp(true)})
	if len(got) != 1 || !got[plain.ID] {
		t.Errorf("has_attachment=true: %v", got)
	}
	got = searchIDs(t, e.txn, user, TransactionFilter{HasAttachment: boolp(false)})
	if got[plain.ID] || !got[shared.ID] {
		t.Errorf("has_attachment=false: %v", got)
	}
}

func TestTransactionNotesAndSource(t *testing.T) {
	e := newRecEnv(t)
	user := e.user.ID

	t.Run("create defaults to web and stores notes", func(t *testing.T) {
		tx, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(), TransactionMeta{Notes: "hello"})
		testutil.AssertNoError(t, err)
		if tx.Source != models.TransactionSourceWeb || tx.Notes != "hello" || tx.SourceClientID != nil {
			t.Errorf("got source=%q notes=%q client=%v", tx.Source, tx.Notes, tx.SourceClientID)
		}
	})

	t.Run("mcp source records the client id", func(t *testing.T) {
		tx, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(),
			TransactionMeta{Source: models.TransactionSourceMCP, SourceClientID: "client-1"})
		testutil.AssertNoError(t, err)
		got, err := e.txn.GetTransactionByID(user, tx.ID)
		testutil.AssertNoError(t, err)
		if got.Source != models.TransactionSourceMCP || got.SourceClientID == nil || *got.SourceClientID != "client-1" {
			t.Errorf("got source=%q client=%v", got.Source, got.SourceClientID)
		}
	})

	t.Run("telegram source on a transfer", func(t *testing.T) {
		other := testutil.CreateTestCashAccountWithBalance(t, e.db, user, 0)
		tx, err := e.txn.CreateTransfer(user, e.cash.ID, other.ID, 100, nil, "move", time.Now(),
			TransactionMeta{Source: models.TransactionSourceTelegram, Notes: "n"})
		testutil.AssertNoError(t, err)
		if tx.Source != models.TransactionSourceTelegram || tx.Notes != "n" {
			t.Errorf("got source=%q notes=%q", tx.Source, tx.Notes)
		}
	})

	t.Run("invalid source and over-long notes are rejected", func(t *testing.T) {
		_, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(), TransactionMeta{Source: "bogus"})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
		_, err = e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(),
			TransactionMeta{Notes: strings.Repeat("é", MaxNotesLen+1)})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
		// 2000 multi-byte characters is within the limit.
		_, err = e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(),
			TransactionMeta{Notes: strings.Repeat("é", MaxNotesLen)})
		testutil.AssertNoError(t, err)
	})

	t.Run("update changes notes", func(t *testing.T) {
		tx, err := e.txn.CreateTransaction(user, e.cash.ID, nil, models.TransactionTypeExpense, 100, "a", time.Now(), TransactionMeta{Notes: "old"})
		testutil.AssertNoError(t, err)
		got, err := e.txn.UpdateTransaction(user, tx.ID, TransactionUpdateFields{Notes: strp("new")})
		testutil.AssertNoError(t, err)
		if got.Notes != "new" || got.Source != models.TransactionSourceWeb {
			t.Errorf("notes=%q source=%q", got.Notes, got.Source)
		}
	})

	t.Run("shared expense carries meta", func(t *testing.T) {
		in := e.in(9000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alex", 3000))
		in.Meta = TransactionMeta{Notes: "split", Source: models.TransactionSourceTelegram}
		tx, err := e.rec.CreateSharedExpense(user, in)
		testutil.AssertNoError(t, err)
		if tx.Notes != "split" || tx.Source != models.TransactionSourceTelegram {
			t.Errorf("notes=%q source=%q", tx.Notes, tx.Source)
		}
	})

	t.Run("account creation writes a system opening balance", func(t *testing.T) {
		acct, err := e.acct.CreateCashAccount(user, "New", "", "USD", 5000)
		testutil.AssertNoError(t, err)
		ob := openingBalanceOf(t, e.txn, user, acct.ID)
		if ob.Source != models.TransactionSourceSystem {
			t.Errorf("opening balance source = %q", ob.Source)
		}
	})
}

func TestBulkUpdateTransactions(t *testing.T) {
	newExpenses := func(t *testing.T, e *recEnv, n int) []string {
		ids := make([]string, n)
		for i := range ids {
			tx, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 1000, "e", time.Now(), TransactionMeta{Notes: "base"})
			testutil.AssertNoError(t, err)
			ids[i] = tx.ID
		}
		return ids
	}
	auditCount := func(e *recEnv, action string) int64 {
		var n int64
		e.db.Model(&models.AuditLog{}).Where("action = ?", action).Count(&n)
		return n
	}

	t.Run("recategorise and append notes with one audit row per change", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 3)
		res, err := e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: ids, Set: BulkTransactionSet{CategoryID: &e.cat.ID, NotesAppend: strp("reviewed")},
		})
		testutil.AssertNoError(t, err)
		if res.Updated != 3 || res.Deleted != 0 {
			t.Fatalf("result = %+v", res)
		}
		for _, id := range ids {
			got, err := e.txn.GetTransactionByID(e.user.ID, id)
			testutil.AssertNoError(t, err)
			if got.CategoryID == nil || *got.CategoryID != e.cat.ID || got.Notes != "base\nreviewed" {
				t.Errorf("%s: category=%v notes=%q", id, got.CategoryID, got.Notes)
			}
		}
		if n := auditCount(e, "BULK_UPDATE_TRANSACTION"); n != 3 {
			t.Errorf("audit rows = %d, want 3", n)
		}
	})

	t.Run("a bad id rolls everything back", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 2)
		_, err := e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: append(ids, "00000000-0000-0000-0000-000000000000"), Set: BulkTransactionSet{CategoryID: &e.cat.ID},
		})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
		got, _ := e.txn.GetTransactionByID(e.user.ID, ids[0])
		if got.CategoryID != nil {
			t.Error("category changed despite the failed request")
		}
	})

	t.Run("a late failure rolls back earlier rows and audit rows", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 2)
		income, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeIncome, 500, "pay", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)
		_, err = e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: append(ids, income.ID), Set: BulkTransactionSet{CategoryID: &e.cat.ID},
		})
		testutil.AssertAppError(t, err, "CATEGORY_TYPE_MISMATCH")
		for _, id := range ids {
			got, _ := e.txn.GetTransactionByID(e.user.ID, id)
			if got.CategoryID != nil {
				t.Error("category changed despite the failed request")
			}
		}
		if n := auditCount(e, "BULK_UPDATE_TRANSACTION"); n != 0 {
			t.Errorf("audit rows = %d, want 0", n)
		}
	})

	t.Run("another user's transaction is not found", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 1)
		stranger := testutil.CreateTestUserWithEmail(t, e.db, "stranger@example.com")
		_, err := e.txn.BulkUpdateTransactions(stranger.ID, BulkTransactionInput{
			IDs: ids, Set: BulkTransactionSet{NotesAppend: strp("x")},
		})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})

	t.Run("another user's category is not found", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 1)
		stranger := testutil.CreateTestUserWithEmail(t, e.db, "stranger@example.com")
		foreign := testutil.CreateTestCategory(t, e.db, stranger.ID, models.CategoryTypeExpense)
		_, err := e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: ids, Set: BulkTransactionSet{CategoryID: &foreign.ID},
		})
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("opening balances are rejected", func(t *testing.T) {
		e := newRecEnv(t)
		acct, err := e.acct.CreateCashAccount(e.user.ID, "New", "", "USD", 5000)
		testutil.AssertNoError(t, err)
		ob := openingBalanceOf(t, e.txn, e.user.ID, acct.ID)
		_, err = e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: []string{ob.ID}, Set: BulkTransactionSet{NotesAppend: strp("x")},
		})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_EDITABLE")
		_, err = e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{IDs: []string{ob.ID}, Delete: true})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_EDITABLE")
	})

	t.Run("input validation", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 1)
		many := make([]string, MaxBulkTransactions+1)
		for i := range many {
			many[i] = ids[0]
		}
		bad := []BulkTransactionInput{
			{IDs: nil, Set: BulkTransactionSet{NotesAppend: strp("x")}},
			{IDs: many, Set: BulkTransactionSet{NotesAppend: strp("x")}},
			{IDs: ids},
			{IDs: ids, Delete: true, Set: BulkTransactionSet{NotesAppend: strp("x")}},
			{IDs: ids, Set: BulkTransactionSet{Shared: boolp(true)}},
			{IDs: ids, Set: BulkTransactionSet{NotesAppend: strp("   ")}},
			{IDs: ids, Set: BulkTransactionSet{NotesAppend: strp(strings.Repeat("a", MaxNotesLen+1))}},
		}
		for i, in := range bad {
			_, err := e.txn.BulkUpdateTransactions(e.user.ID, in)
			if err == nil {
				t.Errorf("case %d: expected an error", i)
			}
		}
	})

	t.Run("appending past the notes limit fails", func(t *testing.T) {
		e := newRecEnv(t)
		tx, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 100, "e", time.Now(),
			TransactionMeta{Notes: strings.Repeat("a", MaxNotesLen)})
		testutil.AssertNoError(t, err)
		_, err = e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: []string{tx.ID}, Set: BulkTransactionSet{NotesAppend: strp("more")},
		})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("bulk delete reverses balances and audits each row", func(t *testing.T) {
		e := newRecEnv(t)
		ids := newExpenses(t, e, 3)
		res, err := e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{IDs: ids, Delete: true})
		testutil.AssertNoError(t, err)
		if res.Deleted != 3 {
			t.Fatalf("result = %+v", res)
		}
		e.wantBalance(e.cash.ID, recCashStart)
		if n := auditCount(e, "BULK_DELETE_TRANSACTION"); n != 3 {
			t.Errorf("audit rows = %d, want 3", n)
		}
		_, err = e.txn.GetTransactionByID(e.user.ID, ids[0])
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})

	t.Run("unshare removes receivables and restores the full amount", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(9000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("Alex", 3000)))
		res, err := e.txn.BulkUpdateTransactions(e.user.ID, BulkTransactionInput{
			IDs: []string{x.ID}, Set: BulkTransactionSet{Shared: boolp(false)},
		})
		testutil.AssertNoError(t, err)
		if res.Updated != 1 {
			t.Fatalf("result = %+v", res)
		}
		got, err := e.txn.GetTransactionByID(e.user.ID, x.ID)
		testutil.AssertNoError(t, err)
		if got.PersonalAmount != nil || len(got.Receivables) != 0 {
			t.Errorf("personal=%v receivables=%d", got.PersonalAmount, len(got.Receivables))
		}
	})
}
