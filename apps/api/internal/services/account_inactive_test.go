package services

import (
	"testing"

	"kuberan/internal/testutil"
)

func TestInactiveAccountStaysReachable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)

	off := false
	_, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{IsActive: &off})
	testutil.AssertNoError(t, err)

	// Active-only lookups (used before writing activity) treat it as missing.
	_, err = svc.GetAccountByID(user.ID, account.ID)
	testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")

	// The read path used by the account page still finds it.
	got, err := svc.GetAccountByIDIncludingInactive(user.ID, account.ID)
	testutil.AssertNoError(t, err)
	if got.IsActive {
		t.Errorf("expected the account to be inactive")
	}

	// Other users still can't see it.
	other := testutil.CreateTestUser(t, db)
	_, err = svc.GetAccountByIDIncludingInactive(other.ID, account.ID)
	testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")

	// And it can be reactivated.
	on := true
	updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{IsActive: &on})
	testutil.AssertNoError(t, err)
	if !updated.IsActive {
		t.Errorf("expected the account to be active again")
	}
	_, err = svc.GetAccountByID(user.ID, account.ID)
	testutil.AssertNoError(t, err)
}
