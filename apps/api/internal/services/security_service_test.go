package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateSecurity(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec, err := svc.CreateSecurity("AAPL", "Apple Inc", models.AssetTypeStock, "USD", "NASDAQ", nil)
		testutil.AssertNoError(t, err)

		if sec.ID == "" {
			t.Fatal("expected non-empty security ID")
		}
		if sec.Symbol != "AAPL" {
			t.Errorf("expected symbol AAPL, got %s", sec.Symbol)
		}
		if sec.Name != "Apple Inc" {
			t.Errorf("expected name Apple Inc, got %s", sec.Name)
		}
		if sec.AssetType != models.AssetTypeStock {
			t.Errorf("expected asset type stock, got %s", sec.AssetType)
		}
		if sec.Currency != "USD" {
			t.Errorf("expected currency USD, got %s", sec.Currency)
		}
		if sec.Exchange != "NASDAQ" {
			t.Errorf("expected exchange NASDAQ, got %s", sec.Exchange)
		}
	})

	t.Run("with_extra_fields", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		maturity := time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC)
		extra := map[string]interface{}{
			"maturity_date":     &maturity,
			"yield_to_maturity": 4.5,
			"coupon_rate":       3.25,
		}

		sec, err := svc.CreateSecurity("TBOND1", "Treasury Bond 2030", models.AssetTypeBond, "USD", "", extra)
		testutil.AssertNoError(t, err)

		if sec.MaturityDate == nil {
			t.Fatal("expected maturity_date to be set")
		}
		if !sec.MaturityDate.Equal(maturity) {
			t.Errorf("expected maturity_date %v, got %v", maturity, *sec.MaturityDate)
		}
		if sec.YieldToMaturity != 4.5 {
			t.Errorf("expected yield_to_maturity 4.5, got %f", sec.YieldToMaturity)
		}
		if sec.CouponRate != 3.25 {
			t.Errorf("expected coupon_rate 3.25, got %f", sec.CouponRate)
		}
	})

	t.Run("with_provider_symbol", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		extra := map[string]interface{}{
			"provider_symbol": "1023.KL",
		}

		sec, err := svc.CreateSecurity("CIMB", "CIMB Group", models.AssetTypeStock, "MYR", "BURSA", extra)
		testutil.AssertNoError(t, err)

		if sec.ProviderSymbol != "1023.KL" {
			t.Errorf("expected provider_symbol 1023.KL, got %s", sec.ProviderSymbol)
		}

		// Verify it persists by re-reading
		fetched, err := svc.GetSecurityByID(sec.ID)
		testutil.AssertNoError(t, err)
		if fetched.ProviderSymbol != "1023.KL" {
			t.Errorf("expected persisted provider_symbol 1023.KL, got %s", fetched.ProviderSymbol)
		}
	})

	t.Run("duplicate_symbol_exchange", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.CreateSecurity("AAPL", "Apple Inc", models.AssetTypeStock, "USD", "NASDAQ", nil)
		testutil.AssertNoError(t, err)

		_, err = svc.CreateSecurity("AAPL", "Apple Inc Copy", models.AssetTypeStock, "USD", "NASDAQ", nil)
		testutil.AssertAppError(t, err, "DUPLICATE_SECURITY")
	})

	t.Run("same_symbol_different_exchange", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.CreateSecurity("AAPL", "Apple NYSE", models.AssetTypeStock, "USD", "NYSE", nil)
		testutil.AssertNoError(t, err)

		sec2, err := svc.CreateSecurity("AAPL", "Apple NASDAQ", models.AssetTypeStock, "USD", "NASDAQ", nil)
		testutil.AssertNoError(t, err)

		if sec2.ID == "" {
			t.Fatal("expected second security to be created successfully")
		}
	})

	t.Run("empty_symbol", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.CreateSecurity("", "Some Name", models.AssetTypeStock, "USD", "NYSE", nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("empty_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.CreateSecurity("SYM", "", models.AssetTypeStock, "USD", "NYSE", nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("defaults_currency_to_usd", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec, err := svc.CreateSecurity("VTI", "Vanguard Total", models.AssetTypeETF, "", "NYSE", nil)
		testutil.AssertNoError(t, err)

		if sec.Currency != "USD" {
			t.Errorf("expected currency USD, got %s", sec.Currency)
		}
	})
}

func TestGetSecurityByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		created := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")

		sec, err := svc.GetSecurityByID(created.ID)
		testutil.AssertNoError(t, err)

		if sec.ID != created.ID {
			t.Errorf("expected ID %s, got %s", created.ID, sec.ID)
		}
		if sec.Symbol != "AAPL" {
			t.Errorf("expected symbol AAPL, got %s", sec.Symbol)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.GetSecurityByID("9999")
		testutil.AssertAppError(t, err, "SECURITY_NOT_FOUND")
	})
}

func TestListSecurities(t *testing.T) {
	t.Run("returns_paginated", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		for i := 0; i < 5; i++ {
			testutil.CreateTestSecurity(t, db)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := svc.ListSecurities("", page)
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

	t.Run("ordered_by_symbol", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "ZZZ", "Zzz Corp", models.AssetTypeStock, "NYSE")
		testutil.CreateTestSecurityWithParams(t, db, "AAA", "Aaa Corp", models.AssetTypeStock, "NYSE")
		testutil.CreateTestSecurityWithParams(t, db, "MMM", "Mmm Corp", models.AssetTypeStock, "NYSE")

		page := pagination.PageRequest{Page: 1, PageSize: 10}
		result, err := svc.ListSecurities("", page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.Data))
		}
		if result.Data[0].Symbol != "AAA" {
			t.Errorf("expected first symbol AAA, got %s", result.Data[0].Symbol)
		}
		if result.Data[1].Symbol != "MMM" {
			t.Errorf("expected second symbol MMM, got %s", result.Data[1].Symbol)
		}
		if result.Data[2].Symbol != "ZZZ" {
			t.Errorf("expected third symbol ZZZ, got %s", result.Data[2].Symbol)
		}
	})

	t.Run("search_by_symbol", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "GOOGL", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "MSFT", "Microsoft Corp", models.AssetTypeStock, "NASDAQ")

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.ListSecurities("aapl", page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 result for 'aapl', got %d", result.TotalItems)
		}
		if len(result.Data) != 1 || result.Data[0].Symbol != "AAPL" {
			t.Errorf("expected AAPL, got %v", result.Data)
		}
	})

	t.Run("search_by_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "GOOGL", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.ListSecurities("apple", page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 result for 'apple', got %d", result.TotalItems)
		}
		if len(result.Data) != 1 || result.Data[0].Symbol != "AAPL" {
			t.Errorf("expected AAPL, got %v", result.Data)
		}
	})

	t.Run("search_case_insensitive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "GOOGL", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")

		page := pagination.PageRequest{Page: 1, PageSize: 20}

		upper, err := svc.ListSecurities("AAPL", page)
		testutil.AssertNoError(t, err)

		lower, err := svc.ListSecurities("aapl", page)
		testutil.AssertNoError(t, err)

		if upper.TotalItems != lower.TotalItems {
			t.Errorf("case-insensitive mismatch: upper=%d, lower=%d", upper.TotalItems, lower.TotalItems)
		}
		if upper.TotalItems != 1 {
			t.Errorf("expected 1 result, got %d", upper.TotalItems)
		}
	})

	t.Run("search_empty_returns_all", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "GOOGL", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "MSFT", "Microsoft Corp", models.AssetTypeStock, "NASDAQ")

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.ListSecurities("", page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 3 {
			t.Errorf("expected 3 results for empty search, got %d", result.TotalItems)
		}
	})
}

func TestListAllSecurities(t *testing.T) {
	t.Run("returns_all_securities", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		testutil.CreateTestSecurityWithParams(t, db, "MSFT", "Microsoft Corp", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestSecurityWithParams(t, db, "GOOGL", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")

		securities, err := svc.ListAllSecurities()
		testutil.AssertNoError(t, err)

		if len(securities) != 3 {
			t.Fatalf("expected 3 securities, got %d", len(securities))
		}
		// Verify ordered by symbol ASC
		if securities[0].Symbol != "AAPL" {
			t.Errorf("expected first symbol AAPL, got %s", securities[0].Symbol)
		}
		if securities[1].Symbol != "GOOGL" {
			t.Errorf("expected second symbol GOOGL, got %s", securities[1].Symbol)
		}
		if securities[2].Symbol != "MSFT" {
			t.Errorf("expected third symbol MSFT, got %s", securities[2].Symbol)
		}
	})

	t.Run("returns_empty_when_none", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		securities, err := svc.ListAllSecurities()
		testutil.AssertNoError(t, err)

		if securities == nil {
			t.Fatal("expected non-nil empty slice, got nil")
		}
		if len(securities) != 0 {
			t.Errorf("expected 0 securities, got %d", len(securities))
		}
	})

	t.Run("excludes_soft_deleted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		active := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		deleted := testutil.CreateTestSecurityWithParams(t, db, "GONE", "Deleted Corp", models.AssetTypeStock, "NYSE")

		// Soft-delete one security
		db.Delete(deleted)

		securities, err := svc.ListAllSecurities()
		testutil.AssertNoError(t, err)

		if len(securities) != 1 {
			t.Fatalf("expected 1 security, got %d", len(securities))
		}
		if securities[0].ID != active.ID {
			t.Errorf("expected security ID %s, got %s", active.ID, securities[0].ID)
		}
	})
}

func TestRecordPrices(t *testing.T) {
	t.Run("valid_bulk_insert", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec1 := testutil.CreateTestSecurity(t, db)
		sec2 := testutil.CreateTestSecurity(t, db)
		now := time.Now().Truncate(time.Second)

		prices := []SecurityPriceInput{
			{SecurityID: sec1.ID, Price: 15000, RecordedAt: now},
			{SecurityID: sec1.ID, Price: 15100, RecordedAt: now.Add(time.Hour)},
			{SecurityID: sec2.ID, Price: 4200, RecordedAt: now},
		}

		count, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		if count != 3 {
			t.Errorf("expected 3 prices recorded, got %d", count)
		}

		// Verify in DB
		var dbCount int64
		db.Model(&models.SecurityPrice{}).Count(&dbCount)
		if dbCount != 3 {
			t.Errorf("expected 3 rows in DB, got %d", dbCount)
		}
	})

	t.Run("idempotent_retry", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec := testutil.CreateTestSecurity(t, db)
		now := time.Now().Truncate(time.Second)

		prices := []SecurityPriceInput{
			{SecurityID: sec.ID, Price: 15000, RecordedAt: now},
		}

		count1, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)
		if count1 != 1 {
			t.Errorf("expected 1 on first insert, got %d", count1)
		}

		// Insert same price again — should not create duplicate
		count2, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)
		if count2 != 0 {
			t.Errorf("expected 0 on duplicate insert, got %d", count2)
		}

		// Verify only 1 row exists
		var dbCount int64
		db.Model(&models.SecurityPrice{}).Count(&dbCount)
		if dbCount != 1 {
			t.Errorf("expected 1 row in DB after retry, got %d", dbCount)
		}
	})

	t.Run("empty_input", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		_, err := svc.RecordPrices([]SecurityPriceInput{})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})
}

func TestGetPriceHistory(t *testing.T) {
	t.Run("returns_paginated", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec := testutil.CreateTestSecurity(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		var prices []SecurityPriceInput
		for i := 0; i < 5; i++ {
			prices = append(prices, SecurityPriceInput{
				SecurityID: sec.ID,
				Price:      int64(15000 + i*100),
				RecordedAt: base.Add(time.Duration(i) * time.Hour),
			})
		}
		_, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		from := base.Add(-time.Hour)
		to := base.Add(10 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 2}

		result, err := svc.GetPriceHistory(sec.ID, from, to, page)
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
		svc := NewSecurityService(db)

		sec := testutil.CreateTestSecurity(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		prices := []SecurityPriceInput{
			{SecurityID: sec.ID, Price: 15000, RecordedAt: base},
			{SecurityID: sec.ID, Price: 15100, RecordedAt: base.Add(24 * time.Hour)},
			{SecurityID: sec.ID, Price: 15200, RecordedAt: base.Add(48 * time.Hour)},
			{SecurityID: sec.ID, Price: 15300, RecordedAt: base.Add(72 * time.Hour)},
		}
		_, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		// Query only the middle 2 days
		from := base.Add(12 * time.Hour)
		to := base.Add(60 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetPriceHistory(sec.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 items in date range, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_security", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec1 := testutil.CreateTestSecurity(t, db)
		sec2 := testutil.CreateTestSecurity(t, db)
		now := time.Now().Truncate(time.Second)

		prices := []SecurityPriceInput{
			{SecurityID: sec1.ID, Price: 15000, RecordedAt: now},
			{SecurityID: sec1.ID, Price: 15100, RecordedAt: now.Add(time.Hour)},
			{SecurityID: sec2.ID, Price: 4200, RecordedAt: now},
		}
		_, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		from := now.Add(-time.Hour)
		to := now.Add(2 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetPriceHistory(sec1.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 prices for sec1, got %d", result.TotalItems)
		}
	})

	t.Run("ordered_by_recorded_at_desc", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewSecurityService(db)

		sec := testutil.CreateTestSecurity(t, db)
		base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		prices := []SecurityPriceInput{
			{SecurityID: sec.ID, Price: 15000, RecordedAt: base},
			{SecurityID: sec.ID, Price: 15100, RecordedAt: base.Add(time.Hour)},
			{SecurityID: sec.ID, Price: 15200, RecordedAt: base.Add(2 * time.Hour)},
		}
		_, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		from := base.Add(-time.Hour)
		to := base.Add(3 * time.Hour)
		page := pagination.PageRequest{Page: 1, PageSize: 20}

		result, err := svc.GetPriceHistory(sec.ID, from, to, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.Data))
		}
		// Most recent first
		if result.Data[0].Price != 15200 {
			t.Errorf("expected first price 15200 (most recent), got %d", result.Data[0].Price)
		}
		if result.Data[2].Price != 15000 {
			t.Errorf("expected last price 15000 (oldest), got %d", result.Data[2].Price)
		}
	})
}
