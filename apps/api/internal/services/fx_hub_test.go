package services

import (
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/testutil"
)

// The oracle publishes each currency in use against the users' base currency
// (USD→MYR, SGD→MYR), so a USD→SGD quote only exists as a cross through MYR.
func TestFXRate_CrossesThroughAUsersBaseCurrency(t *testing.T) {
	svc, db := newFXTest(t, testFXStaleness)
	testutil.CreateTestUser(t, db) // base currency MYR (the default)
	older := fxNow.Add(-30 * time.Hour)
	newer := fxNow.Add(-1 * time.Hour)
	seedRate(t, svc, "USD", "MYR", "4.5", newer)
	seedRate(t, svc, "SGD", "MYR", "2.5", older)

	q, err := svc.Rate("USD", "SGD", fxNow)
	testutil.AssertNoError(t, err)
	if q.Path != FXPathPivotPrefix+"MYR" || q.Rate != mustRate(t, "1.8") {
		t.Errorf("got %s via %s, want 1.8 via pivot:MYR", q.Rate, q.Path)
	}
	if !q.RecordedAt.Equal(older) {
		t.Errorf("RecordedAt = %s, want the older leg %s", q.RecordedAt, older)
	}

	back, err := svc.Rate("SGD", "USD", fxNow)
	testutil.AssertNoError(t, err)
	want, err := fx.Mul(mustRate(t, "2.5"), mustInvert(t, mustRate(t, "4.5")))
	testutil.AssertNoError(t, err)
	if back.Rate != want {
		t.Errorf("SGD→USD = %s, want %s", back.Rate, want)
	}
}

func TestFXRate_NoHubNoCross(t *testing.T) {
	svc, db := newFXTest(t, testFXStaleness)
	testutil.CreateTestUser(t, db)
	seedRate(t, svc, "USD", "MYR", "4.5", fxNow)
	// No SGD leg at all: still unavailable, never guessed.
	_, err := svc.Rate("USD", "SGD", fxNow)
	testutil.AssertAppError(t, err, apperrors.ErrFXRateUnavailable.Code)
}
