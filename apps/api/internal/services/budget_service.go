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
	db *gorm.DB
}

// NewBudgetService creates a new BudgetServicer.
func NewBudgetService(db *gorm.DB) BudgetServicer {
	return &budgetService{db: db}
}

// CreateBudget creates a new recurring budget for a category. It rejects a second
// active budget for the same (user, category, period) with ErrBudgetAlreadyExists (D4).
func (s *budgetService) CreateBudget(
	userID, categoryID string,
	name string,
	amount int64,
	period models.BudgetPeriod,
) (*models.Budget, error) {
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
) (*models.Budget, error) {
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
func computeProgress(budgetID string, amount, spent int64) BudgetProgress {
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
	}
}

// GetBudgetProgress calculates spending vs budget for the current period.
func (s *budgetService) GetBudgetProgress(userID, budgetID string) (*BudgetProgress, error) {
	budget, err := s.GetBudgetByID(userID, budgetID)
	if err != nil {
		return nil, err
	}

	periodStart, periodEnd := periodWindow(budget.Period, time.Now())

	// Sum expense transactions for this category within the period
	var spent int64
	err = s.db.Model(&models.Transaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND category_id = ? AND type = ? AND date BETWEEN ? AND ?",
			userID, budget.CategoryID, models.TransactionTypeExpense, periodStart, periodEnd).
		Scan(&spent).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	progress := computeProgress(budget.ID, budget.Amount, spent)
	return &progress, nil
}

// GetActiveBudgetsProgress returns progress for every active budget in a small,
// fixed number of queries (one to load active budgets, then one grouped aggregate
// per distinct period over the relevant transactions) rather than an N+1 per-budget loop.
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

	// Group the category IDs by their budget's period so each period window is
	// aggregated in a single grouped query.
	now := time.Now()
	categoriesByPeriod := make(map[models.BudgetPeriod][]string)
	for _, b := range budgets {
		categoriesByPeriod[b.Period] = append(categoriesByPeriod[b.Period], b.CategoryID)
	}

	// spentByPeriodCategory[period][categoryID] = spent cents in that period window.
	spentByPeriodCategory := make(map[models.BudgetPeriod]map[string]int64, len(categoriesByPeriod))
	for period, categoryIDs := range categoriesByPeriod {
		periodStart, periodEnd := periodWindow(period, now)

		type categorySpend struct {
			CategoryID string
			Total      int64
		}
		var rows []categorySpend
		if err := s.db.Model(&models.Transaction{}).
			Select("category_id, COALESCE(SUM(amount), 0) AS total").
			Where("user_id = ? AND category_id IN ? AND type = ? AND date BETWEEN ? AND ?",
				userID, categoryIDs, models.TransactionTypeExpense, periodStart, periodEnd).
			Group("category_id").
			Scan(&rows).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		spentForPeriod := make(map[string]int64, len(rows))
		for _, r := range rows {
			spentForPeriod[r.CategoryID] = r.Total
		}
		spentByPeriodCategory[period] = spentForPeriod
	}

	progress := make([]BudgetProgress, 0, len(budgets))
	for _, b := range budgets {
		spent := spentByPeriodCategory[b.Period][b.CategoryID]
		progress = append(progress, computeProgress(b.ID, b.Amount, spent))
	}

	return progress, nil
}
