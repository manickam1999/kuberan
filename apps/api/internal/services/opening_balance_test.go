package services

import (
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// openingBalanceOf returns the opening-balance transaction written for an account.
func openingBalanceOf(t *testing.T, txSvc TransactionServicer, userID, accountID string) *models.Transaction {
	t.Helper()
	typ := models.TransactionTypeOpeningBalance
	page, err := txSvc.GetUserTransactions(userID, pagination.PageRequest{Page: 1, PageSize: 20}, TransactionFilter{AccountID: &accountID, Type: &typ})
	testutil.AssertNoError(t, err)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 opening balance, got %d", len(page.Data))
	}
	return &page.Data[0]
}

func TestOpeningBalance_WrittenOnAccountCreate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)

	acct, err := acctSvc.CreateCashAccount(user.ID, "Everyday", "", "MYR", 842000)
	testutil.AssertNoError(t, err)
	assertBalance(t, acctSvc, user.ID, acct.ID, 842000)

	ob := openingBalanceOf(t, txSvc, user.ID, acct.ID)
	if ob.Amount != 842000 {
		t.Fatalf("expected opening balance 842000, got %d", ob.Amount)
	}
}

func TestOpeningBalance_ExcludedFromIncomeFigures(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)

	acct, err := acctSvc.CreateCashAccount(user.ID, "Everyday", "", "MYR", 842000)
	testutil.AssertNoError(t, err)
	// A real income that happens to share the old opening-balance description
	// must still count: the summaries no longer filter on description.
	_, err = txSvc.CreateTransaction(user.ID, acct.ID, nil, models.TransactionTypeIncome, 5000, "Initial balance", time.Now())
	testutil.AssertNoError(t, err)

	from := time.Now().AddDate(0, 0, -1)
	to := time.Now().AddDate(0, 0, 1)

	cashflow, err := txSvc.GetCashflow(user.ID, from, to)
	testutil.AssertNoError(t, err)
	if cashflow.TotalIncome != 5000 {
		t.Errorf("cash flow income: expected 5000, got %d", cashflow.TotalIncome)
	}

	months, err := txSvc.GetMonthlySummary(user.ID, 1)
	testutil.AssertNoError(t, err)
	if len(months) != 1 || months[0].Income != 5000 {
		t.Errorf("monthly summary income: expected 5000, got %+v", months)
	}

	days, err := txSvc.GetDailySummary(user.ID, from, to)
	testutil.AssertNoError(t, err)
	var dayIncome int64
	for _, d := range days {
		dayIncome += d.Income
	}
	if dayIncome != 5000 {
		t.Errorf("daily summary income: expected 5000, got %d", dayIncome)
	}
}

func TestOpeningBalance_EditAndDelete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)

	acct, err := acctSvc.CreateCashAccount(user.ID, "Everyday", "", "MYR", 10000)
	testutil.AssertNoError(t, err)
	ob := openingBalanceOf(t, txSvc, user.ID, acct.ID)

	amt := int64(25000)
	_, err = txSvc.UpdateTransaction(user.ID, ob.ID, TransactionUpdateFields{Amount: &amt})
	testutil.AssertNoError(t, err)
	assertBalance(t, acctSvc, user.ID, acct.ID, 25000)

	testutil.AssertNoError(t, txSvc.DeleteTransaction(user.ID, ob.ID))
	assertBalance(t, acctSvc, user.ID, acct.ID, 0)
}

func TestOpeningBalance_LiabilityAccountIsNotInverted(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)
	card := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

	// An opening balance on a card is what was already owed: it raises the
	// balance, unlike income, which would lower it.
	testutil.AssertNoError(t, acctSvc.ApplyBalanceDelta(db, card.ID, card.Type, models.TransactionTypeOpeningBalance, 6000))
	ob := testutil.CreateTestTransaction(t, db, user.ID, card.ID, models.TransactionTypeOpeningBalance, 6000)
	assertBalance(t, acctSvc, user.ID, card.ID, 6000)

	testutil.AssertNoError(t, txSvc.DeleteTransaction(user.ID, ob.ID))
	assertBalance(t, acctSvc, user.ID, card.ID, 0)
}

func TestOpeningBalance_Restrictions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)

	acct, err := acctSvc.CreateCashAccount(user.ID, "Everyday", "", "MYR", 10000)
	testutil.AssertNoError(t, err)
	other := testutil.CreateTestCashAccount(t, db, user.ID)
	ob := openingBalanceOf(t, txSvc, user.ID, acct.ID)

	t.Run("cannot be created directly", func(t *testing.T) {
		_, err := txSvc.CreateTransaction(user.ID, acct.ID, nil, models.TransactionTypeOpeningBalance, 100, "", time.Now())
		testutil.AssertAppError(t, err, apperrors.ErrInvalidTransactionType.Code)
	})

	t.Run("type cannot change", func(t *testing.T) {
		income := models.TransactionTypeIncome
		_, err := txSvc.UpdateTransaction(user.ID, ob.ID, TransactionUpdateFields{Type: &income})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidTypeChange.Code)
	})

	t.Run("an income cannot become one", func(t *testing.T) {
		tx, err := txSvc.CreateTransaction(user.ID, acct.ID, nil, models.TransactionTypeIncome, 100, "Pay", time.Now())
		testutil.AssertNoError(t, err)
		opening := models.TransactionTypeOpeningBalance
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &opening})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidTypeChange.Code)
	})

	t.Run("cannot move account", func(t *testing.T) {
		_, err := txSvc.UpdateTransaction(user.ID, ob.ID, TransactionUpdateFields{AccountID: &other.ID})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})

	t.Run("cannot be categorised", func(t *testing.T) {
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)
		catID := &cat.ID
		_, err := txSvc.UpdateTransaction(user.ID, ob.ID, TransactionUpdateFields{CategoryID: &catID})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})

	// None of the rejected edits moved the balance.
	assertBalance(t, acctSvc, user.ID, acct.ID, 10100)
}
