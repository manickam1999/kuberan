package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestAddInvestment(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")

		inv, err := svc.AddInvestment(user.ID, account.ID, sec.ID, 10.0, 15000, "", nil, 0, "")
		testutil.AssertNoError(t, err)

		if inv.ID == "" {
			t.Fatal("expected non-empty investment ID")
		}
		if inv.SecurityID != sec.ID {
			t.Errorf("expected security ID %s, got %s", sec.ID, inv.SecurityID)
		}
		if inv.Quantity != 10.0 {
			t.Errorf("expected quantity 10.0, got %f", inv.Quantity)
		}
		// CostBasis = 10 * 15000 = 150000
		if inv.CostBasis != 150000 {
			t.Errorf("expected cost basis 150000, got %d", inv.CostBasis)
		}
		// No security price exists yet, so CurrentPrice should be 0
		if inv.CurrentPrice != 0 {
			t.Errorf("expected current price 0 (no security price), got %d", inv.CurrentPrice)
		}

		// Verify initial buy transaction was created
		var txCount int64
		db.Model(&models.InvestmentTransaction{}).Where("investment_id = ?", inv.ID).Count(&txCount)
		if txCount != 1 {
			t.Errorf("expected 1 initial buy transaction, got %d", txCount)
		}

		var buyTx models.InvestmentTransaction
		db.Where("investment_id = ?", inv.ID).First(&buyTx)
		if buyTx.Type != models.InvestmentTransactionBuy {
			t.Errorf("expected buy transaction, got %s", buyTx.Type)
		}
		if buyTx.Quantity != 10.0 {
			t.Errorf("expected buy quantity 10.0, got %f", buyTx.Quantity)
		}
		if buyTx.TotalAmount != 150000 {
			t.Errorf("expected buy total 150000, got %d", buyTx.TotalAmount)
		}
	})

	t.Run("not_investment_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		cashAcct := testutil.CreateTestCashAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		_, err := svc.AddInvestment(user.ID, cashAcct.ID, sec.ID, 10.0, 15000, "", nil, 0, "")
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("invalid_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		sec := testutil.CreateTestSecurity(t, db)

		_, err := svc.AddInvestment(user.ID, "9999", sec.ID, 10.0, 15000, "", nil, 0, "")
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("invalid_security", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		_, err := svc.AddInvestment(user.ID, account.ID, "9999", 10.0, 15000, "", nil, 0, "")
		testutil.AssertAppError(t, err, "SECURITY_NOT_FOUND")
	})

	t.Run("custom_date", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		customDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		inv, err := svc.AddInvestment(user.ID, account.ID, sec.ID, 5.0, 20000, "", &customDate, 0, "")
		testutil.AssertNoError(t, err)

		// Verify initial buy transaction uses the custom date
		var buyTx models.InvestmentTransaction
		db.Where("investment_id = ?", inv.ID).First(&buyTx)
		if !buyTx.Date.Equal(customDate) {
			t.Errorf("expected buy date %v, got %v", customDate, buyTx.Date)
		}
		if buyTx.Notes != "Initial purchase" {
			t.Errorf("expected default notes 'Initial purchase', got %q", buyTx.Notes)
		}
	})

	t.Run("custom_fee_and_notes", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		inv, err := svc.AddInvestment(user.ID, account.ID, sec.ID, 10.0, 15000, "", nil, 500, "Bought via broker")
		testutil.AssertNoError(t, err)

		// CostBasis should include fee: 10 * 15000 + 500 = 150500
		if inv.CostBasis != 150500 {
			t.Errorf("expected cost basis 150500, got %d", inv.CostBasis)
		}

		// Verify buy transaction has custom fee and notes
		var buyTx models.InvestmentTransaction
		db.Where("investment_id = ?", inv.ID).First(&buyTx)
		if buyTx.Fee != 500 {
			t.Errorf("expected fee 500, got %d", buyTx.Fee)
		}
		if buyTx.Notes != "Bought via broker" {
			t.Errorf("expected notes 'Bought via broker', got %q", buyTx.Notes)
		}
		// TotalAmount should also include fee: 10 * 15000 + 500 = 150500
		if buyTx.TotalAmount != 150500 {
			t.Errorf("expected total amount 150500, got %d", buyTx.TotalAmount)
		}
	})

	t.Run("defaults_when_omitted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		beforeCreate := time.Now().Add(-time.Second)
		inv, err := svc.AddInvestment(user.ID, account.ID, sec.ID, 10.0, 15000, "", nil, 0, "")
		testutil.AssertNoError(t, err)
		afterCreate := time.Now().Add(time.Second)

		// CostBasis = 10 * 15000 + 0 = 150000
		if inv.CostBasis != 150000 {
			t.Errorf("expected cost basis 150000, got %d", inv.CostBasis)
		}

		// Verify buy transaction has defaults
		var buyTx models.InvestmentTransaction
		db.Where("investment_id = ?", inv.ID).First(&buyTx)
		if buyTx.Fee != 0 {
			t.Errorf("expected fee 0, got %d", buyTx.Fee)
		}
		if buyTx.Notes != "Initial purchase" {
			t.Errorf("expected notes 'Initial purchase', got %q", buyTx.Notes)
		}
		if buyTx.Date.Before(beforeCreate) || buyTx.Date.After(afterCreate) {
			t.Errorf("expected date near now, got %v", buyTx.Date)
		}
	})
}

func TestGetInvestmentByID(t *testing.T) {
	t.Run("found_with_live_price", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		// Record a security price
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 15000, time.Now())

		result, err := svc.GetInvestmentByID(user.ID, inv.ID)
		testutil.AssertNoError(t, err)

		if result.ID != inv.ID {
			t.Errorf("expected ID %s, got %s", inv.ID, result.ID)
		}
		if result.SecurityID != sec.ID {
			t.Errorf("expected security ID %s, got %s", sec.ID, result.SecurityID)
		}
		if result.CurrentPrice != 15000 {
			t.Errorf("expected current price 15000 from security_prices, got %d", result.CurrentPrice)
		}
	})

	t.Run("no_price_returns_zero", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		result, err := svc.GetInvestmentByID(user.ID, inv.ID)
		testutil.AssertNoError(t, err)

		if result.CurrentPrice != 0 {
			t.Errorf("expected current price 0 when no security price exists, got %d", result.CurrentPrice)
		}
	})

	t.Run("returns_latest_price", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		// Create multiple prices at different timestamps
		base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 10000, base)
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 12000, base.Add(time.Hour))
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 15000, base.Add(2*time.Hour))

		result, err := svc.GetInvestmentByID(user.ID, inv.ID)
		testutil.AssertNoError(t, err)

		if result.CurrentPrice != 15000 {
			t.Errorf("expected latest price 15000, got %d", result.CurrentPrice)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetInvestmentByID(user.ID, "9999")
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user1.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		_, err := svc.GetInvestmentByID(user2.ID, inv.ID)
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})
}

func TestGetAccountInvestments(t *testing.T) {
	t.Run("returns_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec1 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, account.ID, sec1.ID)
		sec2 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, account.ID, sec2.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAccountInvestments(user.ID, account.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 investments, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items in data, got %d", len(result.Data))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		for i := 0; i < 5; i++ {
			sec := testutil.CreateTestSecurity(t, db)
			testutil.CreateTestInvestment(t, db, account.ID, sec.ID)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := svc.GetAccountInvestments(user.ID, account.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected total 5, got %d", result.TotalItems)
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page, got %d", len(result.Data))
		}
	})

	t.Run("invalid_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		_, err := svc.GetAccountInvestments(user.ID, "9999", page)
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("excludes_closed_positions", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		// Open position
		sec1 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, account.ID, sec1.ID)

		// Closed position (quantity = 0)
		sec2 := testutil.CreateTestSecurity(t, db)
		closedInv := &models.Investment{
			AccountID:        account.ID,
			SecurityID:       sec2.ID,
			Quantity:         0,
			CostBasis:        0,
			RealizedGainLoss: 30000,
		}
		if err := db.Create(closedInv).Error; err != nil {
			t.Fatalf("failed to create closed investment: %v", err)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAccountInvestments(user.ID, account.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 investment (open only), got %d", result.TotalItems)
		}
		if len(result.Data) != 1 {
			t.Errorf("expected 1 item in data, got %d", len(result.Data))
		}
	})
}

func TestRecordBuy(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares @ $100, cost basis $1000

		buyTx, err := svc.RecordBuy(user.ID, inv.ID, time.Now(), 5.0, 10000, 500, "Buy more", "")
		testutil.AssertNoError(t, err)

		if buyTx.Type != models.InvestmentTransactionBuy {
			t.Errorf("expected type buy, got %s", buyTx.Type)
		}
		if buyTx.Quantity != 5.0 {
			t.Errorf("expected quantity 5.0, got %f", buyTx.Quantity)
		}
		// TotalAmount = 5 * 10000 + 500 = 50500
		if buyTx.TotalAmount != 50500 {
			t.Errorf("expected total 50500, got %d", buyTx.TotalAmount)
		}
		if buyTx.Fee != 500 {
			t.Errorf("expected fee 500, got %d", buyTx.Fee)
		}

		// Verify investment updated in DB
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		// 10 + 5 = 15 shares
		if dbInv.Quantity != 15.0 {
			t.Errorf("expected quantity 15.0, got %f", dbInv.Quantity)
		}
		// 100000 + 50500 = 150500 cents cost basis
		if dbInv.CostBasis != 150500 {
			t.Errorf("expected cost basis 150500, got %d", dbInv.CostBasis)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.RecordBuy(user.ID, "9999", time.Now(), 5.0, 10000, 0, "", "")
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})
}

func TestRecordSell(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares, cost basis 100000

		sellTx, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 4.0, 12000, 300, "Sell some", "")
		testutil.AssertNoError(t, err)

		if sellTx.Type != models.InvestmentTransactionSell {
			t.Errorf("expected type sell, got %s", sellTx.Type)
		}
		if sellTx.Quantity != 4.0 {
			t.Errorf("expected quantity 4.0, got %f", sellTx.Quantity)
		}
		// TotalAmount = 4 * 12000 - 300 = 47700
		if sellTx.TotalAmount != 47700 {
			t.Errorf("expected total 47700, got %d", sellTx.TotalAmount)
		}

		// Verify investment updated in DB
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		// 10 - 4 = 6 shares remaining
		if dbInv.Quantity != 6.0 {
			t.Errorf("expected quantity 6.0, got %f", dbInv.Quantity)
		}
		// Proportional reduction: 100000 * (4/10) = 40000 removed, leaving 60000
		if dbInv.CostBasis != 60000 {
			t.Errorf("expected cost basis 60000, got %d", dbInv.CostBasis)
		}
	})

	t.Run("computes_realized_gain_loss_on_sell", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares @ $100, cost basis 100000

		// Sell 5 shares at $150/share, no fee
		// totalAmount = 5 * 15000 - 0 = 75000
		// costBasisReduction = 100000 * (5/10) = 50000
		// realizedGainLoss = 75000 - 50000 = 25000
		sellTx, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 5.0, 15000, 0, "Sell half at profit", "")
		testutil.AssertNoError(t, err)

		if sellTx.RealizedGainLoss != 25000 {
			t.Errorf("expected tx realized gain/loss 25000, got %d", sellTx.RealizedGainLoss)
		}

		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.RealizedGainLoss != 25000 {
			t.Errorf("expected investment realized gain/loss 25000, got %d", dbInv.RealizedGainLoss)
		}
	})

	t.Run("accumulates_realized_gain_loss_on_investment", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares @ $100, cost basis 100000

		// Sell 1: 3 shares at $120, no fee
		// totalAmount = 3 * 12000 = 36000
		// costBasisReduction = 100000 * (3/10) = 30000
		// realizedGL1 = 36000 - 30000 = 6000
		sell1, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 3.0, 12000, 0, "Sell 1", "")
		testutil.AssertNoError(t, err)
		if sell1.RealizedGainLoss != 6000 {
			t.Errorf("expected sell1 realized gain/loss 6000, got %d", sell1.RealizedGainLoss)
		}

		// After sell 1: 7 shares, cost basis 70000, realized 6000

		// Sell 2: 2 shares at $80, no fee
		// totalAmount = 2 * 8000 = 16000
		// costBasisReduction = 70000 * (2/7) = 20000
		// realizedGL2 = 16000 - 20000 = -4000
		sell2, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 2.0, 8000, 0, "Sell 2", "")
		testutil.AssertNoError(t, err)
		if sell2.RealizedGainLoss != -4000 {
			t.Errorf("expected sell2 realized gain/loss -4000, got %d", sell2.RealizedGainLoss)
		}

		// Investment should have accumulated: 6000 + (-4000) = 2000
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.RealizedGainLoss != 2000 {
			t.Errorf("expected accumulated realized gain/loss 2000, got %d", dbInv.RealizedGainLoss)
		}
	})

	t.Run("realized_gain_loss_for_losing_trade", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares @ $100, cost basis 100000

		// Sell 10 shares at $50/share, no fee (full liquidation at a loss)
		// totalAmount = 10 * 5000 = 50000
		// costBasisReduction = 100000 * (10/10) = 100000
		// realizedGainLoss = 50000 - 100000 = -50000
		sellTx, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 10.0, 5000, 0, "Sell all at loss", "")
		testutil.AssertNoError(t, err)

		if sellTx.RealizedGainLoss != -50000 {
			t.Errorf("expected tx realized gain/loss -50000, got %d", sellTx.RealizedGainLoss)
		}

		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.RealizedGainLoss != -50000 {
			t.Errorf("expected investment realized gain/loss -50000, got %d", dbInv.RealizedGainLoss)
		}
		if dbInv.Quantity != 0 {
			t.Errorf("expected quantity 0, got %f", dbInv.Quantity)
		}
		if dbInv.CostBasis != 0 {
			t.Errorf("expected cost basis 0, got %d", dbInv.CostBasis)
		}
	})

	t.Run("insufficient_shares", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares

		_, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 15.0, 12000, 0, "Too many", "")
		testutil.AssertAppError(t, err, "INSUFFICIENT_SHARES")

		// Verify quantity unchanged
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.Quantity != 10.0 {
			t.Errorf("expected quantity unchanged at 10.0, got %f", dbInv.Quantity)
		}
	})

	t.Run("sell_all_shares", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares, cost basis 100000

		// Sell all 10 at $120/share, no fee
		// totalAmount = 10 * 12000 = 120000
		// costBasisReduction = 100000 * (10/10) = 100000
		// realizedGainLoss = 120000 - 100000 = 20000
		sellTx, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 10.0, 12000, 0, "Sell all", "")
		testutil.AssertNoError(t, err)

		if sellTx.RealizedGainLoss != 20000 {
			t.Errorf("expected tx realized gain/loss 20000, got %d", sellTx.RealizedGainLoss)
		}

		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.Quantity != 0.0 {
			t.Errorf("expected quantity 0.0, got %f", dbInv.Quantity)
		}
		if dbInv.CostBasis != 0 {
			t.Errorf("expected cost basis 0, got %d", dbInv.CostBasis)
		}
		if dbInv.RealizedGainLoss != 20000 {
			t.Errorf("expected realized gain/loss 20000, got %d", dbInv.RealizedGainLoss)
		}
	})
}

func TestRecordDividend(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares, cost basis 100000

		divTx, err := svc.RecordDividend(user.ID, inv.ID, time.Now(), 5000, "Cash", "Q4 dividend")
		testutil.AssertNoError(t, err)

		if divTx.Type != models.InvestmentTransactionDividend {
			t.Errorf("expected type dividend, got %s", divTx.Type)
		}
		if divTx.TotalAmount != 5000 {
			t.Errorf("expected total 5000, got %d", divTx.TotalAmount)
		}
		if divTx.DividendType != "Cash" {
			t.Errorf("expected dividend type Cash, got %s", divTx.DividendType)
		}

		// Verify investment quantity and cost basis unchanged
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.Quantity != 10.0 {
			t.Errorf("expected quantity unchanged at 10.0, got %f", dbInv.Quantity)
		}
		if dbInv.CostBasis != 100000 {
			t.Errorf("expected cost basis unchanged at 100000, got %d", dbInv.CostBasis)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.RecordDividend(user.ID, "9999", time.Now(), 5000, "Cash", "")
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})
}

func TestRecordSplit(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // 10 shares, cost basis 100000

		splitTx, err := svc.RecordSplit(user.ID, inv.ID, time.Now(), 2.0, "2-for-1 split")
		testutil.AssertNoError(t, err)

		if splitTx.Type != models.InvestmentTransactionSplit {
			t.Errorf("expected type split, got %s", splitTx.Type)
		}
		if splitTx.SplitRatio != 2.0 {
			t.Errorf("expected split ratio 2.0, got %f", splitTx.SplitRatio)
		}

		// Verify quantity doubled, cost basis unchanged
		var dbInv models.Investment
		db.First(&dbInv, "id = ?", inv.ID)
		if dbInv.Quantity != 20.0 {
			t.Errorf("expected quantity 20.0 after 2:1 split, got %f", dbInv.Quantity)
		}
		if dbInv.CostBasis != 100000 {
			t.Errorf("expected cost basis unchanged at 100000, got %d", dbInv.CostBasis)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.RecordSplit(user.ID, "9999", time.Now(), 2.0, "")
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})
}

func TestGetPortfolio(t *testing.T) {
	t.Run("aggregation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		// Create two investment accounts
		acct1 := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		acct2 := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		// Account 1: stock - 10 shares, cost basis $1000
		sec1 := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestInvestment(t, db, acct1.ID, sec1.ID)
		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 10000, time.Now()) // $100/share

		// Account 2: ETF - 20 shares, cost basis $2000
		secETF := testutil.CreateTestSecurityWithParams(t, db, "VTI", "Vanguard Total", models.AssetTypeETF, "NYSE")
		etfInv := &models.Investment{
			AccountID:  acct2.ID,
			SecurityID: secETF.ID,
			Quantity:   20.0,
			CostBasis:  200000, // $2000
		}
		if err := db.Create(etfInv).Error; err != nil {
			t.Fatalf("failed to create ETF investment: %v", err)
		}
		testutil.CreateTestSecurityPrice(t, db, secETF.ID, 12000, time.Now()) // $120/share

		portfolio, err := svc.GetPortfolio(user.ID)
		testutil.AssertNoError(t, err)

		// Stock value: 10 * 10000 = 100000
		// ETF value: 20 * 12000 = 240000
		// Total value: 340000
		expectedTotalValue := int64(340000)
		if portfolio.TotalValue != expectedTotalValue {
			t.Errorf("expected total value %d, got %d", expectedTotalValue, portfolio.TotalValue)
		}

		// Total cost basis: 100000 + 200000 = 300000
		expectedCostBasis := int64(300000)
		if portfolio.TotalCostBasis != expectedCostBasis {
			t.Errorf("expected total cost basis %d, got %d", expectedCostBasis, portfolio.TotalCostBasis)
		}

		// Gain/loss: 340000 - 300000 = 40000
		expectedGainLoss := int64(40000)
		if portfolio.TotalGainLoss != expectedGainLoss {
			t.Errorf("expected gain/loss %d, got %d", expectedGainLoss, portfolio.TotalGainLoss)
		}

		// Gain/loss %: 40000 / 300000 * 100 ~= 13.33%
		expectedPct := float64(40000) / float64(300000) * 100
		if portfolio.GainLossPct < expectedPct-0.01 || portfolio.GainLossPct > expectedPct+0.01 {
			t.Errorf("expected gain/loss pct ~%.2f, got %.2f", expectedPct, portfolio.GainLossPct)
		}

		// Holdings by type
		stockSummary, ok := portfolio.HoldingsByType[models.AssetTypeStock]
		if !ok {
			t.Fatal("expected stock type in holdings")
		}
		if stockSummary.Count != 1 {
			t.Errorf("expected 1 stock holding, got %d", stockSummary.Count)
		}
		if stockSummary.Value != 100000 {
			t.Errorf("expected stock value 100000, got %d", stockSummary.Value)
		}

		etfSummary, ok := portfolio.HoldingsByType[models.AssetTypeETF]
		if !ok {
			t.Fatal("expected ETF type in holdings")
		}
		if etfSummary.Count != 1 {
			t.Errorf("expected 1 ETF holding, got %d", etfSummary.Count)
		}
		if etfSummary.Value != 240000 {
			t.Errorf("expected ETF value 240000, got %d", etfSummary.Value)
		}

		// No sells yet, so TotalRealizedGainLoss should be 0
		if portfolio.TotalRealizedGainLoss != 0 {
			t.Errorf("expected total realized gain/loss 0, got %d", portfolio.TotalRealizedGainLoss)
		}
	})

	t.Run("includes_realized_gain_loss", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		sec1 := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		sec2 := testutil.CreateTestSecurityWithParams(t, db, "GOOG", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")

		inv1 := testutil.CreateTestInvestment(t, db, acct.ID, sec1.ID) // 10 shares, cost basis 100000
		inv2 := testutil.CreateTestInvestment(t, db, acct.ID, sec2.ID) // 10 shares, cost basis 100000

		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 10000, time.Now())
		testutil.CreateTestSecurityPrice(t, db, sec2.ID, 10000, time.Now())

		// Sell 5 shares of AAPL at $150 (profit)
		// totalAmount = 5 * 15000 = 75000, costBasisReduction = 50000, realized = 25000
		_, err := svc.RecordSell(user.ID, inv1.ID, time.Now(), 5.0, 15000, 0, "", "")
		testutil.AssertNoError(t, err)

		// Sell 3 shares of GOOG at $80 (loss)
		// totalAmount = 3 * 8000 = 24000, costBasisReduction = 30000, realized = -6000
		_, err = svc.RecordSell(user.ID, inv2.ID, time.Now(), 3.0, 8000, 0, "", "")
		testutil.AssertNoError(t, err)

		portfolio, err := svc.GetPortfolio(user.ID)
		testutil.AssertNoError(t, err)

		// Total realized: 25000 + (-6000) = 19000
		if portfolio.TotalRealizedGainLoss != 19000 {
			t.Errorf("expected total realized gain/loss 19000, got %d", portfolio.TotalRealizedGainLoss)
		}
	})

	t.Run("no_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		portfolio, err := svc.GetPortfolio(user.ID)
		testutil.AssertNoError(t, err)

		if portfolio.TotalValue != 0 {
			t.Errorf("expected total value 0, got %d", portfolio.TotalValue)
		}
		if portfolio.TotalCostBasis != 0 {
			t.Errorf("expected cost basis 0, got %d", portfolio.TotalCostBasis)
		}
		if len(portfolio.HoldingsByType) != 0 {
			t.Errorf("expected empty holdings map, got %d entries", len(portfolio.HoldingsByType))
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)

		acct1 := testutil.CreateTestInvestmentAccount(t, db, user1.ID)
		sec1 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, acct1.ID, sec1.ID)
		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 10000, time.Now())

		acct2 := testutil.CreateTestInvestmentAccount(t, db, user2.ID)
		sec2 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, acct2.ID, sec2.ID)
		testutil.CreateTestSecurityPrice(t, db, sec2.ID, 10000, time.Now())

		portfolio1, err := svc.GetPortfolio(user1.ID)
		testutil.AssertNoError(t, err)

		// Each user has 1 investment with 10 shares @ $100 = $1000 value
		if portfolio1.TotalValue != 100000 {
			t.Errorf("expected user1 total value 100000, got %d", portfolio1.TotalValue)
		}

		portfolio2, err := svc.GetPortfolio(user2.ID)
		testutil.AssertNoError(t, err)
		if portfolio2.TotalValue != 100000 {
			t.Errorf("expected user2 total value 100000, got %d", portfolio2.TotalValue)
		}
	})

	t.Run("excludes_closed_from_count_but_includes_realized_gl", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		// Open position: 10 shares of AAPL, cost basis $1000
		secOpen := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		testutil.CreateTestInvestment(t, db, acct.ID, secOpen.ID)
		testutil.CreateTestSecurityPrice(t, db, secOpen.ID, 15000, time.Now()) // $150/share

		// Closed position: 0 shares of GOOG, realized G/L of $500
		secClosed := testutil.CreateTestSecurityWithParams(t, db, "GOOG", "Alphabet Inc", models.AssetTypeStock, "NASDAQ")
		closedInv := &models.Investment{
			AccountID:        acct.ID,
			SecurityID:       secClosed.ID,
			Quantity:         0,
			CostBasis:        0,
			RealizedGainLoss: 50000, // $500 realized gain
		}
		if err := db.Create(closedInv).Error; err != nil {
			t.Fatalf("failed to create closed investment: %v", err)
		}
		testutil.CreateTestSecurityPrice(t, db, secClosed.ID, 20000, time.Now())

		portfolio, err := svc.GetPortfolio(user.ID)
		testutil.AssertNoError(t, err)

		// TotalValue should only include open position: 10 * 15000 = 150000
		if portfolio.TotalValue != 150000 {
			t.Errorf("expected total value 150000 (open only), got %d", portfolio.TotalValue)
		}

		// TotalCostBasis should only include open position: 100000
		if portfolio.TotalCostBasis != 100000 {
			t.Errorf("expected total cost basis 100000 (open only), got %d", portfolio.TotalCostBasis)
		}

		// TotalRealizedGainLoss should include closed position: 50000
		if portfolio.TotalRealizedGainLoss != 50000 {
			t.Errorf("expected total realized gain/loss 50000 (from closed), got %d", portfolio.TotalRealizedGainLoss)
		}

		// Holdings by type should only count 1 (the open position)
		stockSummary, ok := portfolio.HoldingsByType[models.AssetTypeStock]
		if !ok {
			t.Fatal("expected stock type in holdings")
		}
		if stockSummary.Count != 1 {
			t.Errorf("expected 1 stock holding (open only), got %d", stockSummary.Count)
		}
		if stockSummary.Value != 150000 {
			t.Errorf("expected stock value 150000, got %d", stockSummary.Value)
		}
	})
}

func TestGetAllInvestments(t *testing.T) {
	t.Run("returns_investments_across_accounts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		acct1 := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		acct2 := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		sec1 := testutil.CreateTestSecurityWithParams(t, db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
		sec2 := testutil.CreateTestSecurityWithParams(t, db, "VTI", "Vanguard Total", models.AssetTypeETF, "NYSE")
		testutil.CreateTestInvestment(t, db, acct1.ID, sec1.ID)
		testutil.CreateTestInvestment(t, db, acct2.ID, sec2.ID)

		testutil.CreateTestSecurityPrice(t, db, sec1.ID, 15000, time.Now())
		testutil.CreateTestSecurityPrice(t, db, sec2.ID, 12000, time.Now())

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAllInvestments(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 investments, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items in data, got %d", len(result.Data))
		}

		// Verify CurrentPrice, Security, and Account are populated
		for _, inv := range result.Data {
			if inv.CurrentPrice == 0 {
				t.Errorf("expected non-zero CurrentPrice for investment %s", inv.ID)
			}
			if inv.Security.Symbol == "" {
				t.Errorf("expected Security preloaded for investment %s", inv.ID)
			}
			if inv.Account.Name == "" {
				t.Errorf("expected Account preloaded for investment %s", inv.ID)
			}
		}
	})

	t.Run("returns_empty_for_no_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAllInvestments(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 0 {
			t.Errorf("expected 0 total items, got %d", result.TotalItems)
		}
		if len(result.Data) != 0 {
			t.Errorf("expected empty data, got %d items", len(result.Data))
		}
	})

	t.Run("paginates_results", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		for i := 0; i < 5; i++ {
			sec := testutil.CreateTestSecurity(t, db)
			testutil.CreateTestInvestment(t, db, acct.ID, sec.ID)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := svc.GetAllInvestments(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected total 5, got %d", result.TotalItems)
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page 1, got %d", len(result.Data))
		}

		page2 := pagination.PageRequest{Page: 3, PageSize: 2}
		result2, err := svc.GetAllInvestments(user.ID, page2)
		testutil.AssertNoError(t, err)
		if len(result2.Data) != 1 {
			t.Errorf("expected 1 item on page 3, got %d", len(result2.Data))
		}
	})

	t.Run("excludes_inactive_accounts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		activeAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		inactiveAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		// Deactivate one account
		db.Model(&inactiveAcct).Update("is_active", false)

		sec1 := testutil.CreateTestSecurity(t, db)
		sec2 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, activeAcct.ID, sec1.ID)
		testutil.CreateTestInvestment(t, db, inactiveAcct.ID, sec2.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAllInvestments(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 investment (from active account only), got %d", result.TotalItems)
		}
	})

	t.Run("excludes_closed_positions", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		// Open position (quantity > 0)
		sec1 := testutil.CreateTestSecurity(t, db)
		testutil.CreateTestInvestment(t, db, acct.ID, sec1.ID)

		// Closed position (quantity = 0)
		sec2 := testutil.CreateTestSecurity(t, db)
		closedInv := &models.Investment{
			AccountID:        acct.ID,
			SecurityID:       sec2.ID,
			Quantity:         0,
			CostBasis:        0,
			RealizedGainLoss: 50000,
		}
		if err := db.Create(closedInv).Error; err != nil {
			t.Fatalf("failed to create closed investment: %v", err)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetAllInvestments(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 investment (open only), got %d", result.TotalItems)
		}
		if len(result.Data) != 1 {
			t.Errorf("expected 1 item in data, got %d", len(result.Data))
		}
	})
}

func TestGetInvestmentTransactions(t *testing.T) {
	t.Run("returns_transactions", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)
		inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		// Record some transactions
		_, err := svc.RecordBuy(user.ID, inv.ID, time.Now(), 5.0, 10000, 0, "Buy 1", "")
		testutil.AssertNoError(t, err)
		_, err = svc.RecordDividend(user.ID, inv.ID, time.Now(), 2000, "Cash", "Div")
		testutil.AssertNoError(t, err)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetInvestmentTransactions(user.ID, inv.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 transactions, got %d", result.TotalItems)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		svc := NewInvestmentService(db, acctSvc)
		user := testutil.CreateTestUser(t, db)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		_, err := svc.GetInvestmentTransactions(user.ID, "9999", page)
		testutil.AssertAppError(t, err, "INVESTMENT_NOT_FOUND")
	})
}
