package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestComputeAndRecordSnapshots(t *testing.T) {
	t.Run("creates_snapshots_for_all_users", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user1.ID, 100000)
		testutil.CreateTestCashAccountWithBalance(t, db, user2.ID, 200000)

		recordedAt := time.Now().Truncate(time.Second)
		count, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		if count != 2 {
			t.Errorf("expected 2 snapshots, got %d", count)
		}

		// Verify both snapshots exist
		var snapshots []models.PortfolioSnapshot
		db.Find(&snapshots)
		if len(snapshots) != 2 {
			t.Errorf("expected 2 snapshots in DB, got %d", len(snapshots))
		}
	})

	t.Run("cash_balance_computed_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000) // $10,000.00
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)  // $5,000.00

		recordedAt := time.Now().Truncate(time.Second)
		count, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		if count != 1 {
			t.Fatalf("expected 1 snapshot, got %d", count)
		}

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.CashBalance != 1500000 {
			t.Errorf("expected cash_balance 1500000, got %d", snap.CashBalance)
		}
	})

	t.Run("investment_value_computed_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		investAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec1 := testutil.CreateTestSecurity(t, db)
		sec2 := testutil.CreateTestSecurity(t, db)

		// 10 shares, cost $1,000
		inv1 := &models.Investment{
			AccountID:  investAcct.ID,
			SecurityID: sec1.ID,
			Quantity:   10.0,
			CostBasis:  100000,
		}
		db.Create(inv1)
		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 10000, time.Now()) // $100/share

		// 5 shares, cost $1,000
		inv2 := &models.Investment{
			AccountID:  investAcct.ID,
			SecurityID: sec2.ID,
			Quantity:   5.0,
			CostBasis:  100000,
		}
		db.Create(inv2)
		testutil.CreateTestSecurityPrice(t, db, sec2.ID, 20000, time.Now()) // $200/share

		recordedAt := time.Now().Truncate(time.Second)
		count, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		if count != 1 {
			t.Fatalf("expected 1 snapshot, got %d", count)
		}

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		// 10*10000 + 5*20000 = 100000 + 100000 = 200000
		if snap.InvestmentValue != 200000 {
			t.Errorf("expected investment_value 200000, got %d", snap.InvestmentValue)
		}
	})

	t.Run("debt_balance_computed_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestDebtAccount(t, db, user.ID, 500000)            // $5,000
		testutil.CreateTestCreditCardAccount(t, db, user.ID, 200000)      // $2,000
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000) // need active account for user to appear

		recordedAt := time.Now().Truncate(time.Second)
		count, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		if count != 1 {
			t.Fatalf("expected 1 snapshot, got %d", count)
		}

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.DebtBalance != 700000 {
			t.Errorf("expected debt_balance 700000, got %d", snap.DebtBalance)
		}
	})

	t.Run("net_worth_computed_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)

		// Cash: $15,000
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		// Investments: 10 shares @ $100 + 5 shares @ $200 = $2,000
		investAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec1 := testutil.CreateTestSecurity(t, db)
		sec2 := testutil.CreateTestSecurity(t, db)
		db.Create(&models.Investment{
			AccountID: investAcct.ID, SecurityID: sec1.ID,
			Quantity: 10.0, CostBasis: 100000,
		})
		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 10000, time.Now())
		db.Create(&models.Investment{
			AccountID: investAcct.ID, SecurityID: sec2.ID,
			Quantity: 5.0, CostBasis: 100000,
		})
		testutil.CreateTestSecurityPrice(t, db, sec2.ID, 20000, time.Now())

		// Debt: $7,000
		testutil.CreateTestDebtAccount(t, db, user.ID, 500000)
		testutil.CreateTestCreditCardAccount(t, db, user.ID, 200000)

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		// cash=1500000, investment=200000, debt=700000
		// net_worth = 1500000 + 200000 - 700000 = 1000000
		expectedCash := int64(1500000)
		expectedInvest := int64(200000)
		expectedDebt := int64(700000)
		expectedNetWorth := expectedCash + expectedInvest - expectedDebt

		if snap.CashBalance != expectedCash {
			t.Errorf("expected cash_balance %d, got %d", expectedCash, snap.CashBalance)
		}
		if snap.InvestmentValue != expectedInvest {
			t.Errorf("expected investment_value %d, got %d", expectedInvest, snap.InvestmentValue)
		}
		if snap.DebtBalance != expectedDebt {
			t.Errorf("expected debt_balance %d, got %d", expectedDebt, snap.DebtBalance)
		}
		if snap.TotalNetWorth != expectedNetWorth {
			t.Errorf("expected total_net_worth %d, got %d", expectedNetWorth, snap.TotalNetWorth)
		}
	})

	t.Run("excludes_inactive_accounts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000) // active, $5,000

		// Create a cash account and then deactivate it — should not be counted
		inactiveAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 999999)
		db.Model(inactiveAcct).Update("is_active", false)

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.CashBalance != 500000 {
			t.Errorf("expected cash_balance 500000 (inactive excluded), got %d", snap.CashBalance)
		}
	})

	t.Run("idempotent_retry", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		recordedAt := time.Now().Truncate(time.Second)

		count1, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)
		if count1 != 1 {
			t.Fatalf("expected 1 on first call, got %d", count1)
		}

		// Second call with same recorded_at — should upsert, not fail
		count2, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)
		if count2 != 1 {
			t.Errorf("expected 1 on retry, got %d", count2)
		}

		// Verify only 1 snapshot exists
		var dbCount int64
		db.Model(&models.PortfolioSnapshot{}).Count(&dbCount)
		if dbCount != 1 {
			t.Errorf("expected 1 snapshot in DB after retry, got %d", dbCount)
		}
	})
}

func TestGetSnapshots(t *testing.T) {
	t.Run("returns_paginated", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		for i := 0; i < 5; i++ {
			db.Create(&models.PortfolioSnapshot{
				UserID:          user.ID,
				RecordedAt:      base.Add(time.Duration(i) * time.Hour),
				TotalNetWorth:   int64(100000 + i*10000),
				CashBalance:     int64(100000 + i*10000),
				InvestmentValue: 0,
				DebtBalance:     0,
			})
		}

		from := base.Add(-time.Hour)
		to := base.Add(10 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 2}

		result, err := svc.GetSnapshots(user.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page, got %d", len(result.Data))
		}
		if result.TotalItems != 5 {
			t.Errorf("expected total 5, got %d", result.TotalItems)
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
	})

	t.Run("filters_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		for i := 0; i < 4; i++ {
			db.Create(&models.PortfolioSnapshot{
				UserID:        user.ID,
				RecordedAt:    base.Add(time.Duration(i) * 24 * time.Hour),
				TotalNetWorth: int64(100000),
			})
		}

		// Query only the middle 2 days
		from := base.Add(12 * time.Hour)
		to := base.Add(60 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetSnapshots(user.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 items in date range, got %d", result.TotalItems)
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		recordedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		db.Create(&models.PortfolioSnapshot{
			UserID: user1.ID, RecordedAt: recordedAt, TotalNetWorth: 100000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user2.ID, RecordedAt: recordedAt, TotalNetWorth: 200000,
		})

		from := recordedAt.Add(-time.Hour)
		to := recordedAt.Add(time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetSnapshots(user1.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 snapshot for user1, got %d", result.TotalItems)
		}
		if len(result.Data) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Data))
		}
		if result.Data[0].TotalNetWorth != 100000 {
			t.Errorf("expected net_worth 100000, got %d", result.Data[0].TotalNetWorth)
		}
	})

	t.Run("ordered_by_recorded_at_desc", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		for i := 0; i < 3; i++ {
			db.Create(&models.PortfolioSnapshot{
				UserID:        user.ID,
				RecordedAt:    base.Add(time.Duration(i) * time.Hour),
				TotalNetWorth: int64(100000 + i*10000),
			})
		}

		from := base.Add(-time.Hour)
		to := base.Add(3 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetSnapshots(user.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.Data))
		}
		// Most recent first (i=2 → 120000)
		if result.Data[0].TotalNetWorth != 120000 {
			t.Errorf("expected first net_worth 120000 (most recent), got %d", result.Data[0].TotalNetWorth)
		}
		// Oldest last (i=0 → 100000)
		if result.Data[2].TotalNetWorth != 100000 {
			t.Errorf("expected last net_worth 100000 (oldest), got %d", result.Data[2].TotalNetWorth)
		}
	})
}

func TestGetGroupedSnapshots(t *testing.T) {
	t.Run("groups_by_day_keeps_last_per_day", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		// Day 1: 3 snapshots at 08:00, 12:00, 20:00
		day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day1.Add(8 * time.Hour),
			TotalNetWorth: 100000, CashBalance: 100000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day1.Add(12 * time.Hour),
			TotalNetWorth: 110000, CashBalance: 110000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day1.Add(20 * time.Hour),
			TotalNetWorth: 120000, CashBalance: 120000,
		})

		// Day 2: 3 snapshots at 09:00, 15:00, 21:00
		day2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day2.Add(9 * time.Hour),
			TotalNetWorth: 130000, CashBalance: 130000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day2.Add(15 * time.Hour),
			TotalNetWorth: 140000, CashBalance: 140000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: day2.Add(21 * time.Hour),
			TotalNetWorth: 150000, CashBalance: 150000,
		})

		from := day1
		to := day2.Add(24 * time.Hour)

		result, err := svc.GetGroupedSnapshots(user.ID, from, to, "day")
		testutil.AssertNoError(t, err)

		if len(result) != 2 {
			t.Fatalf("expected 2 daily snapshots, got %d", len(result))
		}
		// Day 1 last snapshot: 20:00 → 120000
		if result[0].TotalNetWorth != 120000 {
			t.Errorf("expected day1 net_worth 120000, got %d", result[0].TotalNetWorth)
		}
		// Day 2 last snapshot: 21:00 → 150000
		if result[1].TotalNetWorth != 150000 {
			t.Errorf("expected day2 net_worth 150000, got %d", result[1].TotalNetWorth)
		}
	})

	t.Run("groups_by_hour_keeps_last_per_hour", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		// Hour 10: snapshots at :00 and :30
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: base,
			TotalNetWorth: 100000, CashBalance: 100000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: base.Add(30 * time.Minute),
			TotalNetWorth: 110000, CashBalance: 110000,
		})

		// Hour 11: snapshots at :00 and :30
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: base.Add(1 * time.Hour),
			TotalNetWorth: 120000, CashBalance: 120000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user.ID, RecordedAt: base.Add(90 * time.Minute),
			TotalNetWorth: 130000, CashBalance: 130000,
		})

		from := base.Add(-time.Hour)
		to := base.Add(2 * time.Hour)

		result, err := svc.GetGroupedSnapshots(user.ID, from, to, "hour")
		testutil.AssertNoError(t, err)

		if len(result) != 2 {
			t.Fatalf("expected 2 hourly snapshots, got %d", len(result))
		}
		// Hour 10 last: :30 → 110000
		if result[0].TotalNetWorth != 110000 {
			t.Errorf("expected hour10 net_worth 110000, got %d", result[0].TotalNetWorth)
		}
		// Hour 11 last: :30 → 130000
		if result[1].TotalNetWorth != 130000 {
			t.Errorf("expected hour11 net_worth 130000, got %d", result[1].TotalNetWorth)
		}
	})

	t.Run("returns_chronological_order", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		// Create snapshots on 3 different days
		for i := 0; i < 3; i++ {
			db.Create(&models.PortfolioSnapshot{
				UserID:        user.ID,
				RecordedAt:    base.Add(time.Duration(i) * 24 * time.Hour),
				TotalNetWorth: int64(100000 + i*10000),
				CashBalance:   int64(100000 + i*10000),
			})
		}

		from := base.Add(-time.Hour)
		to := base.Add(72 * time.Hour)

		result, err := svc.GetGroupedSnapshots(user.ID, from, to, "day")
		testutil.AssertNoError(t, err)

		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}
		// Oldest first
		if result[0].TotalNetWorth != 100000 {
			t.Errorf("expected first net_worth 100000 (oldest), got %d", result[0].TotalNetWorth)
		}
		// Newest last
		if result[2].TotalNetWorth != 120000 {
			t.Errorf("expected last net_worth 120000 (newest), got %d", result[2].TotalNetWorth)
		}
	})

	t.Run("empty_range_returns_empty", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

		result, err := svc.GetGroupedSnapshots(user.ID, from, to, "day")
		testutil.AssertNoError(t, err)

		if len(result) != 0 {
			t.Errorf("expected 0 snapshots, got %d", len(result))
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		recordedAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		db.Create(&models.PortfolioSnapshot{
			UserID: user1.ID, RecordedAt: recordedAt,
			TotalNetWorth: 100000, CashBalance: 100000,
		})
		db.Create(&models.PortfolioSnapshot{
			UserID: user2.ID, RecordedAt: recordedAt,
			TotalNetWorth: 200000, CashBalance: 200000,
		})

		from := recordedAt.Add(-time.Hour)
		to := recordedAt.Add(time.Hour)

		result, err := svc.GetGroupedSnapshots(user1.ID, from, to, "day")
		testutil.AssertNoError(t, err)

		if len(result) != 1 {
			t.Fatalf("expected 1 snapshot for user1, got %d", len(result))
		}
		if result[0].TotalNetWorth != 100000 {
			t.Errorf("expected net_worth 100000, got %d", result[0].TotalNetWorth)
		}
	})
}
