package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
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
	fxService      FXServicer
}

// NewInvestmentService creates a new InvestmentServicer. fxs converts native
// security prices into the user's base currency; production wiring always
// passes the FX service. A nil fxs is allowed in tests only: then only
// same-currency conversion succeeds.
func NewInvestmentService(db *gorm.DB, accountService AccountServicer, fxs FXServicer) InvestmentServicer {
	return &investmentService{db: db, accountService: accountService, fxService: fxs}
}

// populatePrices fills the price fields of the given investments: the latest
// native price, and CurrentPrice converted to the user's base currency (a unit
// price conversion, which is fine here because it is display only). With no
// FX rate CurrentPrice is 0 and FXMissing is set. Security must be loaded.
func (s *investmentService) populatePrices(userID string, investments []*models.Investment) error {
	if len(investments) == 0 {
		return nil
	}
	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return err
	}
	secIDs := make([]string, len(investments))
	for i, inv := range investments {
		secIDs[i] = inv.SecurityID
	}
	prices, err := latestNativePrices(s.db, secIDs)
	if err != nil {
		return err
	}
	v, err := newValuer(s.fxService, base, time.Now(), distinctPriceCurrencies(prices))
	if err != nil {
		return err
	}
	for _, inv := range investments {
		inv.Currency = normalizeCode(inv.Security.Currency)
		p, ok := prices[inv.SecurityID]
		if !ok {
			inv.PriceCurrency = inv.Currency
			continue
		}
		inv.CurrentPriceNative = p.Price
		inv.PriceCurrency = p.Currency
		converted, quote, ok := v.Convert(p.Price, p.Currency)
		if !ok {
			inv.CurrentPrice = 0
			inv.FXMissing = true
			continue
		}
		inv.CurrentPrice = converted
		if quote != nil {
			inv.FXRate = quote.Rate.String()
			inv.FXStale = quote.Stale
		}
	}
	return nil
}

// investmentPtrs returns pointers to the elements of investments.
func investmentPtrs(investments []models.Investment) []*models.Investment {
	out := make([]*models.Investment, len(investments))
	for i := range investments {
		out[i] = &investments[i]
	}
	return out
}

// AddInvestment adds a new investment holding to an investment account. The
// price and fee are in the security's currency; the initial buy's base-currency
// figures use the trade FX rate (an explicit p.FXRate, else 1 for a security in
// the base currency, else the market rate on the trade date).
func (s *investmentService) AddInvestment(userID string, p AddInvestmentParams) (*models.Investment, error) {
	// Verify account exists, belongs to user, and is an investment account
	account, err := s.accountService.GetAccountByID(userID, p.AccountID)
	if err != nil {
		return nil, err
	}
	if account.Type != models.AccountTypeInvestment {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "Account is not an investment account")
	}

	// Verify security exists
	var security models.Security
	if err := s.db.Where("id = ?", p.SecurityID).First(&security).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSecurityNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Apply defaults for optional fields
	txDate := time.Now()
	if p.Date != nil {
		txDate = *p.Date
	}
	txNotes := "Initial purchase"
	if p.Notes != "" {
		txNotes = p.Notes
	}

	explicit, err := parseOptionalRate(p.FXRate)
	if err != nil {
		return nil, err
	}
	gross, err := lineAmount(p.Quantity, p.PurchasePrice)
	if err != nil {
		return nil, err
	}
	totalAmount := gross + p.Fee

	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}
	rate, err := s.tradeRate(base, normalizeCode(security.Currency), txDate, explicit, nil)
	if err != nil {
		return nil, err
	}
	baseAmount, err := baseAmountFor(totalAmount, rate)
	if err != nil {
		return nil, err
	}

	var st holdingState
	if _, err := st.apply(tradeEvent{
		Type: models.InvestmentTransactionBuy, Quantity: p.Quantity, TotalAmount: totalAmount, BaseAmount: baseAmount,
	}); err != nil {
		return nil, err
	}

	investment := &models.Investment{
		AccountID:     p.AccountID,
		SecurityID:    p.SecurityID,
		Quantity:      st.Quantity,
		CostBasis:     st.CostBasis,
		CostBasisBase: st.CostBasisBase,
		WalletAddress: p.WalletAddress,
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
			Quantity:     p.Quantity,
			PricePerUnit: p.PurchasePrice,
			TotalAmount:  totalAmount,
			Fee:          p.Fee,
			Notes:        txNotes,
			FXRateToBase: &rate,
			BaseAmount:   &baseAmount,
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
	investment.Security = security
	if err := s.populatePrices(userID, []*models.Investment{investment}); err != nil {
		return nil, err
	}
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
	invIDs := make([]string, 0, len(investments))
	for i := range investments {
		invIDs = append(invIDs, investments[i].ID)
	}
	if err := s.populatePrices(userID, investmentPtrs(investments)); err != nil {
		return nil, err
	}
	invested, err := getTotalInvestedByID(s.db, invIDs)
	if err != nil {
		return nil, err
	}
	for i := range investments {
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
	invIDs := make([]string, 0, len(investments))
	for i := range investments {
		invIDs = append(invIDs, investments[i].ID)
	}
	if err := s.populatePrices(userID, investmentPtrs(investments)); err != nil {
		return nil, err
	}
	invested, err := getTotalInvestedByID(s.db, invIDs)
	if err != nil {
		return nil, err
	}
	for i := range investments {
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
	if err := s.populatePrices(userID, []*models.Investment{&investment}); err != nil {
		return nil, err
	}

	invested, err := getTotalInvestedByID(s.db, []string{investment.ID})
	if err != nil {
		return nil, err
	}
	investment.TotalInvested = invested[investment.ID]

	return &investment, nil
}

// GetPortfolio returns an aggregated portfolio summary across all investment accounts,
// valued in the user's base currency. Each holding is valued in its native currency
// first (quantity × price) and the line value is converted once. Holdings with no
// FX rate are left out of the totals and listed in Unconverted.
//
// Cost basis is the stored base-currency cost (frozen at each buy's rate). The
// unrealized gain splits into a price return (the native gain converted at
// today's rate) and an FX return (today's rate on the native cost versus the
// frozen base cost); per holding they add up to value − cost basis within one
// cent of rounding.
func (s *investmentService) GetPortfolio(userID string) (*PortfolioSummary, error) {
	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}

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
		Currency:       base,
		Unconverted:    []UnconvertedValue{},
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
	prices, err := latestNativePrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}
	// Quote every currency an open holding is valued or costed in: the price
	// currencies, plus the security currencies (cost basis converts even for a
	// holding with no price yet).
	currencies := distinctPriceCurrencies(prices)
	for i := range investments {
		if investments[i].Quantity > 0 {
			currencies = append(currencies, normalizeCode(investments[i].Security.Currency))
		}
	}
	v, err := newValuer(s.fxService, base, time.Now(), sortedUnique(currencies))
	if err != nil {
		return nil, err
	}

	unconverted := map[string]int64{}
	for i := range investments {
		inv := &investments[i]

		// Always include realized G/L from all positions (open + closed)
		summary.TotalRealizedGainLoss += inv.RealizedGainLossBase

		// Only include open positions in holdings counts, values, and cost basis
		if inv.Quantity <= 0 {
			continue
		}
		cur := normalizeCode(inv.Security.Currency)
		var native int64
		if p, priced := prices[inv.SecurityID]; priced {
			native = valueCents(inv.Quantity, p.Price)
			cur = p.Currency
		}
		value, quote, ok := v.Convert(native, cur)
		priceReturn, _, okPrice := v.Convert(native-inv.CostBasis, cur)
		costNow, _, okCost := v.Convert(inv.CostBasis, cur)
		if !ok || !okPrice || !okCost {
			unconverted[cur] += native
			continue
		}
		if quote != nil && quote.Stale {
			summary.FXStale = true
		}
		summary.TotalValue += value
		summary.TotalCostBasis += inv.CostBasisBase
		summary.TotalPriceReturn += priceReturn
		summary.TotalFXReturn += costNow - inv.CostBasisBase

		ts := summary.HoldingsByType[inv.Security.AssetType]
		ts.Value += value
		ts.Count++
		summary.HoldingsByType[inv.Security.AssetType] = ts
	}

	for _, c := range sortedUnique(mapKeys(unconverted)) {
		summary.Unconverted = append(summary.Unconverted, UnconvertedValue{Currency: c, Value: unconverted[c]})
	}

	summary.TotalGainLoss = summary.TotalValue - summary.TotalCostBasis
	if summary.TotalCostBasis > 0 {
		summary.GainLossPct = float64(summary.TotalGainLoss) / float64(summary.TotalCostBasis) * 100
	}

	return summary, nil
}

// mapKeys returns the keys of m.
func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// cashLegRequest names the optional cash account that settles a trade.
type cashLegRequest struct {
	accountID string
	amount    int64  // caller-stated amount in the account's currency; 0 = not given
	field     string // request field carrying amount ("funding_amount" / "deposit_amount")
	native    int64  // the amount, in the security's currency, that the leg settles
}

// resolveCashLeg validates the cash account of a trade and returns it with the
// amount to move in the ACCOUNT's currency. With no account it returns nil. In
// the security's currency the amount is the native one (a stated amount must
// match it); otherwise the caller must state the amount, else FX_AMOUNT_REQUIRED
// is returned with a market-rate suggestion.
func (s *investmentService) resolveCashLeg(userID string, req cashLegRequest, secCcy string, date time.Time) (*models.Account, int64, error) {
	if req.amount < 0 {
		return nil, 0, apperrors.WithMessage(apperrors.ErrInvalidInput, req.field+" must be greater than zero")
	}
	if req.accountID == "" {
		if req.amount != 0 {
			return nil, 0, apperrors.WithMessage(apperrors.ErrInvalidInput, req.field+" requires an account")
		}
		return nil, 0, nil
	}
	account, err := s.accountService.GetAccountByID(userID, req.accountID)
	if err != nil {
		return nil, 0, err
	}
	if account.Type != models.AccountTypeCash {
		kind := "funding"
		if req.field == "deposit_amount" {
			kind = "deposit"
		}
		return nil, 0, apperrors.WithMessage(apperrors.ErrInvalidInput, kind+" account must be a cash account")
	}
	cur := normalizeCode(account.Currency)
	if cur == secCcy {
		if req.amount != 0 && req.amount != req.native {
			return nil, 0, apperrors.WithMessage(apperrors.ErrInvalidInput,
				fmt.Sprintf("%s must equal %d when the account is in the security's currency", req.field, req.native))
		}
		return account, req.native, nil
	}
	if req.amount == 0 {
		return nil, 0, fxAmountRequiredError(s.fxService, secCcy, cur, req.native, date, req.field)
	}
	return account, req.amount, nil
}

// applyCashLeg writes the cash transaction row and moves the (row-locked)
// account's balance by amount, in the account's own currency.
func (s *investmentService) applyCashLeg(tx *gorm.DB, userID, accountID string, txnType models.TransactionType, amount int64, description string, date time.Time) error {
	accounts, err := lockAccounts(tx, userID, []string{accountID}, true)
	if err != nil {
		return err
	}
	acct := accounts[accountID]
	cashTx := models.Transaction{
		UserID:      userID,
		AccountID:   accountID,
		Type:        models.TransactionTypeInvestment,
		Amount:      amount,
		Description: description,
		Date:        date,
	}
	if err := tx.Create(&cashTx).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return s.accountService.ApplyBalanceDelta(tx, acct.ID, acct.Type, txnType, amount)
}

// RecordBuy records a buy transaction and updates the investment holding. The
// price and fee are in the security's currency. If p.FundingAccountID is set the
// cost is deducted from that cash account: in the account's own currency, which
// must be stated in p.FundingAmount when it differs from the security's.
func (s *investmentService) RecordBuy(userID, investmentID string, p BuyParams) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}
	secCcy := normalizeCode(investment.Security.Currency)

	explicit, err := parseOptionalRate(p.FXRate)
	if err != nil {
		return nil, err
	}
	gross, err := lineAmount(p.Quantity, p.PricePerUnit)
	if err != nil {
		return nil, err
	}
	totalAmount := gross + p.Fee

	fundingAccount, cashAmount, err := s.resolveCashLeg(userID, cashLegRequest{
		accountID: p.FundingAccountID, amount: p.FundingAmount, field: "funding_amount", native: totalAmount,
	}, secCcy, p.Date)
	if err != nil {
		return nil, err
	}

	rate, baseAmount, err := s.priceTrade(userID, secCcy, p.Date, explicit, fundingAccount, cashAmount, totalAmount, totalAmount)
	if err != nil {
		return nil, err
	}

	var invTx models.InvestmentTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-read the holding under a row lock so concurrent writes serialize.
		locked, txErr := lockInvestment(tx, investmentID)
		if txErr != nil {
			return txErr
		}
		st := stateOf(locked)
		if _, txErr := st.apply(tradeEvent{
			Type: models.InvestmentTransactionBuy, Quantity: p.Quantity, TotalAmount: totalAmount, BaseAmount: baseAmount,
		}); txErr != nil {
			return txErr
		}

		invTx = models.InvestmentTransaction{
			InvestmentID: investmentID,
			Type:         models.InvestmentTransactionBuy,
			Date:         p.Date,
			Quantity:     p.Quantity,
			PricePerUnit: p.PricePerUnit,
			TotalAmount:  totalAmount,
			Fee:          p.Fee,
			Notes:        p.Notes,
			FXRateToBase: &rate,
			BaseAmount:   &baseAmount,
		}
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(st.columns()).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Deduct from funding account (row-locked, relative update; plan 021, D10)
		if fundingAccount != nil {
			return s.applyCashLeg(tx, userID, fundingAccount.ID, models.TransactionTypeExpense, cashAmount,
				fmt.Sprintf("Investment purchase: %g %s", p.Quantity, investment.Security.Name), p.Date)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	invTx.Currency = secCcy
	return &invTx, nil
}

// priceTrade resolves the trade's FX rate and base amount. cashAmount is the
// amount moved on fundingAccount (nil = no cash leg) in that account's currency
// and cashNative the part of the trade it settles in the security's currency;
// total is the figure that is converted to base.
func (s *investmentService) priceTrade(
	userID, secCcy string, date time.Time, explicit *fx.Rate,
	cashAccount *models.Account, cashAmount, cashNative, total int64,
) (fx.Rate, int64, error) {
	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return 0, 0, err
	}
	var leg *tradeCashLeg
	if cashAccount != nil {
		leg = &tradeCashLeg{currency: normalizeCode(cashAccount.Currency), amount: cashAmount, native: cashNative}
	}
	rate, err := s.tradeRate(base, secCcy, date, explicit, leg)
	if err != nil {
		return 0, 0, err
	}
	baseAmount, err := baseAmountFor(total, rate)
	if err != nil {
		return 0, 0, err
	}
	return rate, baseAmount, nil
}

// RecordSell records a sell transaction and adjusts the investment holding
// proportionally, in the security's currency and in base. If p.DepositAccountID
// is set the net proceeds are credited to that cash account: in the account's own
// currency, which must be stated in p.DepositAmount when it differs from the
// security's.
func (s *investmentService) RecordSell(userID, investmentID string, p SellParams) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}
	secCcy := normalizeCode(investment.Security.Currency)

	if p.Quantity > investment.Quantity {
		return nil, apperrors.ErrInsufficientShares
	}

	explicit, err := parseOptionalRate(p.FXRate)
	if err != nil {
		return nil, err
	}
	gross, err := lineAmount(p.Quantity, p.PricePerUnit)
	if err != nil {
		return nil, err
	}
	totalAmount := gross - p.Fee

	depositAccount, cashAmount, err := s.resolveCashLeg(userID, cashLegRequest{
		accountID: p.DepositAccountID, amount: p.DepositAmount, field: "deposit_amount", native: totalAmount,
	}, secCcy, p.Date)
	if err != nil {
		return nil, err
	}

	rate, baseAmount, err := s.priceTrade(userID, secCcy, p.Date, explicit, depositAccount, cashAmount, totalAmount, totalAmount)
	if err != nil {
		return nil, err
	}

	var invTx models.InvestmentTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-read and re-validate the holding under a row lock so two concurrent
		// sells can never oversell the position.
		locked, txErr := lockInvestment(tx, investmentID)
		if txErr != nil {
			return txErr
		}
		st := stateOf(locked)
		res, txErr := st.apply(tradeEvent{
			Type: models.InvestmentTransactionSell, Quantity: p.Quantity, TotalAmount: totalAmount, BaseAmount: baseAmount,
		})
		if txErr != nil {
			return txErr
		}

		invTx = models.InvestmentTransaction{
			InvestmentID:         investmentID,
			Type:                 models.InvestmentTransactionSell,
			Date:                 p.Date,
			Quantity:             p.Quantity,
			PricePerUnit:         p.PricePerUnit,
			TotalAmount:          totalAmount,
			Fee:                  p.Fee,
			Notes:                p.Notes,
			RealizedGainLoss:     res.RealizedGainLoss,
			RealizedGainLossBase: res.RealizedGainLossBase,
			FXRateToBase:         &rate,
			BaseAmount:           &baseAmount,
		}
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(st.columns()).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Credit deposit account with net proceeds (row-locked, relative update)
		if depositAccount != nil {
			return s.applyCashLeg(tx, userID, depositAccount.ID, models.TransactionTypeIncome, cashAmount,
				fmt.Sprintf("Sale proceeds: %g %s", p.Quantity, investment.Security.Name), p.Date)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	invTx.Currency = secCcy
	return &invTx, nil
}

// RecordDividend records a dividend without changing quantity, cost basis or
// realized P&L. p.Amount is the gross dividend in the security's currency and
// p.Fee the withholding tax and fees. With p.DepositAccountID the NET amount
// (gross − fee) is credited to that cash account, in the account's own currency,
// which must be stated in p.DepositAmount when it differs from the security's.
func (s *investmentService) RecordDividend(userID, investmentID string, p DividendParams) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
		return nil, err
	}
	secCcy := normalizeCode(investment.Security.Currency)

	if p.Fee < 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "fee must not be negative")
	}
	if p.Fee > p.Amount {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "fee must not exceed the dividend amount")
	}
	explicit, err := parseOptionalRate(p.FXRate)
	if err != nil {
		return nil, err
	}
	net := p.Amount - p.Fee

	depositAccount, cashAmount, err := s.resolveCashLeg(userID, cashLegRequest{
		accountID: p.DepositAccountID, amount: p.DepositAmount, field: "deposit_amount", native: net,
	}, secCcy, p.Date)
	if err != nil {
		return nil, err
	}
	if depositAccount != nil && net <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the net dividend must be positive to deposit it")
	}

	// The cash leg settles the NET dividend; the gross is what gets converted.
	rate, baseAmount, err := s.priceTrade(userID, secCcy, p.Date, explicit, depositAccount, cashAmount, net, p.Amount)
	if err != nil {
		return nil, err
	}

	invTx := models.InvestmentTransaction{
		InvestmentID: investmentID,
		Type:         models.InvestmentTransactionDividend,
		Date:         p.Date,
		TotalAmount:  p.Amount,
		Fee:          p.Fee,
		DividendType: p.DividendType,
		Notes:        p.Notes,
		FXRateToBase: &rate,
		BaseAmount:   &baseAmount,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if txErr := tx.Create(&invTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if depositAccount != nil {
			return s.applyCashLeg(tx, userID, depositAccount.ID, models.TransactionTypeIncome, cashAmount,
				fmt.Sprintf("Dividend: %s", investment.Security.Name), p.Date)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	invTx.Currency = secCcy
	return &invTx, nil
}

// RecordSplit records a stock split and multiplies the investment quantity.
func (s *investmentService) RecordSplit(
	userID, investmentID string,
	date time.Time,
	splitRatio float64,
	notes string,
) (*models.InvestmentTransaction, error) {
	investment, err := s.GetInvestmentByID(userID, investmentID)
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
		st := stateOf(locked)
		if _, txErr := st.apply(tradeEvent{Type: models.InvestmentTransactionSplit, SplitRatio: splitRatio}); txErr != nil {
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

		// Multiply quantity by split ratio; cost bases stay the same
		if txErr := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(st.columns()).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	invTx.Currency = normalizeCode(investment.Security.Currency)
	return &invTx, nil
}

// GetInvestmentTransactions returns a paginated list of transactions for an investment.
func (s *investmentService) GetInvestmentTransactions(userID, investmentID string, page pagination.PageRequest) (*pagination.PageResponse[models.InvestmentTransaction], error) {
	// Verify investment exists and user owns it
	investment, err := s.GetInvestmentByID(userID, investmentID)
	if err != nil {
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

	currency := normalizeCode(investment.Security.Currency)
	for i := range transactions {
		transactions[i].Currency = currency
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
