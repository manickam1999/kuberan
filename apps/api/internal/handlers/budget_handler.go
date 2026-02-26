package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// BudgetHandler handles budget-related requests.
type BudgetHandler struct {
	budgetService services.BudgetServicer
	auditService  services.AuditServicer
}

// NewBudgetHandler creates a new BudgetHandler.
func NewBudgetHandler(budgetService services.BudgetServicer, auditService services.AuditServicer) *BudgetHandler {
	return &BudgetHandler{budgetService: budgetService, auditService: auditService}
}

// CreateBudgetRequest represents the request payload for creating a budget.
type CreateBudgetRequest struct {
	CategoryID string              `json:"category_id" binding:"required"`
	Name       string              `json:"name" binding:"required,min=1,max=100"`
	Amount     int64               `json:"amount" binding:"required,gt=0"`
	Period     models.BudgetPeriod `json:"period" binding:"required,budget_period"`
	StartDate  time.Time           `json:"start_date" binding:"required"`
	EndDate    *time.Time          `json:"end_date"`
}

// UpdateBudgetRequest represents the request payload for updating a budget.
type UpdateBudgetRequest struct {
	Name    string               `json:"name" binding:"omitempty,min=1,max=100"`
	Amount  *int64               `json:"amount" binding:"omitempty,gt=0"`
	Period  *models.BudgetPeriod `json:"period" binding:"omitempty,budget_period"`
	EndDate *time.Time           `json:"end_date"`
}

// CreateBudget handles the creation of a new budget.
// @Summary     Create a budget
// @Description Create a new budget for a category
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateBudgetRequest true "Budget details"
// @Success     201 {object} models.Budget "Budget created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets [post]
func (h *BudgetHandler) CreateBudget(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	budget, err := h.budgetService.CreateBudget(
		userID, req.CategoryID, req.Name, req.Amount, req.Period, req.StartDate, req.EndDate,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_BUDGET", "budget", budget.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "amount": req.Amount, "period": req.Period})

	c.JSON(http.StatusCreated, gin.H{"budget": budget})
}

// GetBudgets handles listing budgets for the authenticated user.
// @Summary     Get budgets
// @Description Get a paginated list of budgets for the authenticated user
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       is_active query bool   false "Filter by active status"
// @Param       period    query string false "Filter by period (monthly/yearly)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Budget] "Paginated budgets"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets [get]
func (h *BudgetHandler) GetBudgets(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	// Parse optional filters
	var isActive *bool
	if v := c.Query("is_active"); v != "" {
		switch v {
		case "true":
			b := true
			isActive = &b
		case "false":
			b := false
			isActive = &b
		default:
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "is_active must be 'true' or 'false'"))
			return
		}
	}

	var period *models.BudgetPeriod
	if v := c.Query("period"); v != "" {
		p := models.BudgetPeriod(v)
		if p != models.BudgetPeriodMonthly && p != models.BudgetPeriodYearly {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "period must be 'monthly' or 'yearly'"))
			return
		}
		period = &p
	}

	result, err := h.budgetService.GetUserBudgets(userID, page, isActive, period)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetBudget handles retrieving a specific budget.
// @Summary     Get budget by ID
// @Description Get a specific budget by ID
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Budget ID"
// @Success     200 {object} models.Budget "Budget details"
// @Failure     400 {object} ErrorResponse "Invalid budget ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Budget not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets/{id} [get]
func (h *BudgetHandler) GetBudget(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	budgetID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	budget, err := h.budgetService.GetBudgetByID(userID, budgetID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"budget": budget})
}

// UpdateBudget handles updating an existing budget.
// @Summary     Update budget
// @Description Update an existing budget
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int                true "Budget ID"
// @Param       request body UpdateBudgetRequest true "Updated budget details"
// @Success     200 {object} models.Budget "Updated budget"
// @Failure     400 {object} ErrorResponse "Invalid input or budget ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Budget not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets/{id} [put]
func (h *BudgetHandler) UpdateBudget(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	budgetID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	budget, err := h.budgetService.UpdateBudget(userID, budgetID, req.Name, req.Amount, req.Period, req.EndDate)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_BUDGET", "budget", budgetID, c.ClientIP(),
		map[string]interface{}{"name": req.Name})

	c.JSON(http.StatusOK, gin.H{"budget": budget})
}

// DeleteBudget handles deleting a budget.
// @Summary     Delete budget
// @Description Delete a budget by ID (soft delete)
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Budget ID"
// @Success     200 {object} MessageResponse "Budget deleted"
// @Failure     400 {object} ErrorResponse "Invalid budget ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Budget not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets/{id} [delete]
func (h *BudgetHandler) DeleteBudget(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	budgetID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.budgetService.DeleteBudget(userID, budgetID); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_BUDGET", "budget", budgetID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"message": "Budget deleted successfully"})
}

// GetBudgetProgress handles retrieving the spending progress for a budget.
// @Summary     Get budget progress
// @Description Get spending progress for a budget in the current period
// @Tags        budgets
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Budget ID"
// @Success     200 {object} services.BudgetProgress "Budget progress"
// @Failure     400 {object} ErrorResponse "Invalid budget ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Budget not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /budgets/{id}/progress [get]
func (h *BudgetHandler) GetBudgetProgress(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	budgetID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	progress, err := h.budgetService.GetBudgetProgress(userID, budgetID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"progress": progress})
}
