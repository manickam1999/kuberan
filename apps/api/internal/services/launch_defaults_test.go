package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateUser_BaseCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		seed     bool // seed a USD/EUR rate first
		wantCode string
		want     string
	}{
		{name: "omitted keeps the default", currency: "", want: "MYR"},
		{name: "supported currency is stored upper-case", currency: "eur", seed: true, want: "EUR"},
		{name: "USD needs no rate", currency: "USD", want: "USD"},
		{name: "currency without a rate is rejected", currency: "EUR", wantCode: "UNSUPPORTED_CURRENCY"},
		{name: "zero-decimal currency is rejected", currency: "JPY", wantCode: "UNSUPPORTED_CURRENCY"},
		{name: "malformed code is rejected", currency: "E1", wantCode: "UNSUPPORTED_CURRENCY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			fxSvc := NewFXService(db, testFXStaleness)
			svc := NewUserService(db, fxSvc)
			if tt.seed {
				seedRate(t, fxSvc, "USD", "EUR", "0.92", time.Now().Add(-time.Hour))
			}

			user, err := svc.CreateUser("new@example.com", "password123", "", "", WithBaseCurrency(tt.currency))
			if tt.wantCode != "" {
				testutil.AssertAppError(t, err, tt.wantCode)
				var n int64
				db.Model(&models.User{}).Where("email = ?", "new@example.com").Count(&n)
				if n != 0 {
					t.Errorf("a rejected sign-up must not create a user, found %d", n)
				}
				return
			}
			testutil.AssertNoError(t, err)
			if user.BaseCurrency != tt.want {
				t.Errorf("returned base currency = %q, want %q", user.BaseCurrency, tt.want)
			}
			var stored models.User
			if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.BaseCurrency != tt.want {
				t.Errorf("stored base currency = %q, want %q", stored.BaseCurrency, tt.want)
			}
		})
	}
}

func TestCreateAccount_DefaultsToBaseCurrency(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		want     string
		create   func(svc AccountServicer, userID, currency string) (*models.Account, error)
	}{
		{"cash default", "", "EUR", func(s AccountServicer, u, c string) (*models.Account, error) {
			return s.CreateCashAccount(u, "Cash", "", c, 0)
		}},
		{"investment default", "", "EUR", func(s AccountServicer, u, c string) (*models.Account, error) {
			return s.CreateInvestmentAccount(u, "Broker", "", c, "", "")
		}},
		{"credit card default", "", "EUR", func(s AccountServicer, u, c string) (*models.Account, error) {
			return s.CreateCreditCardAccount(u, "Card", "", c, 0, 0, nil, 0)
		}},
		{"cash explicit wins", "USD", "USD", func(s AccountServicer, u, c string) (*models.Account, error) {
			return s.CreateCashAccount(u, "Cash", "", c, 0)
		}},
		{"credit card explicit wins", "GBP", "GBP", func(s AccountServicer, u, c string) (*models.Account, error) {
			return s.CreateCreditCardAccount(u, "Card", "", c, 0, 0, nil, 0)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			user := testutil.CreateTestUser(t, db)
			if err := db.Model(&models.User{}).Where("id = ?", user.ID).Update("base_currency", "EUR").Error; err != nil {
				t.Fatal(err)
			}

			account, err := tt.create(NewAccountService(db), user.ID, tt.explicit)
			testutil.AssertNoError(t, err)
			if account.Currency != tt.want {
				t.Errorf("currency = %q, want %q", account.Currency, tt.want)
			}
		})
	}
}

func TestCreateCreditCardAccount_CurrentBalance(t *testing.T) {
	setup := func(t *testing.T) (AccountServicer, TransactionServicer, *models.User, func()) {
		db := testutil.SetupTestDB(t)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		return acctSvc, txSvc, testutil.CreateTestUser(t, db), func() { testutil.TeardownTestDB(t, db) }
	}

	tests := []struct {
		name        string
		balance     int64
		wantRows    int64
		wantBalance int64
		wantCode    string
	}{
		{name: "zero writes no row", balance: 0, wantRows: 0, wantBalance: 0},
		{name: "positive writes one opening balance", balance: 50000, wantRows: 1, wantBalance: 50000},
		{name: "negative is rejected", balance: -1, wantCode: "INVALID_INPUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acctSvc, txSvc, user, done := setup(t)
			defer done()

			account, err := acctSvc.CreateCreditCardAccount(user.ID, "Visa", "", "", 100000, 0, nil, tt.balance)
			if tt.wantCode != "" {
				testutil.AssertAppError(t, err, tt.wantCode)
				return
			}
			testutil.AssertNoError(t, err)

			got, err := acctSvc.GetAccountByID(user.ID, account.ID)
			testutil.AssertNoError(t, err)
			if got.Balance != tt.wantBalance {
				t.Errorf("balance = %d, want %d", got.Balance, tt.wantBalance)
			}

			txs, err := txSvc.GetAccountTransactions(user.ID, account.ID, pagination.PageRequest{Page: 1, PageSize: 20}, TransactionFilter{})
			testutil.AssertNoError(t, err)
			if int64(len(txs.Data)) != tt.wantRows {
				t.Fatalf("transactions = %d, want %d", len(txs.Data), tt.wantRows)
			}
			if tt.wantRows == 1 {
				row := txs.Data[0]
				if row.Type != models.TransactionTypeOpeningBalance || row.Amount != tt.balance {
					t.Errorf("row = %s %d, want opening_balance %d", row.Type, row.Amount, tt.balance)
				}
				if row.Source != models.TransactionSourceSystem {
					t.Errorf("source = %q, want system", row.Source)
				}
			}

			// An opening balance is never income.
			summary, err := txSvc.GetMonthlySummary(user.ID, 1)
			testutil.AssertNoError(t, err)
			for _, m := range summary {
				if m.Income != 0 || m.Expenses != 0 {
					t.Errorf("opening balance leaked into the summary: income=%d expenses=%d", m.Income, m.Expenses)
				}
			}
		})
	}
}

func TestUpdateTransaction_CategoryTypeCheck(t *testing.T) {
	tests := []struct {
		name     string
		txType   models.TransactionType
		catType  models.CategoryType
		wantCode string
	}{
		{"expense with expense category", models.TransactionTypeExpense, models.CategoryTypeExpense, ""},
		{"income with income category", models.TransactionTypeIncome, models.CategoryTypeIncome, ""},
		{"expense with income category", models.TransactionTypeExpense, models.CategoryTypeIncome, "CATEGORY_TYPE_MISMATCH"},
		{"income with expense category", models.TransactionTypeIncome, models.CategoryTypeExpense, "CATEGORY_TYPE_MISMATCH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			acctSvc := NewAccountService(db)
			txSvc := NewTransactionService(db, acctSvc, nil)
			user := testutil.CreateTestUser(t, db)
			account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
			cat := testutil.CreateTestCategory(t, db, user.ID, tt.catType)
			tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, tt.txType, 1000, "x", time.Now(), TransactionMeta{})
			testutil.AssertNoError(t, err)

			catID := &cat.ID
			_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{CategoryID: &catID})
			if tt.wantCode != "" {
				testutil.AssertAppError(t, err, tt.wantCode)
				got, gerr := txSvc.GetTransactionByID(user.ID, tx.ID)
				testutil.AssertNoError(t, gerr)
				if got.CategoryID != nil {
					t.Error("a rejected update must not set the category")
				}
				return
			}
			testutil.AssertNoError(t, err)
		})
	}

	t.Run("clearing the category and unrelated edits skip the check", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "x", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)

		var none *string
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{CategoryID: &none})
		testutil.AssertNoError(t, err)
		desc := "renamed"
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Description: &desc})
		testutil.AssertNoError(t, err)
	})

	t.Run("a type change must fit the category it keeps", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		expenseCat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		incomeCat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &expenseCat.ID, models.TransactionTypeExpense, 1000, "x", time.Now(), TransactionMeta{})
		testutil.AssertNoError(t, err)

		income := models.TransactionTypeIncome
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &income})
		testutil.AssertAppError(t, err, "CATEGORY_TYPE_MISMATCH")

		// Changing both together is fine.
		incomeID := &incomeCat.ID
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &income, CategoryID: &incomeID})
		testutil.AssertNoError(t, err)
	})
}

func TestGetUserAccounts_IncludeInactive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)
	other := testutil.CreateTestUser(t, db)
	active := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100)
	inactive := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 200)
	testutil.CreateTestCashAccountWithBalance(t, db, other.ID, 300)
	if err := db.Model(&models.Account{}).Where("id = ?", inactive.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}

	ids := func(opts ...AccountListOptions) map[string]bool {
		res, err := svc.GetUserAccounts(user.ID, pagination.PageRequest{Page: 1, PageSize: 100}, opts...)
		testutil.AssertNoError(t, err)
		out := map[string]bool{}
		for _, a := range res.Data {
			out[a.ID] = true
		}
		return out
	}

	def := ids()
	if !def[active.ID] || def[inactive.ID] || len(def) != 1 {
		t.Errorf("default list = %v, want only the active account", def)
	}
	all := ids(AccountListOptions{IncludeInactive: true})
	if !all[active.ID] || !all[inactive.ID] || len(all) != 2 {
		t.Errorf("include_inactive list = %v, want both of the user's accounts and no one else's", all)
	}
}
