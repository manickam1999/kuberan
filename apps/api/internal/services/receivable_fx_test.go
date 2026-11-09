package services

import (
	"errors"
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// recFXEnv is a MYR user with a MYR cash account (e.cash) and a USD cash
// account, with a receivable service that can convert.
type recFXEnv struct {
	*recEnv
	fxs FXServicer
	usd *models.Account
}

func newRecFXEnv(t *testing.T, usdRate string) *recFXEnv {
	t.Helper()
	e := newRecEnv(t)
	testutil.AssertNoError(t, e.db.Model(e.user).Update("base_currency", "MYR").Error)
	testutil.AssertNoError(t, e.db.Model(e.cash).Update("currency", "MYR").Error)
	fxs := NewFXService(e.db, testFXStaleness)
	e.rec = NewReceivableService(e.db, e.acct, e.rules, WithReceivableFXService(fxs))
	usd := e.track(testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 500000))
	testutil.AssertNoError(t, e.db.Model(usd).Update("currency", "USD").Error)
	if usdRate != "" {
		seedRate(t, fxs, "USD", "MYR", usdRate, time.Now().Add(-time.Hour))
	}
	return &recFXEnv{recEnv: e, fxs: fxs, usd: usd}
}

// usdExpense creates a USD expense of amount shared fully with one counterparty.
func (e *recFXEnv) usdExpense(amount int64, name string) *models.Receivable {
	e.t.Helper()
	in := e.in(amount, ShareModeFull, models.ReceivableKindLoan, rcpEqual(name)...)
	in.AccountID = e.usd.ID
	x := e.mustCreate(in)
	return &x.Receivables[0]
}

func TestReceivableCurrency_FollowsExpenseAccountOnListGetAndSettle(t *testing.T) {
	e := newRecFXEnv(t, "4")
	usdRec := e.usdExpense(10000, "Alex")
	myrTx := e.split(3000, "Sam")

	list, err := e.rec.ListReceivables(e.user.ID, pageReq(), ReceivableFilter{})
	testutil.AssertNoError(t, err)
	got := map[string]string{}
	for _, r := range list.Data {
		got[r.CounterpartyKey] = r.Currency
	}
	if got["alex"] != "USD" || got["sam"] != "MYR" {
		t.Errorf("list currencies = %v, want alex USD / sam MYR", got)
	}

	// Cross-currency settle: USD 20.00 repaid into the MYR account as MYR 89.00.
	_, set, err := e.rec.Settle(e.user.ID, usdRec.ID, SettleInput{AccountID: e.cash.ID, Amount: 2000, AccountAmount: i64(8900)})
	testutil.AssertNoError(t, err)
	if set.Currency != "USD" || set.Amount != 2000 || set.AccountAmount == nil || *set.AccountAmount != 8900 {
		t.Errorf("settlement = %+v", set)
	}
	one, err := e.rec.GetReceivable(e.user.ID, usdRec.ID)
	testutil.AssertNoError(t, err)
	if one.Currency != "USD" || len(one.Settlements) != 1 || one.Settlements[0].Currency != "USD" || one.Settlements[0].Amount != 2000 {
		t.Errorf("receivable = %+v", one)
	}
	if one.PaidAmount != 2000 || one.Outstanding != 8000 {
		t.Errorf("paid/outstanding = %d/%d, want 2000/8000 (receivable currency)", one.PaidAmount, one.Outstanding)
	}
	_ = myrTx
}

func TestSettle_CrossCurrencyThenDeleteRestoresAccountExactly(t *testing.T) {
	e := newRecFXEnv(t, "4.4712")
	rec := e.usdExpense(10000, "Alex")
	myrBefore, usdBefore := e.balance(e.cash.ID), e.balance(e.usd.ID)

	// Missing account_amount: FX_AMOUNT_REQUIRED with a suggestion (2000 * 4.4712 = 8942).
	_, _, err := e.settle(rec.ID, e.cash.ID, 2000, false)
	assertFXAmountRequired(t, err, "account_amount", int64(8942))
	if e.balance(e.cash.ID) != myrBefore {
		t.Fatal("a rejected settle moved money")
	}

	_, set, err := e.rec.Settle(e.user.ID, rec.ID, SettleInput{AccountID: e.cash.ID, Amount: 2000, AccountAmount: i64(8900)})
	e.check()
	testutil.AssertNoError(t, err)
	if got := e.balance(e.cash.ID); got != myrBefore+8900 {
		t.Errorf("MYR balance = %d, want %d (account_amount credited, not 2000)", got, myrBefore+8900)
	}
	var reimbursement models.Transaction
	testutil.AssertNoError(t, e.db.First(&reimbursement, "id = ?", set.TransactionID).Error)
	if reimbursement.Amount != 8900 {
		t.Errorf("reimbursement amount = %d, want 8900", reimbursement.Amount)
	}
	if e.balance(e.usd.ID) != usdBefore {
		t.Error("the USD expense account must not move on settle")
	}

	r, err := e.delSettlement(set.ID)
	testutil.AssertNoError(t, err)
	if got := e.balance(e.cash.ID); got != myrBefore {
		t.Errorf("MYR balance after delete = %d, want exactly %d", got, myrBefore)
	}
	if r.PaidAmount != 0 || r.Outstanding != 10000 || r.Status != models.ReceivableStatusPending {
		t.Errorf("receivable after delete = %+v", r)
	}
}

func TestSettle_CrossCurrencyWriteOffRemainderReversesBoth(t *testing.T) {
	e := newRecFXEnv(t, "4")
	rec := e.usdExpense(10000, "Alex")
	myrBefore := e.balance(e.cash.ID)

	r, set, err := e.rec.Settle(e.user.ID, rec.ID, SettleInput{AccountID: e.cash.ID, Amount: 6000, AccountAmount: i64(24500), WriteOffRemainder: true})
	e.check()
	testutil.AssertNoError(t, err)
	if r.Status != models.ReceivableStatusWrittenOff || set.WrittenOffAmount != 4000 {
		t.Errorf("receivable %s, written off on settlement %d", r.Status, set.WrittenOffAmount)
	}
	_, err = e.delSettlement(set.ID)
	testutil.AssertNoError(t, err)
	if got := e.balance(e.cash.ID); got != myrBefore {
		t.Errorf("MYR balance = %d, want %d", got, myrBefore)
	}
}

func TestSettle_AccountAmountValidation(t *testing.T) {
	e := newRecFXEnv(t, "")
	usdRec := e.usdExpense(10000, "Alex")
	myrTx := e.split(3000, "Sam")
	myrRec := myrTx.Receivables[0]

	t.Run("cross-currency without a rate still requires account_amount, no suggestion", func(t *testing.T) {
		_, _, err := e.settle(usdRec.ID, e.cash.ID, 100, false)
		assertFXAmountRequired(t, err, "account_amount", 0)
	})
	t.Run("non-positive account_amount is rejected", func(t *testing.T) {
		_, _, err := e.rec.Settle(e.user.ID, usdRec.ID, SettleInput{AccountID: e.cash.ID, Amount: 100, AccountAmount: i64(0)})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})
	t.Run("same currency: equal is accepted and not stored", func(t *testing.T) {
		_, set, err := e.rec.Settle(e.user.ID, myrRec.ID, SettleInput{AccountID: e.cash.ID, Amount: 100, AccountAmount: i64(100)})
		testutil.AssertNoError(t, err)
		if set.AccountAmount != nil {
			t.Errorf("account_amount = %d, want NULL", *set.AccountAmount)
		}
	})
	t.Run("same currency: unequal is rejected", func(t *testing.T) {
		before := e.balance(e.cash.ID)
		_, _, err := e.rec.Settle(e.user.ID, myrRec.ID, SettleInput{AccountID: e.cash.ID, Amount: 100, AccountAmount: i64(99)})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
		if e.balance(e.cash.ID) != before {
			t.Error("a rejected settle moved money")
		}
	})
	t.Run("USD receivable settled into a USD account is same-currency", func(t *testing.T) {
		usd2 := e.track(testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, 0))
		testutil.AssertNoError(t, e.db.Model(usd2).Update("currency", "USD").Error)
		_, set, err := e.rec.Settle(e.user.ID, usdRec.ID, SettleInput{AccountID: usd2.ID, Amount: 100})
		testutil.AssertNoError(t, err)
		if set.AccountAmount != nil || e.balance(usd2.ID) != 100 {
			t.Errorf("account_amount=%v balance=%d", set.AccountAmount, e.balance(usd2.ID))
		}
	})
}

func assertFXAmountRequired(t *testing.T, err error, field string, suggested int64) {
	t.Helper()
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrFXAmountRequired.Code {
		t.Fatalf("err = %v, want FX_AMOUNT_REQUIRED", err)
	}
	d := ae.Details
	if d["field"] != field {
		t.Errorf("details field = %v, want %s", d["field"], field)
	}
	got, has := d["suggested_amount"].(int64)
	if suggested == 0 {
		if has {
			t.Errorf("unexpected suggestion %d without a rate", got)
		}
		return
	}
	if !has || got != suggested {
		t.Errorf("suggested_amount = %v, want %d", d["suggested_amount"], suggested)
	}
}

// --- Summary ---

func TestSummary_FastPathNeedsNoFX(t *testing.T) {
	e := newRecEnv(t)
	testutil.AssertNoError(t, e.db.Model(e.user).Update("base_currency", "MYR").Error)
	testutil.AssertNoError(t, e.db.Model(e.cash).Update("currency", "MYR").Error)
	e.rec = NewReceivableService(e.db, e.acct, e.rules, WithReceivableFXService(panicFX{}))
	e.split(3000, "Sam")
	e.split(1000, "Kim")

	s, err := e.rec.Summary(e.user.ID)
	testutil.AssertNoError(t, err)
	if s.TotalOutstanding != 2000 || s.Currency != "MYR" || s.FXMissing || len(s.ByCurrency) != 1 ||
		s.ByCurrency[0] != (ReceivableCurrencyTotal{Currency: "MYR", Outstanding: 2000}) {
		t.Errorf("summary = %+v", s)
	}
	// nil fx service: same result.
	e.rec = NewReceivableService(e.db, e.acct, e.rules)
	s, err = e.rec.Summary(e.user.ID)
	testutil.AssertNoError(t, err)
	if s.TotalOutstanding != 2000 || s.FXMissing {
		t.Errorf("nil fx summary = %+v", s)
	}
}

func TestSummary_MixedMYRAndUSDConvertsToBase(t *testing.T) {
	e := newRecFXEnv(t, "4")
	// Equal split of 3000 with Sam (MYR): Sam owes 1500. USD loan 10000 to Sam, 2000 to Kim.
	e.split(3000, "Sam")
	e.usdExpense(10000, "Sam")
	e.usdExpense(2000, "Kim")

	s, err := e.rec.Summary(e.user.ID)
	testutil.AssertNoError(t, err)
	// 1500 MYR + (10000+2000) USD * 4 = 49500.
	if s.TotalOutstanding != 49500 || s.Currency != "MYR" || s.FXMissing {
		t.Errorf("total = %d currency %s missing %v, want 49500 MYR", s.TotalOutstanding, s.Currency, s.FXMissing)
	}
	want := []ReceivableCurrencyTotal{{Currency: "MYR", Outstanding: 1500}, {Currency: "USD", Outstanding: 12000}}
	if len(s.ByCurrency) != 2 || s.ByCurrency[0] != want[0] || s.ByCurrency[1] != want[1] {
		t.Errorf("by_currency = %+v, want %+v", s.ByCurrency, want)
	}
	cp := map[string]int64{}
	for _, c := range s.ByCounterparty {
		cp[c.CounterpartyKey] = c.Outstanding
	}
	if cp["sam"] != 1500+40000 || cp["kim"] != 8000 {
		t.Errorf("by_counterparty = %v, want sam 41500 / kim 8000", cp)
	}
	var kinds int64
	for _, k := range s.ByKind {
		kinds += k.Outstanding
	}
	if kinds != 49500 {
		t.Errorf("by_kind sums to %d, want 49500", kinds)
	}
}

func TestSummary_MissingRateIsLeftOutNotOneToOne(t *testing.T) {
	e := newRecFXEnv(t, "")
	e.split(3000, "Sam")
	e.usdExpense(10000, "Alex")

	s, err := e.rec.Summary(e.user.ID)
	testutil.AssertNoError(t, err)
	if s.TotalOutstanding != 1500 || !s.FXMissing {
		t.Errorf("total = %d fx_missing %v, want 1500 (MYR only) and fx_missing", s.TotalOutstanding, s.FXMissing)
	}
	if len(s.ByCurrency) != 2 || s.ByCurrency[1] != (ReceivableCurrencyTotal{Currency: "USD", Outstanding: 10000}) {
		t.Errorf("by_currency = %+v, want native USD 10000 retained", s.ByCurrency)
	}
}
