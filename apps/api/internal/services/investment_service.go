package services

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// InvestmentStatus filters investments by position state.
type InvestmentStatus string

const (
	InvestmentStatusOpen   InvestmentStatus = "open"   // quantity > 0
	InvestmentStatusClosed InvestmentStatus = "closed" // quantity = 0
	InvestmentStatusAll    InvestmentStatus = "all"    // no filter
)

// applyStatusFilter applies the quantity clause that corresponds to the given status.
// Unknown values fall back to InvestmentStatusOpen so callers that omit the filter
// keep their original behaviour.
func applyStatusFilter(db *gorm.DB, status InvestmentStatus) *gorm.DB {
	switch status {
	case InvestmentStatusClosed:
		return db.Where("quantity = 0")
	case InvestmentStatusAll:
		return db
	default:
		return db.Where("quantity > 0")
	}
}

// getLatestPrices fetches the most recent price for each security ID from security_prices.
// Returns a map of security_id -> price (int64 cents). Securities with no price entries
// are not included in the map.
//
// It uses a correlated "ORDER BY recorded_at DESC LIMIT 1" subquery per
// security, which Postgres answers from the (security_id, recorded_at) unique
// index with one index probe per id instead of scanning every price row. The
// query is portable (it also runs on the SQLite used in tests).
func getLatestPrices(db *gorm.DB, securityIDs []string) (map[string]int64, error) {
	if len(securityIDs) == 0 {
		return map[string]int64{}, nil
	}

	type priceRow struct {
		SecurityID string
		Price      *int64
	}
	var rows []priceRow

	if err := db.Table("securities s").
		Select("s.id AS security_id, "+
			"(SELECT sp.price FROM security_prices sp WHERE sp.security_id = s.id "+
			"ORDER BY sp.recorded_at DESC LIMIT 1) AS price").
		Where("s.id IN ?", uniqueStrings(securityIDs)).
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.Price != nil {
			result[r.SecurityID] = *r.Price
		}
	}
	return result, nil
}

// uniqueStrings returns ids without duplicates, preserving first-seen order.
func uniqueStrings(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// getTotalInvestedByID returns the sum of buy transaction TotalAmount per investment ID.
// Used to expose the original capital deployed for closed positions, where CostBasis is
// driven to zero by proportional sell-side reductions.
func getTotalInvestedByID(db *gorm.DB, investmentIDs []string) (map[string]int64, error) {
	if len(investmentIDs) == 0 {
		return map[string]int64{}, nil
	}

	type row struct {
		InvestmentID string
		Total        int64
	}
	var rows []row

	if err := db.Table("investment_transactions").
		Select("investment_id, COALESCE(SUM(total_amount), 0) AS total").
		Where("investment_id IN ? AND type = ?", investmentIDs, models.InvestmentTransactionBuy).
		Group("investment_id").
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := make(map[string]int64, len(rows))
	for _, r := range rows {
		result[r.InvestmentID] = r.Total
	}
	return result, nil
}

// investmentListOrder gives paginated investment lists a deterministic order.
const investmentListOrder = "created_at ASC, id ASC"

// investmentService handles investment-related business logic.
type investmentService struct {
	db             *gorm.DB
	accountService AccountServicer
}

// NewInvestmentService creates a new InvestmentServicer.
func NewInvestmentService(db *gorm.DB, accountService AccountServicer) InvestmentServicer {
	return &investmentService{db: db, accountService: accountService}
}

// AddInvestment adds a new investment holding to an investment account.
func (s *investmentService) AddInvestment(
	userID, accountID, securityID string,
	quantity float64,
	purchasePrice int64,
	walletAddress string,
	date *time.Time,
	fee int64,
	notes string,
) (*models.Investment, error) {
	// Verify account exists, belongs to user, and is an investment account
	account, err := s.accountService.GetAccountByID(userID, accountID)
	if err != nil {
		return nil, err
	}
	if account.Type != models.AccountTypeInvestment {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Account is not an investment account")
	}

	// Verify security exists
	var security models.Security
	if err := s.db.Where("id = ?", securityID).First(&security).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSecurityNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Apply defaults for optional fields
	txDate := time.Now()
	if date != nil {
		txDate = *date
	}
	txNotes := "Initial purchase"
	if notes != "" {
		txNotes = notes
	}

	gross, err := lineAmount(quantity, purchasePrice)
	if err != nil {
		return nil, err
	}
	costBasis := gross + fee

	investment := &models.Investment{
		AccountID:     accountID,
		SecurityID:    securityID,
		Quantity:      quantity,
		CostBasis:     costBasis,
		WalletAddress: walletAddress,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if txErr := tx.Create(investment).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Create initial buy transaction
		invTx := &models.InvestmentTransaction{
			InvestmentID: investment.ID,
			Type:         models.InvestmentTransactionBuy,
			Date:         txDate,
			Quantity:     quantity,
			PricePerUnit: purchasePrice,
			TotalAmount:  costBasis,
			Fee:          fee,
			Notes:        txNotes,
		}
		if txErr := tx.Create(invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Populate current price from security_prices for the response
	prices, err := getLatestPrices(s.db, []string{securityID})
	if err != nil {
		return nil, err
	}
	investment.CurrentPrice = prices[securityID]

	investment.Security = security
	return investment, nil
}

// GetAccountInvestments returns a paginated list of investments for an account.
func (s *investmentService) GetAccountInvestments(userID, accountID string, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error) {
	// Verify account exists and belongs to user
	if _, err := s.accountService.GetAccountByID(userID, accountID); err != nil {
		return nil, err
	}

	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Investment{}).Where("account_id = ? AND quantity > 0", accountID)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var investments []models.Investment
	if err := s.db.Preload("Security").Where("account_id = ? AND quantity > 0", accountID).
		Order(investmentListOrder).
		Scopes(pagination.Paginate(page)).Find(&investments).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Batch populate current prices from security_prices
	secIDs := make([]string, 0, len(investments))
	invIDs := make([]string, 0, len(investments))
	for i := range investments {
		secIDs = append(secIDs, investments[i].SecurityID)
		invIDs = append(invIDs, investments[i].ID)
	}
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}
	invested, err := getTotalInvestedByID(s.db, invIDs)
	if err != nil {
		return nil, err
	}
	for i := range investments {
		investments[i].CurrentPrice = prices[investments[i].SecurityID]
		investments[i].TotalInvested = invested[investments[i].ID]
	}

	result := pagination.NewPageResponse(investments, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetAllInvestments returns a paginated list of investments across all active
// investment accounts for the given user, filtered by status (open/closed/all).
func (s *investmentService) GetAllInvestments(userID string, status InvestmentStatus, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error) {
	page.Defaults()

	// Find all active investment account IDs for the user
	var accountIDs []string
	if err := s.db.Model(&models.Account{}).
		Where("user_id = ? AND type = ? AND is_active = ?", userID, models.AccountTypeInvestment, true).
		Pluck("id", &accountIDs).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if len(accountIDs) == 0 {
		empty := pagination.NewPageResponse([]models.Investment{}, page.Page, page.PageSize, 0)
		return &empty, nil
	}

	var totalItems int64
	base := applyStatusFilter(
		s.db.Model(&models.Investment{}).Where("account_id IN ?", accountIDs),
		status,
	)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var investments []models.Investment
	query := applyStatusFilter(
		s.db.Preload("Security").Preload("Account").Where("account_id IN ?", accountIDs),
		status,
	)
	if err := query.Order(investmentListOrder).Scopes(pagination.Paginate(page)).Find(&investments).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Batch populate current prices from security_prices
	secIDs := make([]string, 0, len(investments))
	invIDs := make([]string, 0, len(investments))
	for i := range investments {
		secIDs = append(secIDs, investments[i].SecurityID)
		invIDs = append(invIDs, investments[i].ID)
	}
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}
	invested, err := getTotalInvestedByID(s.db, invIDs)
	if err != nil {
		return nil, err
	}
	for i := range investments {
		investments[i].CurrentPrice = prices[investments[i].SecurityID]
		investments[i].TotalInvested = invested[investments[i].ID]
	}

	result := pagination.NewPageResponse(investments, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetInvestmentByID returns an investment if the parent account belongs to the user.
func (s *investmentService) GetInvestmentByID(userID, investmentID string) (*models.Investment, error) {
	var investment models.Investment
	if err := s.db.Preload("Account").Preload("Security").Where("id = ?", investmentID).First(&investment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrInvestmentNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Verify account belongs to user
	if investment.Account.UserID != userID {
		return nil, apperrors.ErrInvestmentNotFound
	}

	// Populate current price from security_prices
	prices, err := getLatestPrices(s.db, []string{investment.SecurityID})
	if err != nil {
		return nil, err
	}
	investment.CurrentPrice = prices[investment.SecurityID]

	invested, err := getTotalInvestedByID(s.db, []string{investment.ID})
	if err != nil {
		return nil, err
	}
	investment.TotalInvested = invested[investment.ID]

	return &investment, nil
}

// GetPortfolio returns an aggregated portfolio summary across all investment accounts.
func (s *investmentService) GetPortfolio(userID string) (*PortfolioSummary, error) {
	// Get all investment accounts for the user
	var accounts []models.Account
	if err := s.db.Where("user_id = ? AND type = ? AND is_active = ?", userID, models.AccountTypeInvestment, true).
		Find(&accounts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	accountIDs := make([]string, len(accounts))
	for i := range accounts {
		accountIDs[i] = accounts[i].ID
	}

	summary := &PortfolioSummary{
		HoldingsByType: make(map[models.AssetType]TypeSummary),
	}

	if len(accountIDs) == 0 {
		return summary, nil
	}

	// Get all investments across those accounts with Security preloaded
	var investments []models.Investment
	if err := s.db.Preload("Security").Where("account_id IN ?", accountIDs).Find(&investments).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Batch fetch live prices from security_prices
	secIDs := make([]string, 0, len(investments))
	for i := range investments {
		secIDs = append(secIDs, investments[i].SecurityID)
	}
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}

	for i := range investments {
		inv := &investments[i]

		// Always include realized G/L from all positions (open + closed)
		summary.TotalRealizedGainLoss += inv.RealizedGainLoss

		// Only include open positions in holdings counts, values, and cost basis
		if inv.Quantity > 0 {
			value := valueCents(inv.Quantity, prices[inv.SecurityID])
			summary.TotalValue += value
			summary.TotalCostBasis += inv.CostBasis

			ts := summary.HoldingsByType[inv.Security.AssetType]
			ts.Value += value
			ts.Count++
			summary.HoldingsByType[inv.Security.AssetType] = ts
		}
	}

	summary.TotalGainLoss = summary.TotalValue - summary.TotalCostBasis
	if summary.TotalCostBasis > 0 {
		summary.GainLossPct = float64(summary.TotalGainLoss) / float64(summary.TotalCostBasis) * 100
	}

	return summary, nil
}

// RecordBuy records a buy transaction and updates the investment holding.
// If fundingAccountID is non-empty, the total cost is deducted from the specified cash account.
func (s *investmentService) RecordBuy(
	userID, investmentID string,
	date time.Time,
	quantity float64,
	pricePerUnit int64,
	fee int64,
	notes string,
	fundingAccountID string,
) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}

	// Validate funding account if specified
	var fundingAccount *models.Account
	if fundingAccountID != "" {
		account, accErr := s.accountService.GetAccountByID(userID, fundingAccountID)
		if accErr != nil {
			return nil, accErr
		}
		if account.Type != models.AccountTypeCash {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "funding account must be a cash account")
		}
		fundingAccount = account
	}

	gross, err := lineAmount(quantity, pricePerUnit)
	if err != nil {
		return nil, err
	}
	totalAmount := gross + fee

	var invTx models.InvestmentTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-read the holding under a row lock so concurrent writes serialize.
		locked, txErr := lockInvestment(tx, investmentID)
		if txErr != nil {
			return txErr
		}

		invTx = models.InvestmentTransaction{
			InvestmentID: investmentID,
			Type:         models.InvestmentTransactionBuy,
			Date:         date,
			Quantity:     quantity,
			PricePerUnit: pricePerUnit,
			TotalAmount:  totalAmount,
			Fee:          fee,
			Notes:        notes,
		}
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Update investment: quantity and cost basis increase
		newQuantity := locked.Quantity + quantity
		newCostBasis := locked.CostBasis + totalAmount
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(map[string]interface{}{
			"quantity":   newQuantity,
			"cost_basis": newCostBasis,
		}).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Deduct from funding account (row-locked, relative update; plan 021, D10)
		if fundingAccount != nil {
			accounts, txErr := lockAccounts(tx, userID, []string{fundingAccountID}, true)
			if txErr != nil {
				return txErr
			}
			locked := accounts[fundingAccountID]
			cashTx := models.Transaction{
				UserID:      userID,
				AccountID:   fundingAccountID,
				Type:        models.TransactionTypeInvestment,
				Amount:      totalAmount,
				Description: fmt.Sprintf("Investment purchase: %g %s", quantity, investment.Security.Name),
				Date:        date,
			}
			if txErr := tx.Create(&cashTx).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}
			if txErr := s.accountService.ApplyBalanceDelta(tx, locked.ID, locked.Type, models.TransactionTypeExpense, totalAmount); txErr != nil {
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &invTx, nil
}

// RecordSell records a sell transaction and adjusts the investment holding proportionally.
// If depositAccountID is non-empty, the net proceeds are also credited to the specified cash account.
func (s *investmentService) RecordSell(
	userID, investmentID string,
	date time.Time,
	quantity float64,
	pricePerUnit int64,
	fee int64,
	notes string,
	depositAccountID string,
) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}

	if quantity > investment.Quantity {
		return nil, apperrors.ErrInsufficientShares
	}

	// Validate deposit account if specified
	var depositAccount *models.Account
	if depositAccountID != "" {
		account, accErr := s.accountService.GetAccountByID(userID, depositAccountID)
		if accErr != nil {
			return nil, accErr
		}
		if account.Type != models.AccountTypeCash {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "deposit account must be a cash account")
		}
		depositAccount = account
	}

	gross, err := lineAmount(quantity, pricePerUnit)
	if err != nil {
		return nil, err
	}
	totalAmount := gross - fee

	var invTx models.InvestmentTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-read and re-validate the holding under a row lock so two concurrent
		// sells can never oversell the position.
		locked, txErr := lockInvestment(tx, investmentID)
		if txErr != nil {
			return txErr
		}
		if locked.Quantity <= 0 || quantity > locked.Quantity {
			return apperrors.ErrInsufficientShares
		}

		// Proportional cost basis reduction (rounded to the nearest cent; never
		// more than the remaining cost basis since quantity <= locked.Quantity)
		costBasisReduction := int64(math.Round(float64(locked.CostBasis) * (quantity / locked.Quantity)))

		// Realized gain/loss = sell proceeds - proportional cost basis
		realizedGainLoss := totalAmount - costBasisReduction

		invTx = models.InvestmentTransaction{
			InvestmentID:     investmentID,
			Type:             models.InvestmentTransactionSell,
			Date:             date,
			Quantity:         quantity,
			PricePerUnit:     pricePerUnit,
			TotalAmount:      totalAmount,
			Fee:              fee,
			Notes:            notes,
			RealizedGainLoss: realizedGainLoss,
		}
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		newQuantity := locked.Quantity - quantity
		newCostBasis := locked.CostBasis - costBasisReduction
		newRealizedGainLoss := locked.RealizedGainLoss + realizedGainLoss
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(map[string]interface{}{
			"quantity":           newQuantity,
			"cost_basis":         newCostBasis,
			"realized_gain_loss": newRealizedGainLoss,
		}).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Credit deposit account with net proceeds (row-locked, relative update)
		if depositAccount != nil {
			accounts, txErr := lockAccounts(tx, userID, []string{depositAccountID}, true)
			if txErr != nil {
				return txErr
			}
			locked := accounts[depositAccountID]
			cashTx := models.Transaction{
				UserID:      userID,
				AccountID:   depositAccountID,
				Type:        models.TransactionTypeInvestment,
				Amount:      totalAmount,
				Description: fmt.Sprintf("Sale proceeds: %g %s", quantity, investment.Security.Name),
				Date:        date,
			}
			if txErr := tx.Create(&cashTx).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}
			if txErr := s.accountService.ApplyBalanceDelta(tx, locked.ID, locked.Type, models.TransactionTypeIncome, totalAmount); txErr != nil {
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &invTx, nil
}

// RecordDividend records a dividend transaction without changing quantity or cost basis.
func (s *investmentService) RecordDividend(
	userID, investmentID string,
	date time.Time,
	amount int64,
	dividendType, notes string,
) (*models.InvestmentTransaction, error) {
	if _, err := s.GetInvestmentByID(userID, investmentID); err != nil {
		return nil, err
	}

	invTx := &models.InvestmentTransaction{
		InvestmentID: investmentID,
		Type:         models.InvestmentTransactionDividend,
		Date:         date,
		TotalAmount:  amount,
		DividendType: dividendType,
		Notes:        notes,
	}

	if err := s.db.Create(invTx).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return invTx, nil
}

// RecordSplit records a stock split and multiplies the investment quantity.
func (s *investmentService) RecordSplit(
	userID, investmentID string,
	date time.Time,
	splitRatio float64,
	notes string,
) (*models.InvestmentTransaction, error) {
	_, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}

	var invTx models.InvestmentTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-read the holding under a row lock so the split multiplies the
		// current quantity, not a stale pre-transaction read.
		locked, txErr := lockInvestment(tx, investmentID)
		if txErr != nil {
			return txErr
		}

		invTx = models.InvestmentTransaction{
			InvestmentID: investmentID,
			Type:         models.InvestmentTransactionSplit,
			Date:         date,
			Quantity:     locked.Quantity,
			SplitRatio:   splitRatio,
			Notes:        notes,
		}
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Multiply quantity by split ratio; cost basis stays the same
		newQuantity := locked.Quantity * splitRatio
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Update("quantity", newQuantity).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &invTx, nil
}

// GetInvestmentTransactions returns a paginated list of transactions for an investment.
func (s *investmentService) GetInvestmentTransactions(userID, investmentID string, page pagination.PageRequest) (*pagination.PageResponse[models.InvestmentTransaction], error) {
	// Verify investment exists and user owns it
	if _, err := s.GetInvestmentByID(userID, investmentID); err != nil {
		return nil, err
	}

	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.InvestmentTransaction{}).Where("investment_id = ?", investmentID)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var transactions []models.InvestmentTransaction
	if err := base.Order("date DESC").Scopes(pagination.Paginate(page)).Find(&transactions).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(transactions, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// lockInvestment re-reads an investment row with a row lock (SELECT ... FOR
// UPDATE on Postgres) inside an open DB transaction. Ownership must already
// have been checked by the caller (GetInvestmentByID).
func lockInvestment(tx *gorm.DB, investmentID string) (*models.Investment, error) {
	var inv models.Investment
	if err := lockUpdate(tx).Where("id = ?", investmentID).First(&inv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrInvestmentNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &inv, nil
}
