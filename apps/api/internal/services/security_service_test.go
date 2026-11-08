package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
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

		res, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)

		if count := res.Recorded; count != 3 {
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

		res1, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)
		if count1 := res1.Recorded; count1 != 1 {
			t.Errorf("expected 1 on first insert, got %d", count1)
		}

		// Insert same price again — should not create duplicate
		res2, err := svc.RecordPrices(prices)
		testutil.AssertNoError(t, err)
		if count2 := res2.Recorded; count2 != 0 {
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

func TestRecordPrices_Currency(t *testing.T) {
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	newSec := func(t *testing.T, db *gorm.DB, currency string) *models.Security {
		t.Helper()
		sec := testutil.CreateTestSecurity(t, db)
		testutil.AssertNoError(t, db.Model(sec).Update("currency", currency).Error)
		return sec
	}

	t.Run("empty_currency_is_stored_as_security_currency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		sec := newSec(t, db, "usd") // legacy lower-case labels still resolve
		res, err := NewSecurityService(db).RecordPrices([]SecurityPriceInput{{SecurityID: sec.ID, Price: 100, RecordedAt: at}})
		testutil.AssertNoError(t, err)
		if res.Recorded != 1 || res.Rejected == nil || len(res.Rejected) != 0 {
			t.Fatalf("result = %+v", res)
		}
		var row models.SecurityPrice
		testutil.AssertNoError(t, db.Where("security_id = ?", sec.ID).First(&row).Error)
		if row.Currency != "USD" {
			t.Errorf("stored currency = %q, want USD", row.Currency)
		}
	})

	t.Run("matching_currency_is_case_insensitive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		sec := newSec(t, db, "MYR")
		res, err := NewSecurityService(db).RecordPrices([]SecurityPriceInput{{SecurityID: sec.ID, Price: 100, Currency: "myr", RecordedAt: at}})
		testutil.AssertNoError(t, err)
		if res.Recorded != 1 || len(res.Rejected) != 0 {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("mismatch_and_unknown_are_rejected_and_the_rest_inserted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		myr := newSec(t, db, "MYR")
		usd := newSec(t, db, "USD")
		res, err := NewSecurityService(db).RecordPrices([]SecurityPriceInput{
			{SecurityID: myr.ID, Price: 1, RecordedAt: at},
			{SecurityID: usd.ID, Price: 2, Currency: "MYR", RecordedAt: at},
			{SecurityID: "00000000-0000-0000-0000-0000000000ff", Price: 3, RecordedAt: at},
			{SecurityID: "not-a-uuid", Price: 4, RecordedAt: at},
			{SecurityID: usd.ID, Price: 5, Currency: "USD", RecordedAt: at},
		})
		testutil.AssertNoError(t, err)
		if res.Recorded != 2 || len(res.Rejected) != 3 {
			t.Fatalf("result = %+v", res)
		}
		mm := res.Rejected[0]
		if mm.Index != 1 || mm.Reason != PriceRejectedCurrencyMismatch || mm.Currency != "MYR" || mm.ExpectedCurrency != "USD" || mm.SecurityID != usd.ID {
			t.Errorf("mismatch entry = %+v", mm)
		}
		if res.Rejected[1].Index != 2 || res.Rejected[1].Reason != PriceRejectedUnknownSecurity ||
			res.Rejected[2].Index != 3 || res.Rejected[2].Reason != PriceRejectedUnknownSecurity {
			t.Errorf("unknown entries = %+v", res.Rejected[1:])
		}
		var n int64
		db.Model(&models.SecurityPrice{}).Count(&n)
		if n != 2 {
			t.Errorf("rows = %d, want 2", n)
		}
	})

	t.Run("all_rejected_is_not_an_error", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		sec := newSec(t, db, "MYR")
		res, err := NewSecurityService(db).RecordPrices([]SecurityPriceInput{{SecurityID: sec.ID, Price: 1, Currency: "USD", RecordedAt: at}})
		testutil.AssertNoError(t, err)
		if res.Recorded != 0 || len(res.Rejected) != 1 {
			t.Fatalf("result = %+v", res)
		}
	})
}

func TestUpdateSecurity(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	setup := func(t *testing.T) (SecurityServicer, *gorm.DB, *models.Security) {
		t.Helper()
		db := testutil.SetupTestDB(t)
		t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
		sec := testutil.CreateTestSecurity(t, db)
		testutil.AssertNoError(t, db.Model(sec).Update("currency", "MYR").Error)
		return NewSecurityService(db), db, sec
	}
	priceCount := func(db *gorm.DB, id string) int64 {
		var n int64
		db.Model(&models.SecurityPrice{}).Where("security_id = ?", id).Count(&n)
		return n
	}

	t.Run("currency_locked_without_reset", func(t *testing.T) {
		svc, db, sec := setup(t)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, at)
		_, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Currency: strPtr("USD")})
		testutil.AssertAppError(t, err, apperrors.ErrSecurityCurrencyLocked.Code)
		got, _ := svc.GetSecurityByID(sec.ID)
		if got.Currency != "MYR" || priceCount(db, sec.ID) != 1 {
			t.Errorf("nothing may change: currency=%s prices=%d", got.Currency, priceCount(db, sec.ID))
		}
	})

	t.Run("reset_deletes_only_this_securitys_prices", func(t *testing.T) {
		svc, db, sec := setup(t)
		other := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, at)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 101, at.Add(time.Hour))
		testutil.CreateTestSecurityPrice(t, db, other.ID, 7, at)

		res, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Currency: strPtr("usd"), ResetPrices: true})
		testutil.AssertNoError(t, err)
		if res.PricesDeleted != 2 || res.Security.Currency != "USD" {
			t.Fatalf("result = %+v %+v", res, res.Security)
		}
		if priceCount(db, sec.ID) != 0 || priceCount(db, other.ID) != 1 {
			t.Errorf("prices: target=%d other=%d", priceCount(db, sec.ID), priceCount(db, other.ID))
		}
	})

	t.Run("currency_change_without_prices_needs_no_reset", func(t *testing.T) {
		svc, _, sec := setup(t)
		res, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Currency: strPtr("USD")})
		testutil.AssertNoError(t, err)
		if res.Security.Currency != "USD" || res.PricesDeleted != 0 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("same_currency_with_prices_is_not_locked", func(t *testing.T) {
		svc, db, sec := setup(t)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, at)
		res, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Currency: strPtr("myr"), Name: strPtr("Renamed")})
		testutil.AssertNoError(t, err)
		if res.Security.Name != "Renamed" || priceCount(db, sec.ID) != 1 {
			t.Errorf("result = %+v prices=%d", res.Security, priceCount(db, sec.ID))
		}
	})

	t.Run("reset_without_currency_change_is_invalid", func(t *testing.T) {
		svc, db, sec := setup(t)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, at)
		for _, u := range []SecurityUpdate{
			{ResetPrices: true, Name: strPtr("x")},
			{ResetPrices: true, Currency: strPtr("MYR")},
		} {
			_, err := svc.UpdateSecurity(sec.ID, u)
			testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
		}
		if priceCount(db, sec.ID) != 1 {
			t.Error("prices must survive an invalid reset")
		}
	})

	t.Run("exchange_and_provider_symbol", func(t *testing.T) {
		svc, _, sec := setup(t)
		res, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Exchange: strPtr("LSE"), ProviderSymbol: strPtr("RBOT.L")})
		testutil.AssertNoError(t, err)
		if res.Security.Exchange != "LSE" || res.Security.ProviderSymbol != "RBOT.L" {
			t.Errorf("security = %+v", res.Security)
		}
	})

	t.Run("exchange_conflict", func(t *testing.T) {
		svc, db, sec := setup(t)
		twin := testutil.CreateTestSecurityWithParams(t, db, sec.Symbol, "Twin", models.AssetTypeStock, "LSE")
		_, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{Exchange: strPtr("LSE")})
		testutil.AssertAppError(t, err, apperrors.ErrDuplicateSecurity.Code)
		got, _ := svc.GetSecurityByID(sec.ID)
		if got.Exchange != sec.Exchange || twin.ID == sec.ID {
			t.Errorf("exchange must be unchanged, got %q", got.Exchange)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		svc, _, _ := setup(t)
		_, err := svc.UpdateSecurity("00000000-0000-0000-0000-0000000000ff", SecurityUpdate{Name: strPtr("x")})
		testutil.AssertAppError(t, err, apperrors.ErrSecurityNotFound.Code)
	})

	t.Run("requires_a_field_and_a_name", func(t *testing.T) {
		svc, _, sec := setup(t)
		_, err := svc.UpdateSecurity(sec.ID, SecurityUpdate{})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
		_, err = svc.UpdateSecurity(sec.ID, SecurityUpdate{Name: strPtr("  ")})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})
}
