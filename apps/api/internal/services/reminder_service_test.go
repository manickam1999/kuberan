package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func utcYMD(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNextDueDate(t *testing.T) {
	tests := []struct {
		name   string
		dueDay int
		from   time.Time
		want   time.Time
	}{
		{"later this month", 20, utcYMD(2026, 11, 9), utcYMD(2026, 11, 20)},
		{"today counts", 9, utcYMD(2026, 11, 9), utcYMD(2026, 11, 9)},
		{"already passed rolls to next month", 1, utcYMD(2026, 11, 9), utcYMD(2026, 12, 1)},
		{"year rollover", 5, utcYMD(2026, 12, 20), utcYMD(2027, 1, 5)},
		{"31 in a 30-day month clamps", 31, utcYMD(2026, 11, 9), utcYMD(2026, 11, 30)},
		{"31 rolls into February (non-leap)", 31, utcYMD(2027, 1, 31).AddDate(0, 0, 1), utcYMD(2027, 2, 28)},
		{"30 in a leap February", 30, utcYMD(2028, 2, 10), utcYMD(2028, 2, 29)},
		{"time of day is ignored", 9, time.Date(2026, 11, 9, 23, 59, 0, 0, time.UTC), utcYMD(2026, 11, 9)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextDueDate(tt.dueDay, tt.from)
			if !got.Equal(tt.want) {
				t.Fatalf("NextDueDate(%d, %v) = %v, want %v", tt.dueDay, tt.from, got, tt.want)
			}
		})
	}
}

func TestCreateDebtAccount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)

	t.Run("amount owed is an opening balance, never inverted", func(t *testing.T) {
		acct, err := svc.CreateDebtAccount(user.ID, CreateDebtParams{
			Name: "Car loan", Currency: "MYR", AmountOwed: 5000000, InterestRate: 3.5, MonthlyPayment: 120000, DueDay: 15,
		})
		testutil.AssertNoError(t, err)
		if acct.Type != models.AccountTypeDebt || acct.Balance != 5000000 {
			t.Fatalf("unexpected account: type=%s balance=%d", acct.Type, acct.Balance)
		}
		if acct.MonthlyPayment == nil || *acct.MonthlyPayment != 120000 || acct.DueDay == nil || *acct.DueDay != 15 {
			t.Fatalf("monthly payment / due day not stored: %+v %+v", acct.MonthlyPayment, acct.DueDay)
		}
		var rows []models.Transaction
		testutil.AssertNoError(t, db.Where("account_id = ?", acct.ID).Find(&rows).Error)
		if len(rows) != 1 || rows[0].Type != models.TransactionTypeOpeningBalance || rows[0].Amount != 5000000 {
			t.Fatalf("expected one opening_balance row of 5000000, got %+v", rows)
		}
		var rem []models.Reminder
		testutil.AssertNoError(t, db.Where("ref_id = ?", acct.ID).Find(&rem).Error)
		if len(rem) != 1 || rem[0].Kind != models.ReminderKindLoanDue || !rem[0].Enabled {
			t.Fatalf("expected one loan_due reminder, got %+v", rem)
		}
	})

	t.Run("currency defaults to the base currency and no due day means no reminder", func(t *testing.T) {
		acct, err := svc.CreateDebtAccount(user.ID, CreateDebtParams{Name: "Family loan", AmountOwed: 100})
		testutil.AssertNoError(t, err)
		if acct.Currency != "MYR" && acct.Currency != user.BaseCurrency {
			t.Fatalf("expected base currency, got %s (base %s)", acct.Currency, user.BaseCurrency)
		}
		if acct.MonthlyPayment != nil || acct.DueDay != nil {
			t.Fatalf("unset fields must stay nil")
		}
		var n int64
		db.Model(&models.Reminder{}).Where("ref_id = ?", acct.ID).Count(&n)
		if n != 0 {
			t.Fatalf("expected no reminder, got %d", n)
		}
	})

	t.Run("zero amount writes no opening row", func(t *testing.T) {
		acct, err := svc.CreateDebtAccount(user.ID, CreateDebtParams{Name: "Paid up", Currency: "USD"})
		testutil.AssertNoError(t, err)
		var n int64
		db.Model(&models.Transaction{}).Where("account_id = ?", acct.ID).Count(&n)
		if n != 0 {
			t.Fatalf("expected no transactions, got %d", n)
		}
	})

	t.Run("validation", func(t *testing.T) {
		bad := []CreateDebtParams{
			{Name: ""},
			{Name: "x", AmountOwed: -1},
			{Name: "x", MonthlyPayment: -1},
			{Name: "x", DueDay: 32},
			{Name: "x", DueDay: -1},
			{Name: "x", InterestRate: 101},
			{Name: "x", Currency: "JPY"},
		}
		for _, p := range bad {
			if _, err := svc.CreateDebtAccount(user.ID, p); err == nil {
				t.Errorf("expected an error for %+v", p)
			}
		}
	})
}

func TestUpdateAccount_LoanAndCardDueDay(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)

	card, err := svc.CreateCreditCardAccountWithParams(user.ID, CreateCreditCardParams{Name: "Amex", Currency: "MYR", CurrentBalance: 123450, DueDay: 1})
	testutil.AssertNoError(t, err)
	if card.DueDay == nil || *card.DueDay != 1 {
		t.Fatalf("due day not stored on create")
	}
	var rem []models.Reminder
	db.Where("ref_id = ?", card.ID).Find(&rem)
	if len(rem) != 1 || rem[0].Kind != models.ReminderKindCardDue {
		t.Fatalf("expected one card_due reminder, got %+v", rem)
	}

	// Changing the due day replaces the unsent reminder.
	ten := 10
	_, err = svc.UpdateAccount(user.ID, card.ID, AccountUpdateFields{DueDay: &ten})
	testutil.AssertNoError(t, err)
	rem = nil
	db.Where("ref_id = ?", card.ID).Find(&rem)
	if len(rem) != 1 || rem[0].DueOn.Day() != 10 {
		t.Fatalf("expected one reminder due on the 10th, got %+v", rem)
	}

	// Clearing the due day removes it.
	zero := 0
	_, err = svc.UpdateAccount(user.ID, card.ID, AccountUpdateFields{DueDay: &zero})
	testutil.AssertNoError(t, err)
	var n int64
	db.Model(&models.Reminder{}).Where("ref_id = ?", card.ID).Count(&n)
	if n != 0 {
		t.Fatalf("expected the reminder to be removed, got %d", n)
	}

	// Invalid due day is rejected and nothing changes.
	bad := 40
	if _, err := svc.UpdateAccount(user.ID, card.ID, AccountUpdateFields{DueDay: &bad}); err == nil {
		t.Fatal("expected an error for due day 40")
	}

	// Loan: monthly payment can be set, changed and cleared; cards ignore it.
	loan, err := svc.CreateDebtAccount(user.ID, CreateDebtParams{Name: "Loan", Currency: "MYR", AmountOwed: 1000})
	testutil.AssertNoError(t, err)
	pay := int64(25000)
	got, err := svc.UpdateAccount(user.ID, loan.ID, AccountUpdateFields{MonthlyPayment: &pay})
	testutil.AssertNoError(t, err)
	if got.MonthlyPayment == nil || *got.MonthlyPayment != 25000 {
		t.Fatalf("monthly payment not updated")
	}
	clear0 := int64(0)
	got, err = svc.UpdateAccount(user.ID, loan.ID, AccountUpdateFields{MonthlyPayment: &clear0})
	testutil.AssertNoError(t, err)
	if got.MonthlyPayment != nil {
		t.Fatalf("monthly payment not cleared")
	}
	got, err = svc.UpdateAccount(user.ID, card.ID, AccountUpdateFields{MonthlyPayment: &pay})
	testutil.AssertNoError(t, err)
	if got.MonthlyPayment != nil {
		t.Fatalf("credit cards must ignore monthly payment")
	}
}

func TestReminders_DeactivateRemovesUnsent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)
	loan, err := svc.CreateDebtAccount(user.ID, CreateDebtParams{Name: "Loan", Currency: "MYR", AmountOwed: 1000, DueDay: 5})
	testutil.AssertNoError(t, err)

	off := false
	_, err = svc.UpdateAccount(user.ID, loan.ID, AccountUpdateFields{IsActive: &off})
	testutil.AssertNoError(t, err)
	var n int64
	db.Model(&models.Reminder{}).Where("ref_id = ?", loan.ID).Count(&n)
	if n != 0 {
		t.Fatalf("expected no reminders for an inactive account, got %d", n)
	}
}

func TestReminders_FixedDepositLifecycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	svc := NewAccountService(db, WithCategoryService(catSvc))
	user := testutil.CreateTestUser(t, db)
	src := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
	dst := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	today := utcDay(time.Now())
	fd, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
		Name: "FD", Currency: "USD", SourceAccountID: src.ID, Principal: 100000, InterestRate: 4,
		StartDate: today, MaturityDate: today.AddDate(0, 6, 0),
		PayoutType: models.PayoutTypeCumulative, Compounding: models.CompoundingMonthly,
	})
	testutil.AssertNoError(t, err)

	var rem []models.Reminder
	db.Where("ref_id = ?", fd.ID).Find(&rem)
	if len(rem) != 1 || rem[0].Kind != models.ReminderKindFDMaturity {
		t.Fatalf("expected one fd_maturity reminder, got %+v", rem)
	}
	if !utcDay(rem[0].DueOn).Equal(today.AddDate(0, 6, 0)) {
		t.Fatalf("reminder due %v, want maturity %v", rem[0].DueOn, today.AddDate(0, 6, 0))
	}
	if !rem[0].RemindAt.Equal(reminderRemindAt(today.AddDate(0, 6, 0), fdReminderLeadDays)) {
		t.Fatalf("unexpected remind_at %v", rem[0].RemindAt)
	}

	// Moving the maturity date moves the reminder.
	newDate := today.AddDate(0, 8, 0)
	_, err = svc.UpdateAccount(user.ID, fd.ID, AccountUpdateFields{MaturityDate: &newDate})
	testutil.AssertNoError(t, err)
	rem = nil
	db.Where("ref_id = ?", fd.ID).Find(&rem)
	if len(rem) != 1 || !utcDay(rem[0].DueOn).Equal(newDate) {
		t.Fatalf("expected the reminder to follow the maturity date, got %+v", rem)
	}

	// Closing the deposit removes it.
	_, err = svc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: &dst.ID})
	testutil.AssertNoError(t, err)
	var n int64
	db.Model(&models.Reminder{}).Where("ref_id = ?", fd.ID).Count(&n)
	if n != 0 {
		t.Fatalf("expected no reminders after close, got %d", n)
	}
}

// reminderEnv is a user with an active Telegram link and a card due on a
// chosen day, plus a clock set to when the card's reminder becomes sendable.
type reminderEnv struct {
	svc   ReminderServicer
	accts AccountServicer
	user  *models.User
	card  *models.Account
	link  *models.TelegramLink
	now   time.Time
}

func reminderFixture(t *testing.T, dueDay int) reminderEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	env := reminderEnv{accts: NewAccountService(db), svc: NewReminderService(db)}
	env.user = testutil.CreateTestUser(t, db)
	env.link = testutil.CreateTestTelegramLink(t, db, env.user.ID, 777)
	var err error
	env.card, err = env.accts.CreateCreditCardAccountWithParams(env.user.ID, CreateCreditCardParams{Name: "Amex Gold", Currency: "MYR", CurrentBalance: 123450, DueDay: dueDay})
	testutil.AssertNoError(t, err)
	// Wind the clock to the day the reminder becomes sendable.
	var rem models.Reminder
	testutil.AssertNoError(t, db.Where("ref_id = ?", env.card.ID).First(&rem).Error)
	env.now = rem.RemindAt.Add(time.Minute)
	return env
}

func TestReminderService_DueAndMarkSent(t *testing.T) {
	env := reminderFixture(t, 1)
	svc, card, now := env.svc, env.card, env.now

	t.Run("not due before remind_at", func(t *testing.T) {
		got, err := svc.Due(now.AddDate(0, 0, -2))
		testutil.AssertNoError(t, err)
		if len(got) != 0 {
			t.Fatalf("expected nothing due yet, got %+v", got)
		}
	})

	var id string
	t.Run("due once remind_at has passed, in the account's own currency", func(t *testing.T) {
		got, err := svc.Due(now)
		testutil.AssertNoError(t, err)
		if len(got) != 1 {
			t.Fatalf("expected 1 due reminder, got %+v", got)
		}
		d := got[0]
		if d.TelegramUserID != 777 || d.AccountName != "Amex Gold" || d.Amount != 123450 || d.Currency != "MYR" || d.Kind != models.ReminderKindCardDue {
			t.Fatalf("unexpected due reminder: %+v", d)
		}
		if d.AccountID != card.ID {
			t.Fatalf("wrong account")
		}
		id = d.ID
	})

	t.Run("mark sent is idempotent and hides it", func(t *testing.T) {
		first, err := svc.MarkSent(id, now)
		testutil.AssertNoError(t, err)
		if first.SentAt == nil {
			t.Fatal("sent_at not set")
		}
		later := now.Add(time.Hour)
		second, err := svc.MarkSent(id, later)
		testutil.AssertNoError(t, err)
		if !second.SentAt.Equal(*first.SentAt) {
			t.Fatalf("sent_at moved on the second confirmation: %v vs %v", second.SentAt, first.SentAt)
		}
		got, err := svc.Due(later)
		testutil.AssertNoError(t, err)
		if len(got) != 0 {
			t.Fatalf("a sent reminder must not be due again: %+v", got)
		}
	})

	t.Run("the next month's occurrence was created", func(t *testing.T) {
		list, err := svc.ListUpcoming(card.UserID, card.ID, now)
		testutil.AssertNoError(t, err)
		if len(list) != 2 {
			t.Fatalf("expected the sent and the next reminder, got %+v", list)
		}
		if !list[1].DueOn.After(list[0].DueOn) || list[1].SentAt != nil {
			t.Fatalf("next occurrence should be later and unsent: %+v", list)
		}
	})

	t.Run("unknown reminder", func(t *testing.T) {
		_, err := svc.MarkSent("00000000-0000-0000-0000-000000000000", now)
		if err != apperrors.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestReminderService_DueFilters(t *testing.T) {
	t.Run("kind switched off on the link", func(t *testing.T) {
		env := reminderFixture(t, 1)
		svc, user, now := env.svc, env.user, env.now
		tg := NewTelegramService(dbOf(t, svc))
		off := false
		_, err := tg.UpdateReminderSettings(user.ID, ReminderSettings{CardDue: &off})
		testutil.AssertNoError(t, err)
		got, err := svc.Due(now)
		testutil.AssertNoError(t, err)
		if len(got) != 0 {
			t.Fatalf("card reminders are off: %+v", got)
		}
		on := true
		_, err = tg.UpdateReminderSettings(user.ID, ReminderSettings{CardDue: &on})
		testutil.AssertNoError(t, err)
		got, _ = svc.Due(now)
		if len(got) != 1 {
			t.Fatalf("expected the reminder back, got %+v", got)
		}
	})

	t.Run("reminder switched off for the account", func(t *testing.T) {
		env := reminderFixture(t, 1)
		svc, user, card, now := env.svc, env.user, env.card, env.now
		list, err := svc.ListUpcoming(user.ID, card.ID, now)
		testutil.AssertNoError(t, err)
		_, err = svc.SetEnabled(user.ID, list[0].ID, false)
		testutil.AssertNoError(t, err)
		got, _ := svc.Due(now)
		if len(got) != 0 {
			t.Fatalf("disabled reminder must not be due: %+v", got)
		}
	})

	t.Run("no active link", func(t *testing.T) {
		env := reminderFixture(t, 1)
		svc, link, now := env.svc, env.link, env.now
		db := dbOf(t, svc)
		testutil.AssertNoError(t, db.Model(link).Update("is_active", false).Error)
		got, _ := svc.Due(now)
		if len(got) != 0 {
			t.Fatalf("unlinked users get no reminders: %+v", got)
		}
	})

	t.Run("settled card needs no nudge", func(t *testing.T) {
		env := reminderFixture(t, 1)
		svc, card, now := env.svc, env.card, env.now
		db := dbOf(t, svc)
		testutil.AssertNoError(t, db.Model(&models.Account{}).Where("id = ?", card.ID).Update("balance", 0).Error)
		got, _ := svc.Due(now)
		if len(got) != 0 {
			t.Fatalf("nothing owed, nothing due: %+v", got)
		}
	})

	t.Run("stale reminders roll to the next occurrence", func(t *testing.T) {
		env := reminderFixture(t, 1)
		svc, user, card, now := env.svc, env.user, env.card, env.now
		// Two months on, the unsent reminder's due date is long gone.
		got, err := svc.Due(now.AddDate(0, 2, 0))
		testutil.AssertNoError(t, err)
		_ = got
		list, err := svc.ListUpcoming(user.ID, card.ID, now.AddDate(0, 2, 0))
		testutil.AssertNoError(t, err)
		if len(list) != 1 || list[0].DueOn.Before(utcDay(now.AddDate(0, 2, 0))) {
			t.Fatalf("expected one upcoming reminder after the roll, got %+v", list)
		}
	})
}

func TestReminderService_SetEnabledIsUserScoped(t *testing.T) {
	env := reminderFixture(t, 1)
	svc, user, card, now := env.svc, env.user, env.card, env.now
	list, err := svc.ListUpcoming(user.ID, card.ID, now)
	testutil.AssertNoError(t, err)
	if _, err := svc.SetEnabled("someone-else", list[0].ID, false); err != apperrors.ErrNotFound {
		t.Fatalf("expected ErrNotFound for another user, got %v", err)
	}
	other, err := svc.ListUpcoming("someone-else", "", now)
	testutil.AssertNoError(t, err)
	if len(other) != 0 {
		t.Fatalf("another user must not see the reminders: %+v", other)
	}
}

func TestReminders_DisabledStaysDisabledAcrossOccurrences(t *testing.T) {
	env := reminderFixture(t, 1)
	svc, user, card, now := env.svc, env.user, env.card, env.now
	list, _ := svc.ListUpcoming(user.ID, card.ID, now)
	_, err := svc.SetEnabled(user.ID, list[0].ID, false)
	testutil.AssertNoError(t, err)
	// Mark it sent anyway (e.g. it was enabled when sent); the next one inherits the switch.
	_, err = svc.MarkSent(list[0].ID, now)
	testutil.AssertNoError(t, err)
	list, _ = svc.ListUpcoming(user.ID, card.ID, now)
	for _, r := range list {
		if r.Enabled {
			t.Fatalf("expected every occurrence to stay disabled: %+v", list)
		}
	}
}

// dbOf digs the DB out of a reminder service built by reminderFixture.
func dbOf(t *testing.T, svc ReminderServicer) *gorm.DB {
	t.Helper()
	rs, ok := svc.(*reminderService)
	if !ok {
		t.Fatal("not a *reminderService")
	}
	return rs.db
}
