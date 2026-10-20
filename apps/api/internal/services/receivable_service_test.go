package services

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// monthRange returns the first and last instant of the current UTC month.
func monthRange() (from, to time.Time) {
	now := time.Now().UTC()
	from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 1, 0).Add(-time.Second)
	return from, to
}

// TestSharedExpense_AcceptanceCriteria is the plan's acceptance test: a $60
// expense split three ways costs the account $60 but counts $20 everywhere, and
// a $20 repayment credits the account without becoming income.
func TestSharedExpense_AcceptanceCriteria(t *testing.T) {
	e := newRecEnv(t)
	from, to := monthRange()
	budgetSvc := NewBudgetService(e.db)
	budget, err := budgetSvc.CreateBudget(e.user.ID, e.cat.ID, "Food", 50000, models.BudgetPeriodMonthly)
	testutil.AssertNoError(t, err)

	expense := e.split(6000, "Alex", "Sam")

	// Account: -$60 immediately.
	e.wantBalance(e.cash.ID, recCashStart-6000)
	e.wantPersonal(expense.ID, 2000)
	if expense.PersonalAmount == nil || *expense.PersonalAmount != 2000 || len(expense.Receivables) != 2 {
		t.Fatalf("returned expense = %+v", expense)
	}

	t.Run("spending by category", func(t *testing.T) {
		res, err := e.txn.GetSpendingByCategory(e.user.ID, from, to)
		testutil.AssertNoError(t, err)
		if res.TotalSpent != 2000 || len(res.Items) != 1 || res.Items[0].Total != 2000 {
			t.Errorf("spending = %+v, want 2000", res)
		}
	})
	t.Run("cashflow", func(t *testing.T) {
		res, err := e.txn.GetCashflow(e.user.ID, from, to)
		testutil.AssertNoError(t, err)
		if res.TotalExpenses != 2000 || res.TotalIncome != 0 {
			t.Errorf("cashflow = %d expenses / %d income, want 2000 / 0", res.TotalExpenses, res.TotalIncome)
		}
	})
	t.Run("monthly summary", func(t *testing.T) {
		items, err := e.txn.GetMonthlySummary(e.user.ID, 1)
		testutil.AssertNoError(t, err)
		if len(items) != 1 || items[0].Expenses != 2000 {
			t.Errorf("monthly = %+v, want expenses 2000", items)
		}
	})
	t.Run("budget progress", func(t *testing.T) {
		p, err := budgetSvc.GetBudgetProgress(e.user.ID, budget.ID)
		testutil.AssertNoError(t, err)
		if p.Spent != 2000 {
			t.Errorf("budget spent = %d, want 2000", p.Spent)
		}
	})
	t.Run("top expenses", func(t *testing.T) {
		res, err := e.txn.GetTopExpenses(e.user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)
		if len(res.Items) != 1 || res.Items[0].Amount != 6000 || res.Items[0].PersonalAmount == nil || *res.Items[0].PersonalAmount != 2000 {
			t.Errorf("top expenses = %+v", res.Items)
		}
	})

	// Repay $20.
	r := expense.Receivables[0]
	if r.Outstanding != 2000 || r.Status != models.ReceivableStatusPending {
		t.Fatalf("new receivable = %+v", r)
	}
	settled, settlement := e.mustSettle(r.ID, 2000)
	e.wantRec(settled, models.ReceivableStatusSettled, 2000, 0, 0)
	e.wantBalance(e.cash.ID, recCashStart-6000+2000)
	if settlement.AccountID != e.cash.ID || settlement.Amount != 2000 {
		t.Errorf("settlement = %+v", settlement)
	}
	e.wantPersonal(expense.ID, 2000) // a repayment never changes my share

	items, err := e.txn.GetMonthlySummary(e.user.ID, 1)
	testutil.AssertNoError(t, err)
	if items[0].Income != 0 || items[0].Expenses != 2000 {
		t.Errorf("after repayment monthly = %+v, want income 0 expenses 2000", items[0])
	}
	cf, err := e.txn.GetCashflow(e.user.ID, from, to)
	testutil.AssertNoError(t, err)
	if cf.TotalIncome != 0 {
		t.Errorf("cashflow income = %d, want 0", cf.TotalIncome)
	}
}

func TestCreateSharedExpense_Rounding(t *testing.T) {
	tests := []struct {
		name        string
		amount      int64
		names       []string
		wantShares  []int64
		wantPersona int64
	}{
		{"1000 with 2 others", 1000, []string{"A", "B"}, []int64{333, 333}, 334},
		{"even split", 6000, []string{"A", "B"}, []int64{2000, 2000}, 2000},
		{"one other, odd cents", 101, []string{"A"}, []int64{50}, 51},
		{"four others", 1000, []string{"A", "B", "C", "D"}, []int64{200, 200, 200, 200}, 200},
		{"smallest valid", 3, []string{"A", "B"}, []int64{1, 1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRecEnv(t)
			expense := e.split(tt.amount, tt.names...)
			if len(expense.Receivables) != len(tt.wantShares) {
				t.Fatalf("receivables = %d, want %d", len(expense.Receivables), len(tt.wantShares))
			}
			for i, r := range expense.Receivables {
				if r.Amount != tt.wantShares[i] || r.CounterpartyName != tt.names[i] {
					t.Errorf("receivable %d = %s/%d, want %s/%d", i, r.CounterpartyName, r.Amount, tt.names[i], tt.wantShares[i])
				}
			}
			e.wantPersonal(expense.ID, tt.wantPersona)
			e.wantBalance(e.cash.ID, recCashStart-tt.amount)
		})
	}
}

func TestCreateSharedExpense_Validation(t *testing.T) {
	long := strings.Repeat("x", 101)
	many := make([]SharedCounterpartyInput, 21)
	for i := range many {
		many[i] = SharedCounterpartyInput{Name: fmt.Sprintf("P%d", i)}
	}
	twenty := many[:20]

	tests := []struct {
		name   string
		mutate func(e *recEnv, in *CreateSharedExpenseInput)
		code   string
	}{
		{"zero amount", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Amount = 0 }, "INVALID_INPUT"},
		{"missing account", func(_ *recEnv, in *CreateSharedExpenseInput) { in.AccountID = "" }, "INVALID_INPUT"},
		{"invalid kind", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Kind = "gift" }, "INVALID_RECEIVABLE_KIND"},
		{"empty kind", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Kind = "" }, "INVALID_RECEIVABLE_KIND"},
		{"invalid mode", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Mode = "weird" }, "INVALID_INPUT"},
		{"no counterparties", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Counterparties = nil }, "INVALID_SHARES"},
		{"21 counterparties", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Counterparties = many }, "INVALID_SHARES"},
		{"duplicate keys", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Counterparties = rcpEqual("Alex", "  ALEX ") }, "INVALID_SHARES"},
		{"blank name", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Counterparties = rcpEqual("   ") }, "INVALID_SHARES"},
		{"name too long", func(_ *recEnv, in *CreateSharedExpenseInput) { in.Counterparties = rcpEqual(long) }, "INVALID_SHARES"},
		{"equal with explicit amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", 100)}
		}, "INVALID_SHARES"},
		{"equal share rounds to zero", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Amount = 2
			in.Counterparties = rcpEqual("A", "B")
		}, "INVALID_SHARES"},
		{"custom missing amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeCustom
			in.Counterparties = rcpEqual("A")
		}, "INVALID_SHARES"},
		{"custom zero amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeCustom
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", 0)}
		}, "INVALID_SHARES"},
		{"custom negative amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeCustom
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", -5)}
		}, "INVALID_SHARES"},
		{"custom sum exceeds amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeCustom
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", 3000), rcpAmt("B", 3001)}
		}, "INVALID_SHARES"},
		{"custom single share above amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeCustom
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", 6001)}
		}, "INVALID_SHARES"},
		{"full with two counterparties", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeFull
			in.Counterparties = rcpEqual("A", "B")
		}, "INVALID_SHARES"},
		{"full with explicit amount", func(_ *recEnv, in *CreateSharedExpenseInput) {
			in.Mode = ShareModeFull
			in.Counterparties = []SharedCounterpartyInput{rcpAmt("A", 6000)}
		}, "INVALID_SHARES"},
		{"unknown account", func(_ *recEnv, in *CreateSharedExpenseInput) { in.AccountID = "no-such-account" }, "ACCOUNT_NOT_FOUND"},
		{"other user's account", func(e *recEnv, in *CreateSharedExpenseInput) {
			_, a, _ := e.otherUser()
			in.AccountID = a.ID
		}, "ACCOUNT_NOT_FOUND"},
		{"other user's category", func(e *recEnv, in *CreateSharedExpenseInput) {
			_, _, c := e.otherUser()
			in.CategoryID = &c.ID
		}, "CATEGORY_NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRecEnv(t)
			in := e.in(6000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alex", "Sam")...)
			tt.mutate(e, &in)
			_, err := e.create(in)
			testutil.AssertAppError(t, err, tt.code)
			// Nothing may have been written.
			var n int64
			e.db.Model(&models.Transaction{}).Where("user_id = ?", e.user.ID).Count(&n)
			if n != 0 {
				t.Errorf("%d transactions written by a rejected request", n)
			}
			e.wantBalance(e.cash.ID, recCashStart)
		})
	}

	t.Run("20 counterparties accepted", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(2100, ShareModeEqual, models.ReceivableKindSplit, twenty...))
		if len(x.Receivables) != 20 {
			t.Errorf("receivables = %d", len(x.Receivables))
		}
		e.wantPersonal(x.ID, 2100-20*100)
	})
	t.Run("custom sum equal to amount leaves personal 0", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(6000, ShareModeCustom, models.ReceivableKindClaim, rcpAmt("A", 4000), rcpAmt("B", 2000)))
		e.wantPersonal(x.ID, 0)
	})
	t.Run("names are normalized", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(300, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("  Alex   Tan ")...))
		if r := x.Receivables[0]; r.CounterpartyName != "Alex Tan" || r.CounterpartyKey != "alex tan" {
			t.Errorf("name/key = %q/%q", r.CounterpartyName, r.CounterpartyKey)
		}
	})
	t.Run("date defaults to now and description is kept", func(t *testing.T) {
		e := newRecEnv(t)
		in := e.in(300, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("A")...)
		in.Date = time.Time{}
		x := e.mustCreate(in)
		if time.Since(x.Date) > time.Minute || x.Description != "dinner" {
			t.Errorf("date/description = %v / %q", x.Date, x.Description)
		}
	})
}

func TestCreateSharedExpense_FullMode(t *testing.T) {
	kinds := []models.ReceivableKind{
		models.ReceivableKindOnBehalf, models.ReceivableKindClaim, models.ReceivableKindLoan,
		models.ReceivableKindDeposit, models.ReceivableKindRefund,
	}
	from, to := monthRange()
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			e := newRecEnv(t)
			in := e.in(50000, ShareModeFull, kind, rcpEqual("Alex")...)
			in.CategoryID = nil
			x := e.mustCreate(in)

			e.wantBalance(e.cash.ID, recCashStart-50000)
			e.wantPersonal(x.ID, 0)
			if x.CategoryID != nil {
				t.Errorf("full mode must leave the category NULL, got %v", *x.CategoryID)
			}
			if r := x.Receivables[0]; r.Amount != 50000 || r.Kind != kind || r.Outstanding != 50000 {
				t.Errorf("receivable = %+v", r)
			}
			spend, err := e.txn.GetSpendingByCategory(e.user.ID, from, to)
			testutil.AssertNoError(t, err)
			if spend.TotalSpent != 0 {
				t.Errorf("spending = %d, want 0 for a $0 share", spend.TotalSpent)
			}
			top, err := e.txn.GetTopExpenses(e.user.ID, from, to, 10, nil)
			testutil.AssertNoError(t, err)
			if len(top.Items) != 0 {
				t.Errorf("top expenses should exclude a $0 share, got %+v", top.Items)
			}
		})
	}

	t.Run("an explicit category is kept", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(1000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
		if x.CategoryID == nil || *x.CategoryID != e.cat.ID {
			t.Errorf("category = %v, want %s", x.CategoryID, e.cat.ID)
		}
	})
}

func TestCreateSharedExpense_RuleAutoCategorization(t *testing.T) {
	e := newRecEnv(t)
	ruleCat := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
	_, err := e.rules.CreateRule(e.user.ID, CreateRuleInput{
		Name:       "dinner",
		Conditions: []RuleConditionInput{descContains("dinner")},
		Actions:    []RuleActionInput{setCat(ruleCat.ID)},
	})
	testutil.AssertNoError(t, err)

	t.Run("equal mode resolves the rule", func(t *testing.T) {
		in := e.in(3000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("A", "B")...)
		in.CategoryID = nil
		x := e.mustCreate(in)
		if x.CategoryID == nil || *x.CategoryID != ruleCat.ID {
			t.Errorf("category = %v, want rule category %s", x.CategoryID, ruleCat.ID)
		}
	})
	t.Run("custom mode resolves the rule", func(t *testing.T) {
		in := e.in(3000, ShareModeCustom, models.ReceivableKindSplit, rcpAmt("A", 1000))
		in.CategoryID = nil
		x := e.mustCreate(in)
		if x.CategoryID == nil || *x.CategoryID != ruleCat.ID {
			t.Errorf("category = %v, want rule category %s", x.CategoryID, ruleCat.ID)
		}
	})
	t.Run("full mode never auto-categorizes", func(t *testing.T) {
		in := e.in(3000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("A")...)
		in.CategoryID = nil
		x := e.mustCreate(in)
		if x.CategoryID != nil {
			t.Errorf("category = %v, want nil", *x.CategoryID)
		}
	})
	t.Run("explicit category wins", func(t *testing.T) {
		x := e.mustCreate(e.in(3000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("A", "B")...))
		if x.CategoryID == nil || *x.CategoryID != e.cat.ID {
			t.Errorf("category = %v, want explicit %s", x.CategoryID, e.cat.ID)
		}
	})
}

func TestSettle_PartialPaymentsAndStatus(t *testing.T) {
	e := newRecEnv(t)
	x := e.mustCreate(e.in(10000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
	id := x.Receivables[0].ID

	steps := []struct {
		pay         int64
		status      models.ReceivableStatus
		paid, owed  int64
		wantBalance int64
	}{
		{2500, models.ReceivableStatusPartiallyPaid, 2500, 7500, recCashStart - 10000 + 2500},
		{2500, models.ReceivableStatusPartiallyPaid, 5000, 5000, recCashStart - 10000 + 5000},
		{5000, models.ReceivableStatusSettled, 10000, 0, recCashStart},
	}
	for i, s := range steps {
		r, _ := e.mustSettle(id, s.pay)
		e.wantRec(r, s.status, s.paid, 0, s.owed)
		e.wantBalance(e.cash.ID, s.wantBalance)
		e.wantPersonal(x.ID, 0)
		t.Logf("step %d ok", i)
	}

	t.Run("settled receivable cannot be paid again", func(t *testing.T) {
		_, _, err := e.settle(id, e.cash.ID, 1, false)
		assertAppCode(t, err, "SETTLEMENT_EXCEEDS_OUTSTANDING", http.StatusBadRequest)
		e.wantBalance(e.cash.ID, recCashStart)
	})
}

func TestSettle_Validation(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		code   string
	}{
		{"zero", 0, "SETTLEMENT_EXCEEDS_OUTSTANDING"},
		{"negative", -1, "SETTLEMENT_EXCEEDS_OUTSTANDING"},
		{"overpay by one cent", 2001, "SETTLEMENT_EXCEEDS_OUTSTANDING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRecEnv(t)
			x := e.split(6000, "Alex", "Sam")
			_, _, err := e.settle(x.Receivables[0].ID, e.cash.ID, tt.amount, false)
			assertAppCode(t, err, tt.code, http.StatusBadRequest)
			e.wantBalance(e.cash.ID, recCashStart-6000)
			var n int64
			e.db.Model(&models.ReceivableSettlement{}).Count(&n)
			if n != 0 {
				t.Errorf("settlement rows = %d", n)
			}
		})
	}

	t.Run("exactly the outstanding amount succeeds", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		r, _ := e.mustSettle(x.Receivables[0].ID, 2000)
		e.wantRec(r, models.ReceivableStatusSettled, 2000, 0, 0)
	})
	t.Run("missing account", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, _, err := e.settle(x.Receivables[0].ID, "", 100, false)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})
	t.Run("unknown receivable", func(t *testing.T) {
		e := newRecEnv(t)
		_, _, err := e.settle("missing", e.cash.ID, 100, false)
		assertAppCode(t, err, "RECEIVABLE_NOT_FOUND", http.StatusNotFound)
	})
	t.Run("inactive account is not found", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		inactive := e.track(testutil.CreateTestCashAccount(t, e.db, e.user.ID))
		e.db.Model(inactive).Update("is_active", false)
		_, _, err := e.settle(x.Receivables[0].ID, inactive.ID, 100, false)
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
}

func TestSettle_TargetAccountTypes(t *testing.T) {
	t.Run("cash credits the balance", func(t *testing.T) {
		e := newRecEnv(t)
		other := e.track(testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 100))
		x := e.split(6000, "Alex", "Sam")
		_, _, err := e.settle(x.Receivables[0].ID, other.ID, 2000, false)
		testutil.AssertNoError(t, err)
		e.wantBalance(other.ID, 2100)
		e.wantBalance(e.cash.ID, recCashStart-6000)
	})
	t.Run("credit card: the balance owed decreases", func(t *testing.T) {
		e := newRecEnv(t)
		cc := e.track(testutil.CreateTestCreditCardAccount(t, e.db, e.user.ID, 5000))
		x := e.split(6000, "Alex", "Sam")
		_, s, err := e.settle(x.Receivables[0].ID, cc.ID, 2000, false)
		testutil.AssertNoError(t, err)
		e.wantBalance(cc.ID, 3000)
		_, err = e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
		e.wantBalance(cc.ID, 5000)
	})
	t.Run("debt: the balance owed decreases", func(t *testing.T) {
		e := newRecEnv(t)
		debt := e.track(testutil.CreateTestDebtAccount(t, e.db, e.user.ID, 9000))
		x := e.split(6000, "Alex", "Sam")
		_, s, err := e.settle(x.Receivables[0].ID, debt.ID, 2000, false)
		testutil.AssertNoError(t, err)
		e.wantBalance(debt.ID, 7000)
		_, err = e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
		e.wantBalance(debt.ID, 9000)
	})
	t.Run("expense on a credit card raises the balance owed", func(t *testing.T) {
		e := newRecEnv(t)
		cc := e.track(testutil.CreateTestCreditCardAccount(t, e.db, e.user.ID, 0))
		in := e.in(6000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alex", "Sam")...)
		in.AccountID = cc.ID
		e.mustCreate(in)
		e.wantBalance(cc.ID, 6000)
	})

	rejected := map[string]*models.Account{}
	t.Run("investment and fixed deposit are rejected", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		rejected["investment"] = testutil.CreateTestInvestmentAccount(t, e.db, e.user.ID)
		fd := &models.Account{UserID: e.user.ID, Name: "FD", Type: models.AccountTypeFixedDeposit, Currency: "USD", IsActive: true, Balance: 100000}
		testutil.AssertNoError(t, e.db.Create(fd).Error)
		rejected["fixed_deposit"] = fd
		for name, acct := range rejected {
			e.track(acct)
			_, _, err := e.settle(x.Receivables[0].ID, acct.ID, 100, false)
			assertAppCode(t, err, "INVALID_SETTLEMENT_ACCOUNT", http.StatusBadRequest)
			t.Logf("%s rejected", name)
			e.wantBalance(acct.ID, e.initial[acct.ID])
		}
	})
}

func TestSettle_Details(t *testing.T) {
	t.Run("default description, null category, date defaults to now", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		var rt models.Transaction
		testutil.AssertNoError(t, e.db.First(&rt, "id = ?", s.TransactionID).Error)
		if rt.Description != "Repayment from Alex: dinner" || rt.CategoryID != nil || rt.Type != models.TransactionTypeReimbursement {
			t.Errorf("reimbursement = %+v", rt)
		}
		if time.Since(rt.Date) > time.Minute || time.Since(s.Date) > time.Minute {
			t.Errorf("dates = %v / %v, want now", rt.Date, s.Date)
		}
	})
	t.Run("explicit date and description", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		when := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		_, s, err := e.rec.Settle(e.user.ID, x.Receivables[0].ID, SettleInput{AccountID: e.cash.ID, Amount: 500, Date: when, Description: "venmo"})
		testutil.AssertNoError(t, err)
		e.check()
		var rt models.Transaction
		testutil.AssertNoError(t, e.db.First(&rt, "id = ?", s.TransactionID).Error)
		if rt.Description != "venmo" || !rt.Date.Equal(when) || !s.Date.Equal(when) {
			t.Errorf("reimbursement = %+v", rt)
		}
	})
	t.Run("description is truncated to 500 characters", func(t *testing.T) {
		e := newRecEnv(t)
		in := e.in(6000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alex", "Sam")...)
		in.Description = strings.Repeat("é", 600)
		x := e.mustCreate(in)
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		var rt models.Transaction
		testutil.AssertNoError(t, e.db.First(&rt, "id = ?", s.TransactionID).Error)
		if n := len([]rune(rt.Description)); n != 500 {
			t.Errorf("description length = %d runes, want 500", n)
		}
		if !strings.HasPrefix(rt.Description, "Repayment from Alex: éé") {
			t.Errorf("description = %.40q", rt.Description)
		}
	})
	t.Run("empty expense description has no dangling separator", func(t *testing.T) {
		e := newRecEnv(t)
		in := e.in(6000, ShareModeEqual, models.ReceivableKindSplit, rcpEqual("Alex", "Sam")...)
		in.Description = ""
		x := e.mustCreate(in)
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		var rt models.Transaction
		testutil.AssertNoError(t, e.db.First(&rt, "id = ?", s.TransactionID).Error)
		if rt.Description != "Repayment from Alex" {
			t.Errorf("description = %q", rt.Description)
		}
	})
	t.Run("reimbursements are never auto-categorized by rules (D12)", func(t *testing.T) {
		e := newRecEnv(t)
		ruleCat := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
		_, err := e.rules.CreateRule(e.user.ID, CreateRuleInput{
			Name: "repayments", Conditions: []RuleConditionInput{descContains("Repayment")}, Actions: []RuleActionInput{setCat(ruleCat.ID)},
		})
		testutil.AssertNoError(t, err)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		_, err = e.txn.ApplyRule(e.user.ID, mustFirstRuleID(t, e), ApplyRuleOptions{Scope: RuleApplyScopeAll, Overwrite: true})
		testutil.AssertNoError(t, err)
		var rt models.Transaction
		testutil.AssertNoError(t, e.db.First(&rt, "id = ?", s.TransactionID).Error)
		if rt.CategoryID != nil {
			t.Errorf("reimbursement was categorized: %v", *rt.CategoryID)
		}
		e.check()
	})
	t.Run("reimbursement filter lists repayments", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		e.mustSettle(x.Receivables[0].ID, 500)
		typ := models.TransactionTypeReimbursement
		page, err := e.txn.GetUserTransactions(e.user.ID, pageReq(), TransactionFilter{Type: &typ})
		testutil.AssertNoError(t, err)
		if page.TotalItems != 1 || page.Data[0].Type != models.TransactionTypeReimbursement || page.Data[0].Amount != 500 {
			t.Errorf("reimbursement list = %+v", page)
		}
	})
}

func TestWriteOff(t *testing.T) {
	t.Run("deposit returned in part with write_off_remainder", func(t *testing.T) {
		e := newRecEnv(t)
		from, to := monthRange()
		in := e.in(100000, ShareModeFull, models.ReceivableKindDeposit, rcpEqual("Landlord")...)
		in.CategoryID = &e.cat.ID
		x := e.mustCreate(in)
		e.wantPersonal(x.ID, 0)

		r, _, err := e.settle(x.Receivables[0].ID, e.cash.ID, 85000, true)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusWrittenOff, 85000, 15000, 0)
		e.wantPersonal(x.ID, 15000)
		e.wantBalance(e.cash.ID, recCashStart-100000+85000)

		// $150 now counts as spending in the expense's month, and nothing is owed.
		spend, err := e.txn.GetSpendingByCategory(e.user.ID, from, to)
		testutil.AssertNoError(t, err)
		if spend.TotalSpent != 15000 {
			t.Errorf("spending = %d, want 15000", spend.TotalSpent)
		}
		sum, err := e.rec.Summary(e.user.ID)
		testutil.AssertNoError(t, err)
		if sum.TotalOutstanding != 0 || len(sum.ByCounterparty) != 0 {
			t.Errorf("summary = %+v, want empty", sum)
		}
		top, err := e.txn.GetTopExpenses(e.user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)
		if len(top.Items) != 1 {
			t.Errorf("a written-off expense now has a non-zero share and must be listed, got %+v", top.Items)
		}

		// Undo restores the pre-write-off state.
		r, err = e.undoWriteOff(x.Receivables[0].ID)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusPartiallyPaid, 85000, 0, 15000)
		e.wantPersonal(x.ID, 0)
	})

	t.Run("standalone write-off", func(t *testing.T) {
		tests := []struct {
			name     string
			amount   *int64
			wantOff  int64
			wantStat models.ReceivableStatus
			wantOut  int64
		}{
			{"whole balance by default", nil, 5000, models.ReceivableStatusWrittenOff, 0},
			{"partial", i64(1500), 1500, models.ReceivableStatusPartiallyPaid, 3500},
			{"exact outstanding", i64(5000), 5000, models.ReceivableStatusWrittenOff, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := newRecEnv(t)
				x := e.mustCreate(e.in(5000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
				r, err := e.writeOff(x.Receivables[0].ID, tt.amount)
				testutil.AssertNoError(t, err)
				e.wantRec(r, tt.wantStat, 0, tt.wantOff, tt.wantOut)
				e.wantPersonal(x.ID, tt.wantOff)
				e.wantBalance(e.cash.ID, recCashStart-5000) // a write-off never moves money
			})
		}
	})

	t.Run("rejections", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(5000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
		id := x.Receivables[0].ID
		for _, amt := range []int64{5001, 0, -3} {
			_, err := e.writeOff(id, i64(amt))
			assertAppCode(t, err, "WRITE_OFF_EXCEEDS_OUTSTANDING", http.StatusBadRequest)
		}
		_, err := e.undoWriteOff(id)
		assertAppCode(t, err, "INVALID_INPUT", http.StatusBadRequest) // nothing to undo

		e.mustSettle(id, 5000)
		_, err = e.writeOff(id, nil)
		assertAppCode(t, err, "WRITE_OFF_EXCEEDS_OUTSTANDING", http.StatusBadRequest) // nothing outstanding
		e.wantPersonal(x.ID, 0)
	})

	t.Run("write-off is not an obstacle to deleting the expense, a settlement is", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(5000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Alex")...))
		_, err := e.writeOff(x.Receivables[0].ID, nil)
		testutil.AssertNoError(t, err)
		testutil.AssertNoError(t, e.deleteTxn(x.ID))
		e.wantBalance(e.cash.ID, recCashStart)

		y := e.mustCreate(e.in(5000, ShareModeFull, models.ReceivableKindLoan, rcpEqual("Sam")...))
		e.mustSettle(y.Receivables[0].ID, 1000)
		_, err = e.writeOff(y.Receivables[0].ID, nil)
		testutil.AssertNoError(t, err)
		assertAppCode(t, e.deleteTxn(y.ID), "RECEIVABLE_HAS_SETTLEMENTS", http.StatusConflict)
	})

	t.Run("deleting a settlement after a write-off keeps the write-off", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.mustCreate(e.in(10000, ShareModeFull, models.ReceivableKindDeposit, rcpEqual("Landlord")...))
		_, s, err := e.settle(x.Receivables[0].ID, e.cash.ID, 8000, true)
		testutil.AssertNoError(t, err)
		r, err := e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusPartiallyPaid, 0, 2000, 8000)
		e.wantPersonal(x.ID, 2000)
		e.wantBalance(e.cash.ID, recCashStart-10000)
	})
}

func TestDeleteSettlement(t *testing.T) {
	t.Run("reverses the balance exactly once", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		id := x.Receivables[0].ID
		_, s1 := e.mustSettle(id, 500)
		_, s2 := e.mustSettle(id, 1500)
		e.wantBalance(e.cash.ID, recCashStart-6000+2000)

		r, err := e.delSettlement(s1.ID)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusPartiallyPaid, 1500, 0, 500)
		e.wantBalance(e.cash.ID, recCashStart-6000+1500) // -500, not -1000 (no double debit)

		// The reimbursement transaction is gone.
		var n int64
		e.db.Model(&models.Transaction{}).Where("id = ?", s1.TransactionID).Count(&n)
		if n != 0 {
			t.Error("reimbursement transaction still live")
		}

		// A second delete of the same settlement fails and changes nothing.
		_, err = e.delSettlement(s1.ID)
		assertAppCode(t, err, "SETTLEMENT_NOT_FOUND", http.StatusNotFound)
		e.wantBalance(e.cash.ID, recCashStart-6000+1500)

		r, err = e.delSettlement(s2.ID)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusPending, 0, 0, 2000)
		e.wantBalance(e.cash.ID, recCashStart-6000)
		e.wantPersonal(x.ID, 2000)
	})
	t.Run("a settled receivable reopens", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 2000)
		r, err := e.delSettlement(s.ID)
		testutil.AssertNoError(t, err)
		e.wantRec(r, models.ReceivableStatusPending, 0, 0, 2000)
	})
	t.Run("unknown settlement", func(t *testing.T) {
		e := newRecEnv(t)
		_, err := e.delSettlement("missing")
		assertAppCode(t, err, "SETTLEMENT_NOT_FOUND", http.StatusNotFound)
	})
	t.Run("the reimbursement cannot be deleted or edited directly", func(t *testing.T) {
		e := newRecEnv(t)
		x := e.split(6000, "Alex", "Sam")
		_, s := e.mustSettle(x.Receivables[0].ID, 500)
		assertAppCode(t, e.deleteTxn(s.TransactionID), "TRANSACTION_NOT_EDITABLE", http.StatusBadRequest)
		_, err := e.updateTxn(s.TransactionID, TransactionUpdateFields{Amount: i64(900)})
		assertAppCode(t, err, "TRANSACTION_NOT_EDITABLE", http.StatusBadRequest)
		e.wantBalance(e.cash.ID, recCashStart-6000+500)
	})
}

// TestNetWorthUnaffectedByReceivables checks D4: net worth follows account
// balances only, so it drops by the full amount and recovers as repayments arrive.
func TestNetWorthUnaffectedByReceivables(t *testing.T) {
	e := newRecEnv(t)
	snap := NewPortfolioSnapshotService(e.db)
	netWorth := func() int64 {
		t.Helper()
		s, err := snap.(*portfolioSnapshotService).computeSnapshot(e.user.ID, time.Now())
		testutil.AssertNoError(t, err)
		return s.TotalNetWorth
	}
	if got := netWorth(); got != recCashStart {
		t.Fatalf("starting net worth = %d", got)
	}
	x := e.split(6000, "Alex", "Sam")
	if got := netWorth(); got != recCashStart-6000 {
		t.Errorf("after $60 split net worth = %d, want %d", got, recCashStart-6000)
	}
	_, s := e.mustSettle(x.Receivables[0].ID, 2000)
	if got := netWorth(); got != recCashStart-4000 {
		t.Errorf("after $20 repayment net worth = %d, want %d", got, recCashStart-4000)
	}
	_, err := e.writeOff(x.Receivables[1].ID, nil)
	testutil.AssertNoError(t, err)
	if got := netWorth(); got != recCashStart-4000 {
		t.Errorf("a write-off must not change net worth, got %d", got)
	}
	_, err = e.delSettlement(s.ID)
	testutil.AssertNoError(t, err)
	if got := netWorth(); got != recCashStart-6000 {
		t.Errorf("after reversing the repayment net worth = %d", got)
	}
}
