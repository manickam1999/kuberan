package services

// Every balance writer must lock the account row inside its DB transaction and
// change the balance with a relative update (plan 021, D10), otherwise it
// overwrites a concurrent change with a stale absolute value. SQLite gives no
// real concurrency, so these tests inject a "concurrent" +bump on an account
// right after the writer's first transactions-row insert, i.e. between the
// writer's reads and its balance write. A writer that still writes an absolute
// balance read before the bump loses it.

import (
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

const concurrentBump int64 = 777

// bumpAfterFirstTxnInsert registers a callback that, once, adds delta to the
// account's balance using the same connection as the running DB transaction.
func bumpAfterFirstTxnInsert(t *testing.T, db *gorm.DB, accountID string, delta int64) (restore func()) {
	t.Helper()
	done := false
	const name = "test:concurrent_bump"
	bump := func(tx *gorm.DB) {
		if done || tx.Error != nil || tx.Statement.Table != "transactions" {
			return
		}
		done = true
		if _, err := tx.Statement.ConnPool.ExecContext(tx.Statement.Context,
			"UPDATE accounts SET balance = balance + ? WHERE id = ?", delta, accountID); err != nil {
			_ = tx.AddError(err)
		}
	}
	testutil.AssertNoError(t, db.Callback().Create().After("gorm:create").Register(name, bump))
	testutil.AssertNoError(t, db.Callback().Update().After("gorm:update").Register(name, bump))
	return func() {
		_ = db.Callback().Create().Remove(name)
		_ = db.Callback().Update().Remove(name)
	}
}

// drainBeforeFirstLock sets the account's balance to newBalance right before the
// first locking read of that account, simulating another writer that committed
// after the caller's up-front validation but before it took the row lock. The
// drain runs inside the writer's own DB transaction, so a failed operation rolls
// it back too: assert on the error and on the absence of side effects, not on
// the balance.
func drainBeforeFirstLock(t *testing.T, db *gorm.DB, accountID string, newBalance int64) (restore func()) {
	t.Helper()
	done := false
	const name = "test:drain_before_lock"
	testutil.AssertNoError(t, db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if done || tx.Statement.Table != "accounts" {
			return
		}
		if _, locking := tx.Statement.Clauses["FOR"]; !locking {
			return
		}
		done = true
		if _, err := tx.Statement.ConnPool.ExecContext(tx.Statement.Context,
			"UPDATE accounts SET balance = ? WHERE id = ?", newBalance, accountID); err != nil {
			_ = tx.AddError(err)
		}
	}))
	return func() { _ = db.Callback().Query().Remove(name) }
}

func accountBalance(t *testing.T, db *gorm.DB, id string) int64 {
	t.Helper()
	var a models.Account
	if err := db.First(&a, "id = ?", id).Error; err != nil {
		t.Fatalf("load account: %v", err)
	}
	return a.Balance
}

func TestBalanceWritersKeepConcurrentChanges(t *testing.T) {
	t.Run("CreateTransaction expense", func(t *testing.T) {
		e := newRecEnv(t)
		defer bumpAfterFirstTxnInsert(t, e.db, e.cash.ID, concurrentBump)()
		_, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 300, "coffee", time.Now())
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart-300+concurrentBump)
	})

	t.Run("CreateTransaction income", func(t *testing.T) {
		e := newRecEnv(t)
		defer bumpAfterFirstTxnInsert(t, e.db, e.cash.ID, concurrentBump)()
		_, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeIncome, 300, "gift", time.Now())
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart+300+concurrentBump)
	})

	t.Run("CreateSharedExpense", func(t *testing.T) {
		e := newRecEnv(t)
		defer bumpAfterFirstTxnInsert(t, e.db, e.cash.ID, concurrentBump)()
		e.initial[e.cash.ID] += concurrentBump // the injected change is part of the ledger
		e.split(900, "Alice")
		e.wantBalance(e.cash.ID, recCashStart-900+concurrentBump)
	})

	t.Run("CreateTransfer both legs", func(t *testing.T) {
		e := newRecEnv(t)
		to := e.track(testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 1000))
		defer bumpAfterFirstTxnInsert(t, e.db, e.cash.ID, concurrentBump)()
		_, err := e.txn.CreateTransfer(e.user.ID, e.cash.ID, to.ID, 400, "move", time.Now())
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart-400+concurrentBump)
		e.wantBalance(to.ID, 1400)
	})

	t.Run("CreateTransfer destination", func(t *testing.T) {
		e := newRecEnv(t)
		to := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 1000)
		defer bumpAfterFirstTxnInsert(t, e.db, to.ID, concurrentBump)()
		_, err := e.txn.CreateTransfer(e.user.ID, e.cash.ID, to.ID, 400, "move", time.Now())
		testutil.AssertNoError(t, err)
		e.wantBalance(to.ID, 1000+400+concurrentBump)
	})

	t.Run("UpdateTransaction", func(t *testing.T) {
		e := newRecEnv(t)
		x, err := e.txn.CreateTransaction(e.user.ID, e.cash.ID, nil, models.TransactionTypeExpense, 300, "coffee", time.Now())
		testutil.AssertNoError(t, err)
		defer bumpAfterFirstTxnInsert(t, e.db, e.cash.ID, concurrentBump)()
		e.initial[e.cash.ID] += concurrentBump // the injected change is part of the ledger
		amount := int64(500)
		_, err = e.updateTxn(x.ID, TransactionUpdateFields{Amount: &amount})
		testutil.AssertNoError(t, err)
		e.wantBalance(e.cash.ID, recCashStart-500+concurrentBump)
	})

	t.Run("RecordBuy with funding account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		invAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		cash := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, invAcct.ID, sec.ID)

		defer bumpAfterFirstTxnInsert(t, db, cash.ID, concurrentBump)()
		_, err := svc.RecordBuy(user.ID, inv.ID, time.Now(), 5.0, 10000, 500, "buy", cash.ID)
		testutil.AssertNoError(t, err)
		if got, want := accountBalance(t, db, cash.ID), int64(1000000-50500)+concurrentBump; got != want {
			t.Errorf("balance = %d, want %d", got, want)
		}
	})

	t.Run("RecordSell with deposit account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		invAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		cash := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, invAcct.ID, sec.ID)

		defer bumpAfterFirstTxnInsert(t, db, cash.ID, concurrentBump)()
		_, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 4.0, 12000, 300, "sell", cash.ID)
		testutil.AssertNoError(t, err)
		if got, want := accountBalance(t, db, cash.ID), int64(1000+4*12000-300)+concurrentBump; got != want {
			t.Errorf("balance = %d, want %d", got, want)
		}
	})

	t.Run("CreateFixedDepositAccount source", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		source := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		defer bumpAfterFirstTxnInsert(t, db, source.ID, concurrentBump)()
		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name: "FD", Currency: "USD", SourceAccountID: source.ID, Principal: 200000, InterestRate: 5,
			StartDate: fdDate(2025, 1, 1), MaturityDate: fdDate(2026, 1, 1),
			PayoutType: models.PayoutTypeCumulative, Compounding: models.CompoundingAnnual,
		})
		testutil.AssertNoError(t, err)
		if got, want := accountBalance(t, db, source.ID), int64(300000)+concurrentBump; got != want {
			t.Errorf("balance = %d, want %d", got, want)
		}
	})

	t.Run("CreateFixedDepositAccount re-checks the balance on the locked row", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		source := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 250000)

		defer drainBeforeFirstLock(t, db, source.ID, 1)()
		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name: "FD", Currency: "USD", SourceAccountID: source.ID, Principal: 200000, InterestRate: 5,
			StartDate: fdDate(2025, 1, 1), MaturityDate: fdDate(2026, 1, 1),
			PayoutType: models.PayoutTypeCumulative, Compounding: models.CompoundingAnnual,
		})
		assertAppCode(t, err, "SOURCE_ACCOUNT_INSUFFICIENT", http.StatusBadRequest)
		var fds int64
		db.Model(&models.Account{}).Where("type = ?", models.AccountTypeFixedDeposit).Count(&fds)
		if fds != 0 {
			t.Errorf("%d fixed deposit accounts left behind, want 0", fds)
		}
	})

	t.Run("CloseFixedDeposit destination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		dest := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
		fd, details := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal: 100000, InterestRate: 5.0, StartDate: fdDate(2023, 1, 1), MaturityDate: fdDate(2024, 1, 1),
			PayoutType: models.PayoutTypeCumulative, Status: models.FixedDepositStatusMatured,
		})
		db.Model(details).Update("maturity_value", 105000)

		defer bumpAfterFirstTxnInsert(t, db, dest.ID, concurrentBump)()
		_, err := svc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: ptrStr(dest.ID)})
		testutil.AssertNoError(t, err)
		if got, want := accountBalance(t, db, dest.ID), int64(105000)+concurrentBump; got != want {
			t.Errorf("balance = %d, want %d", got, want)
		}
	})

	t.Run("CloseFixedDeposit cannot pay out twice", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		dest := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
		fd, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal: 100000, InterestRate: 5.0, StartDate: fdDate(2023, 1, 1), MaturityDate: fdDate(2024, 1, 1),
			PayoutType: models.PayoutTypePeriodic, Status: models.FixedDepositStatusMatured,
		})
		_, err := svc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: ptrStr(dest.ID)})
		testutil.AssertNoError(t, err)
		_, err = svc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: ptrStr(dest.ID)})
		assertAppCode(t, err, "FIXED_DEPOSIT_ALREADY_CLOSED", http.StatusConflict)
		if got := accountBalance(t, db, dest.ID); got != 100000 {
			t.Errorf("dest balance = %d, want 100000", got)
		}
	})
}

func TestCreateTransferChecksBalanceOnLockedRow(t *testing.T) {
	e := newRecEnv(t)
	to := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 0)

	// Another writer drains the source after the request started but before the lock.
	restore := drainBeforeFirstLock(t, e.db, e.cash.ID, 100)
	_, err := e.txn.CreateTransfer(e.user.ID, e.cash.ID, to.ID, 5000, "raced", time.Now())
	restore()
	assertAppCode(t, err, "INSUFFICIENT_BALANCE", http.StatusBadRequest)
	var transfers int64
	e.db.Model(&models.Transaction{}).Where("type = ?", models.TransactionTypeTransfer).Count(&transfers)
	if transfers != 0 {
		t.Errorf("%d transfer rows left behind, want 0", transfers)
	}
	e.wantBalance(to.ID, 0)

	_, err = e.txn.CreateTransfer(e.user.ID, e.cash.ID, to.ID, recCashStart+1, "too much", time.Now())
	assertAppCode(t, err, "INSUFFICIENT_BALANCE", http.StatusBadRequest)
	e.wantBalance(e.cash.ID, recCashStart)
	e.wantBalance(to.ID, 0)

	// Credit-card (liability) sources are exempt from the check.
	card := testutil.CreateTestCreditCardAccount(t, e.db, e.user.ID, 0)
	_, err = e.txn.CreateTransfer(e.user.ID, card.ID, to.ID, 5000, "cash advance", time.Now())
	testutil.AssertNoError(t, err)
	e.wantBalance(card.ID, 5000)
	e.wantBalance(to.ID, 5000)
}
