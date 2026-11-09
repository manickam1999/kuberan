package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// budgetService handles budget-related business logic.
type budgetService struct {
	db        *gorm.DB
	fxService FXServicer // optional; converts spending in other currencies
}

// BudgetServiceOption configures optional dependencies of the budget service.
type BudgetServiceOption func(*budgetService)

// WithBudgetFXService sets the FX service used to convert spending in other
// currencies into the budget's currency. Without it such spending is reported
// as missing (fx_missing), never counted 1:1.
func WithBudgetFXService(fxs FXServicer) BudgetServiceOption {
	return func(s *budgetService) { s.fxService = fxs }
}

// NewBudgetService creates a new BudgetServicer.
func NewBudgetService(db *gorm.DB, opts ...BudgetServiceOption) BudgetServicer {
	s := &budgetService{db: db}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// resolveBudgetCurrency validates an explicit currency (upper-casing it) or
// returns the user's base currency when none is given.
func (s *budgetService) resolveBudgetCurrency(userID, requested string) (string, error) {
	if requested == "" {
		return userBaseCurrency(s.db, userID)
	}
	code := normalizeCode(requested)
	if !isCurrencyCode(code) {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "currency must be a 3-letter ISO 4217 code")
	}
	if !isTwoDecimalCurrency(code) {
		return "", apperrors.ErrUnsupportedCurrency
	}
	return code, nil
}

// CreateBudget creates a new recurring budget for a category. It rejects a second
// active budget for the same (user, category, period) with ErrBudgetAlreadyExists (D4).
func (s *budgetService) CreateBudget(
	userID, categoryID string,
	name string,
	amount int64,
	period models.BudgetPeriod,
	opts ...BudgetOption,
) (*models.Budget, error) {
	if err := CheckTextLength("name", name, MaxNameLen); err != nil {
		return nil, err
	}
	var o budgetOptions
	for _, opt := range opts {
		opt(&o)
	}
	currency, err := s.resolveBudgetCurrency(userID, o.currency)
	if err != nil {
		return nil, err
	}

	// Verify category exists and belongs to user
	var category models.Category
	if err := s.db.Where("id = ? AND user_id = ?", categoryID, userID).First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrCategoryNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// D4: only one active budget per (user, category, period).
	var existing int64
	if err := s.db.Model(&models.Budget{}).
		Where("user_id = ? AND category_id = ? AND period = ? AND is_active = ?",
			userID, categoryID, period, true).
		Count(&existing).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if existing > 0 {
		return nil, apperrors.ErrBudgetAlreadyExists
	}

	budget := &models.Budget{
		UserID:     userID,
		CategoryID: categoryID,
		Name:       name,
		Amount:     amount,
		Period:     period,
		IsActive:   true,
		Currency:   currency,
	}

	if err := s.db.Create(budget).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return budget, nil
}

// GetUserBudgets returns a paginated list of budgets for the user with optional filters,
// ordered by newest first for deterministic pagination.
func (s *budgetService) GetUserBudgets(
	userID string,
	page pagination.PageRequest,
	isActive *bool,
	period *models.BudgetPeriod,
) (*pagination.PageResponse[models.Budget], error) {
	page.Defaults()

	base := s.db.Model(&models.Budget{}).Where("user_id = ?", userID)
	if isActive != nil {
		base = base.Where("is_active = ?", *isActive)
	}
	if period != nil {
		base = base.Where("period = ?", *period)
	}

	var totalItems int64
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var budgets []models.Budget
	if err := base.Preload("Category").
		Order("created_at DESC").
		Scopes(pagination.Paginate(page)).
		Find(&budgets).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(budgets, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetBudgetByID returns a budget by ID if it belongs to the user.
func (s *budgetService) GetBudgetByID(userID, budgetID string) (*models.Budget, error) {
	var budget models.Budget
	if err := s.db.Preload("Category").Where("id = ? AND user_id = ?", budgetID, userID).First(&budget).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrBudgetNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &budget, nil
}

// UpdateBudget updates an existing budget's fields. Nil pointers are left unchanged.
func (s *budgetService) UpdateBudget(
	userID, budgetID string,
	name string,
	amount *int64,
	period *models.BudgetPeriod,
	isActive *bool,
	opts ...BudgetOption,
) (*models.Budget, error) {
	if err := CheckTextLength("name", name, MaxNameLen); err != nil {
		return nil, err
	}
	var o budgetOptions
	for _, opt := range opts {
		opt(&o)
	}

	budget, err := s.GetBudgetByID(userID, budgetID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	if name != "" {
		updates["name"] = name
	}
	if amount != nil {
		updates["amount"] = *amount
	}
	if period != nil {
		updates["period"] = *period
	}
	if isActive != nil {
		updates["is_active"] = *isActive
	}
	if o.currency != "" {
		currency, cErr := s.resolveBudgetCurrency(userID, o.currency)
		if cErr != nil {
			return nil, cErr
		}
		updates["currency"] = currency
	}

	if len(updates) > 0 {
		if err := s.db.Model(budget).Updates(updates).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	return budget, nil
}

// DeleteBudget soft-deletes a budget.
func (s *budgetService) DeleteBudget(userID, budgetID string) error {
	budget, err := s.GetBudgetByID(userID, budgetID)
	if err != nil {
		return err
	}

	if err := s.db.Delete(budget).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// periodWindow returns the current calendar-period window for the given period,
// derived from now. Budgets are recurring caps, so progress is always the current window.
func periodWindow(period models.BudgetPeriod, now time.Time) (start, end time.Time) {
	switch period {
	case models.BudgetPeriodMonthly:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, -1)
		end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, now.Location())
	case models.BudgetPeriodYearly:
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = time.Date(now.Year(), 12, 31, 23, 59, 59, 999999999, now.Location())
	}
	return start, end
}

// computeProgress builds a BudgetProgress from a budget amount and spent total.
func computeProgress(budgetID, currency string, amount, spent int64, fxMissing bool) BudgetProgress {
	remaining := amount - spent
	var percentage float64
	if amount > 0 {
		percentage = float64(spent) / float64(amount) * 100
	}
	return BudgetProgress{
		BudgetID:   budgetID,
		Budgeted:   amount,
		Spent:      spent,
		Remaining:  remaining,
		Percentage: percentage,
		Currency:   currency,
		FXMissing:  fxMissing,
	}
}

// budgetSpendRow is one (UTC day, account currency[, category]) group of expenses.
type budgetSpendRow struct {
	Bucket     string
	At         time.Time
	Currency   string
	CategoryID string
	Total      int64
}

// budgetSpend loads expense groups of a user in [start, end] for the given
// categories, grouped by (UTC day, account currency, category).
func (s *budgetService) budgetSpend(userID string, categoryIDs []string, start, end time.Time) ([]budgetSpendRow, bool, error) {
	dayExpr, dayGroup, isPostgres := dayExprs(s.db)
	var rows []budgetSpendRow
	err := s.db.Table("transactions t").
		Joins("JOIN accounts a ON a.id = t.account_id").
		Select(dayExpr+", a.currency AS currency, t.category_id AS category_id, COALESCE(SUM("+effectiveAmountSQL+"), 0) AS total").
		Where("t.user_id = ? AND t.category_id IN ? AND t.type = ? AND t.deleted_at IS NULL AND t.date BETWEEN ? AND ?",
			userID, categoryIDs, models.TransactionTypeExpense, start, end).
		Group(dayGroup + ", a.currency, t.category_id").
		Scan(&rows).Error
	if err != nil {
		return nil, false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return rows, isPostgres, nil
}

// convertBudgetSpend sums the rows of one category converted to target at each
// row's own day. fxMissing is true when a group had no rate (it is left out).
func convertBudgetSpend(rows []budgetSpendRow, isPostgres bool, categoryID string, dv *datedValuer) (spent int64, fxMissing bool) {
	for _, r := range rows {
		if r.CategoryID != categoryID {
			continue
		}
		converted, ok := dv.Convert(r.Total, r.Currency, dayKey(isPostgres, r.Bucket, r.At))
		if !ok {
			fxMissing = true
			continue
		}
		spent += converted
	}
	return spent, fxMissing
}

// budgetValuer builds the dated valuer for target over the days and currencies
// present in rows.
func (s *budgetService) budgetValuer(rows []budgetSpendRow, isPostgres bool, target string) (*datedValuer, error) {
	days := map[string]struct{}{}
	currencies := map[string]struct{}{}
	for _, r := range rows {
		days[dayKey(isPostgres, r.Bucket, r.At)] = struct{}{}
		currencies[normalizeCode(r.Currency)] = struct{}{}
	}
	first, last, ok := dayBounds(days)
	if !ok {
		return &datedValuer{target: normalizeCode(target)}, nil
	}
	return newDatedValuer(s.fxService, target, first, last, currencyList(currencies))
}

// GetBudgetProgress calculates spending vs budget for the current period. Spending
// is converted to the budget's currency at each transaction's own day.
func (s *budgetService) GetBudgetProgress(userID, budgetID string) (*BudgetProgress, error) {
	budget, err := s.GetBudgetByID(userID, budgetID)
	if err != nil {
		return nil, err
	}

	periodStart, periodEnd := periodWindow(budget.Period, time.Now())
	rows, isPostgres, err := s.budgetSpend(userID, []string{budget.CategoryID}, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	dv, err := s.budgetValuer(rows, isPostgres, budget.Currency)
	if err != nil {
		return nil, err
	}
	spent, missing := convertBudgetSpend(rows, isPostgres, budget.CategoryID, dv)

	progress := computeProgress(budget.ID, budget.Currency, budget.Amount, spent, missing)
	return &progress, nil
}

// GetActiveBudgetsProgress returns progress for every active budget in a small,
// fixed number of queries (one to load active budgets, then one grouped aggregate
// per distinct period, plus one batched FX lookup per distinct (period, budget
// currency) that needs conversion) rather than an N+1 per-budget loop.
// Inactive budgets are excluded. Ordering matches GetUserBudgets (created_at DESC).
func (s *budgetService) GetActiveBudgetsProgress(userID string) ([]BudgetProgress, error) {
	var budgets []models.Budget
	if err := s.db.
		Where("user_id = ? AND is_active = ?", userID, true).
		Order("created_at DESC").
		Find(&budgets).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if len(budgets) == 0 {
		return []BudgetProgress{}, nil
	}

	now := time.Now()
	categoriesByPeriod := make(map[models.BudgetPeriod][]string)
	for i := range budgets {
		b := &budgets[i]
		categoriesByPeriod[b.Period] = append(categoriesByPeriod[b.Period], b.CategoryID)
	}

	type periodData struct {
		rows       []budgetSpendRow
		isPostgres bool
		valuers    map[string]*datedValuer // by budget currency
	}
	data := make(map[models.BudgetPeriod]*periodData, len(categoriesByPeriod))
	for period, categoryIDs := range categoriesByPeriod {
		periodStart, periodEnd := periodWindow(period, now)
		rows, isPostgres, err := s.budgetSpend(userID, categoryIDs, periodStart, periodEnd)
		if err != nil {
			return nil, err
		}
		data[period] = &periodData{rows: rows, isPostgres: isPostgres, valuers: map[string]*datedValuer{}}
	}

	progress := make([]BudgetProgress, 0, len(budgets))
	for i := range budgets {
		b := &budgets[i]
		pd := data[b.Period]
		dv, ok := pd.valuers[b.Currency]
		if !ok {
			var err error
			if dv, err = s.budgetValuer(pd.rows, pd.isPostgres, b.Currency); err != nil {
				return nil, err
			}
			pd.valuers[b.Currency] = dv
		}
		spent, missing := convertBudgetSpend(pd.rows, pd.isPostgres, b.CategoryID, dv)
		progress = append(progress, computeProgress(b.ID, b.Currency, b.Amount, spent, missing))
	}

	return progress, nil
}
