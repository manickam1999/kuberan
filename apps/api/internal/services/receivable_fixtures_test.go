package services

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// recEnv is the shared fixture of the receivable tests. Every operation goes
// through a wrapper that runs check() afterwards, so the plan-021 invariants
// (D6 and the ledger) are asserted after every call, successful or not.
type recEnv struct {
	t       *testing.T
	db      *gorm.DB
	acct    AccountServicer
	txn     TransactionServicer
	rec     ReceivableServicer
	rules   RuleServicer
	user    *models.User
	cash    *models.Account
	cat     *models.Category
	initial map[string]int64 // tracked account id -> balance before any transaction
}

const recCashStart int64 = 1000000

func newRecEnv(t *testing.T) *recEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })

	catSvc := NewCategoryService(db)
	acct := NewAccountService(db)
	rules := NewRuleService(db, catSvc)
	e := &recEnv{
		t:       t,
		db:      db,
		acct:    acct,
		txn:     NewTransactionService(db, acct, rules),
		rec:     NewReceivableService(db, acct, rules),
		rules:   rules,
		initial: map[string]int64{},
	}
	e.user = testutil.CreateTestUser(t, db)
	e.cash = e.track(testutil.CreateTestCashAccountWithBalance(t, db, e.user.ID, recCashStart))
	e.cat = testutil.CreateTestCategory(t, db, e.user.ID, models.CategoryTypeExpense)
	return e
}

// track registers an account whose balance the ledger check should verify.
func (e *recEnv) track(a *models.Account) *models.Account {
	e.initial[a.ID] = a.Balance
	return a
}

func (e *recEnv) balance(accountID string) int64 {
	e.t.Helper()
	var a models.Account
	if err := e.db.First(&a, "id = ?", accountID).Error; err != nil {
		e.t.Fatalf("load account: %v", err)
	}
	return a.Balance
}

// check asserts the D6 invariant, receivable/settlement consistency and the account ledger.
func (e *recEnv) check() {
	e.t.Helper()
	t := e.t

	var txns []models.Transaction
	if err := e.db.Where("user_id = ?", e.user.ID).Find(&txns).Error; err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	var recs []models.Receivable
	if err := e.db.Where("user_id = ?", e.user.ID).Find(&recs).Error; err != nil {
		t.Fatalf("load receivables: %v", err)
	}
	var sets []models.ReceivableSettlement
	if err := e.db.Where("user_id = ?", e.user.ID).Find(&sets).Error; err != nil {
		t.Fatalf("load settlements: %v", err)
	}

	byID := map[string]models.Transaction{}
	for i := range txns {
		byID[txns[i].ID] = txns[i]
	}
	recsByTxn := map[string][]models.Receivable{}
	paidBySettlements := map[string]int64{}
	recByID := map[string]models.Receivable{}
	for i := range recs {
		r := &recs[i]
		if _, ok := byID[r.TransactionID]; !ok {
			t.Errorf("INVARIANT: live receivable %s has no live expense", r.ID)
		}
		recsByTxn[r.TransactionID] = append(recsByTxn[r.TransactionID], *r)
		recByID[r.ID] = *r
		if r.PaidAmount < 0 || r.WrittenOffAmount < 0 || r.PaidAmount+r.WrittenOffAmount > r.Amount {
			t.Errorf("INVARIANT: receivable %s amounts out of range: %+v", r.ID, r)
		}
		if r.Status != r.DeriveStatus() {
			t.Errorf("INVARIANT: receivable %s status %s, derived %s", r.ID, r.Status, r.DeriveStatus())
		}
	}
	reimbursements := 0
	for i := range sets {
		s := &sets[i]
		if _, ok := recByID[s.ReceivableID]; !ok {
			t.Errorf("INVARIANT: live settlement %s has no live receivable", s.ID)
			continue
		}
		paidBySettlements[s.ReceivableID] += s.Amount
		rt, ok := byID[s.TransactionID]
		if !ok || rt.Type != models.TransactionTypeReimbursement || rt.Amount != s.Amount || rt.AccountID != s.AccountID {
			t.Errorf("INVARIANT: settlement %s does not match a live reimbursement: %+v", s.ID, rt)
		}
		if rt.CategoryID != nil {
			t.Errorf("INVARIANT: reimbursement %s must be uncategorized", rt.ID)
		}
	}
	writtenOffBySettlements := map[string]int64{}
	for i := range sets {
		writtenOffBySettlements[sets[i].ReceivableID] += sets[i].WrittenOffAmount
	}
	for i := range recs {
		r := &recs[i]
		if writtenOffBySettlements[r.ID] > r.WrittenOffAmount {
			t.Errorf("INVARIANT: receivable %s written_off %d < sum of settlement write-offs %d", r.ID, r.WrittenOffAmount, writtenOffBySettlements[r.ID])
		}
		if paidBySettlements[r.ID] != r.PaidAmount {
			t.Errorf("INVARIANT: receivable %s paid_amount %d != sum of settlements %d", r.ID, r.PaidAmount, paidBySettlements[r.ID])
		}
	}

	for i := range txns {
		x := &txns[i]
		switch x.Type {
		case models.TransactionTypeReimbursement:
			reimbursements++
			if x.PersonalAmount != nil {
				t.Errorf("INVARIANT: reimbursement %s has personal_amount", x.ID)
			}
		case models.TransactionTypeExpense:
			rs := recsByTxn[x.ID]
			if len(rs) == 0 {
				if x.PersonalAmount != nil {
					t.Errorf("INVARIANT: expense %s without receivables has personal_amount %d", x.ID, *x.PersonalAmount)
				}
				continue
			}
			var owed int64
			for i := range rs {
				owed += rs[i].Amount - rs[i].WrittenOffAmount
			}
			want := x.Amount - owed
			if want < 0 {
				t.Errorf("INVARIANT: expense %s shares exceed amount", x.ID)
			}
			if x.PersonalAmount == nil || *x.PersonalAmount != want {
				t.Errorf("INVARIANT: expense %s personal_amount = %v, want %d", x.ID, x.PersonalAmount, want)
			}
		default:
			if len(recsByTxn[x.ID]) > 0 {
				t.Errorf("INVARIANT: %s transaction %s has receivables", x.Type, x.ID)
			}
		}
	}
	if reimbursements != len(sets) {
		t.Errorf("INVARIANT: %d reimbursements but %d settlements", reimbursements, len(sets))
	}

	// Ledger: balance == initial + sum of live transaction effects.
	expected := map[string]int64{}
	for id, b := range e.initial {
		expected[id] = b
	}
	types := map[string]models.AccountType{}
	for id := range e.initial {
		var a models.Account
		if err := e.db.First(&a, "id = ?", id).Error; err != nil {
			t.Fatalf("load account: %v", err)
		}
		types[id] = a.Type
	}
	for i := range txns {
		x := &txns[i]
		if _, ok := e.initial[x.AccountID]; !ok {
			continue
		}
		if sign, ok := ledgerSign(types[x.AccountID], x.Type); ok {
			expected[x.AccountID] += sign * x.Amount
		}
	}
	for id, want := range expected {
		if got := e.balance(id); got != want {
			t.Errorf("INVARIANT: account %s balance = %d, ledger says %d", id, got, want)
		}
	}
}

// ledgerSign is the test's own oracle for how a transaction moves a balance
// (deliberately not the production balanceDelta): +1 credits the account, -1
// debits it, and liability accounts (credit_card, debt) hold the amount owed so
// their signs are inverted. ok is false for types without a single-leg effect.
func ledgerSign(acct models.AccountType, txn models.TransactionType) (sign int64, ok bool) {
	table := map[models.AccountType]map[models.TransactionType]int64{
		models.AccountTypeCash: {
			models.TransactionTypeIncome:        +1,
			models.TransactionTypeReimbursement: +1,
			models.TransactionTypeExpense:       -1,
		},
		models.AccountTypeCreditCard: {
			models.TransactionTypeIncome:        -1,
			models.TransactionTypeReimbursement: -1,
			models.TransactionTypeExpense:       +1,
		},
		models.AccountTypeDebt: {
			models.TransactionTypeIncome:        -1,
			models.TransactionTypeReimbursement: -1,
			models.TransactionTypeExpense:       +1,
		},
	}
	sign, ok = table[acct][txn]
	return sign, ok
}

// --- input builders ---

func rcpEqual(names ...string) []SharedCounterpartyInput {
	out := make([]SharedCounterpartyInput, len(names))
	for i, n := range names {
		out[i] = SharedCounterpartyInput{Name: n}
	}
	return out
}

func rcpAmt(name string, amount int64) SharedCounterpartyInput {
	return SharedCounterpartyInput{Name: name, Amount: &amount}
}

func i64(v int64) *int64 { return &v }

// in builds a shared-expense input on the env's cash account and category.
func (e *recEnv) in(amount int64, mode ShareMode, kind models.ReceivableKind, cps ...SharedCounterpartyInput) CreateSharedExpenseInput {
	return CreateSharedExpenseInput{
		AccountID:      e.cash.ID,
		CategoryID:     &e.cat.ID,
		Amount:         amount,
		Description:    "dinner",
		Date:           time.Now(),
		Kind:           kind,
		Mode:           mode,
		Counterparties: cps,
	}
}

// --- operation wrappers (each runs check()) ---

func (e *recEnv) create(in CreateSharedExpenseInput) (*models.Transaction, error) {
	e.t.Helper()
	res, err := e.rec.CreateSharedExpense(e.user.ID, in)
	e.check()
	return res, err
}

// mustCreate creates a shared expense or fails the test.
func (e *recEnv) mustCreate(in CreateSharedExpenseInput) *models.Transaction {
	e.t.Helper()
	res, err := e.create(in)
	testutil.AssertNoError(e.t, err)
	return res
}

// split creates a $-split of amount equally with the named counterparties.
func (e *recEnv) split(amount int64, names ...string) *models.Transaction {
	e.t.Helper()
	return e.mustCreate(e.in(amount, ShareModeEqual, models.ReceivableKindSplit, rcpEqual(names...)...))
}

func (e *recEnv) update(txnID string, in UpdateReceivablesInput) (*models.Transaction, error) {
	e.t.Helper()
	res, err := e.rec.UpdateReceivables(e.user.ID, txnID, in)
	e.check()
	return res, err
}

func (e *recEnv) settle(recID, accountID string, amount int64, writeOff bool) (*models.Receivable, *models.ReceivableSettlement, error) {
	e.t.Helper()
	r, s, err := e.rec.Settle(e.user.ID, recID, SettleInput{AccountID: accountID, Amount: amount, WriteOffRemainder: writeOff})
	e.check()
	return r, s, err
}

func (e *recEnv) mustSettle(recID string, amount int64) (*models.Receivable, *models.ReceivableSettlement) {
	e.t.Helper()
	r, s, err := e.settle(recID, e.cash.ID, amount, false)
	testutil.AssertNoError(e.t, err)
	return r, s
}

func (e *recEnv) delSettlement(id string) (*models.Receivable, error) {
	e.t.Helper()
	r, err := e.rec.DeleteSettlement(e.user.ID, id)
	e.check()
	return r, err
}

func (e *recEnv) writeOff(recID string, amount *int64) (*models.Receivable, error) {
	e.t.Helper()
	r, err := e.rec.WriteOff(e.user.ID, recID, amount)
	e.check()
	return r, err
}

func (e *recEnv) undoWriteOff(recID string) (*models.Receivable, error) {
	e.t.Helper()
	r, err := e.rec.UndoWriteOff(e.user.ID, recID)
	e.check()
	return r, err
}

func (e *recEnv) updateTxn(id string, u TransactionUpdateFields) (*models.Transaction, error) {
	e.t.Helper()
	r, err := e.txn.UpdateTransaction(e.user.ID, id, u)
	e.check()
	return r, err
}

func (e *recEnv) deleteTxn(id string) error {
	e.t.Helper()
	err := e.txn.DeleteTransaction(e.user.ID, id)
	e.check()
	return err
}

// get reloads a receivable (with settlements and expense) or fails the test.
func (e *recEnv) get(id string) *models.Receivable {
	e.t.Helper()
	r, err := e.rec.GetReceivable(e.user.ID, id)
	testutil.AssertNoError(e.t, err)
	return r
}

func (e *recEnv) personal(txnID string) *int64 {
	e.t.Helper()
	var x models.Transaction
	if err := e.db.First(&x, "id = ?", txnID).Error; err != nil {
		e.t.Fatalf("load txn: %v", err)
	}
	return x.PersonalAmount
}

func (e *recEnv) wantPersonal(txnID string, want int64) {
	e.t.Helper()
	got := e.personal(txnID)
	if got == nil || *got != want {
		e.t.Errorf("personal_amount = %v, want %d", got, want)
	}
}

func (e *recEnv) wantBalance(accountID string, want int64) {
	e.t.Helper()
	if got := e.balance(accountID); got != want {
		e.t.Errorf("balance = %d, want %d", got, want)
	}
}

func (e *recEnv) wantRec(r *models.Receivable, status models.ReceivableStatus, paid, writtenOff, outstanding int64) {
	e.t.Helper()
	if r.Status != status || r.PaidAmount != paid || r.WrittenOffAmount != writtenOff || r.Outstanding != outstanding {
		e.t.Errorf("receivable = {status:%s paid:%d wo:%d out:%d}, want {%s %d %d %d}",
			r.Status, r.PaidAmount, r.WrittenOffAmount, r.Outstanding, status, paid, writtenOff, outstanding)
	}
}

// otherUser returns a second user with a cash account and category, to prove isolation.
func (e *recEnv) otherUser() (*models.User, *models.Account, *models.Category) {
	e.t.Helper()
	u := testutil.CreateTestUser(e.t, e.db)
	a := testutil.CreateTestCashAccountWithBalance(e.t, e.db, u.ID, 500000)
	c := testutil.CreateTestCategory(e.t, e.db, u.ID, models.CategoryTypeExpense)
	return u, a, c
}

// failOnCreate makes inserts into the given table fail until the returned func is called.
func failOnCreate(t *testing.T, db *gorm.DB, table string) (restore func()) {
	t.Helper()
	name := "test:fail_create_" + table
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("forced failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	return func() {
		if err := db.Callback().Create().Remove(name); err != nil {
			t.Fatalf("remove callback: %v", err)
		}
	}
}

// failOnDelete makes soft deletes of the given table fail until the returned func is called.
func failOnDelete(t *testing.T, db *gorm.DB, table string) (restore func()) {
	t.Helper()
	name := "test:fail_delete_" + table
	if err := db.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("forced failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	return func() {
		if err := db.Callback().Delete().Remove(name); err != nil {
			t.Fatalf("remove callback: %v", err)
		}
	}
}

// assertAppCode asserts err is an AppError with the given code and HTTP status.
func assertAppCode(t *testing.T, err error, code string, status int) {
	t.Helper()
	testutil.AssertAppError(t, err, code)
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) && appErr.StatusCode != status {
		t.Errorf("error %s status = %d, want %d", code, appErr.StatusCode, status)
	}
}

func pageReq() pagination.PageRequest { return pagination.PageRequest{Page: 1, PageSize: 50} }

// mustFirstRuleID returns the id of the user's only rule.
func mustFirstRuleID(t *testing.T, e *recEnv) string {
	t.Helper()
	rules, err := e.rules.ListRules(e.user.ID)
	testutil.AssertNoError(t, err)
	if len(rules) == 0 {
		t.Fatal("no rules")
	}
	return rules[0].ID
}
