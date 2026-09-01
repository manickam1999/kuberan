package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// TransactionHandler handles transaction-related requests.
type TransactionHandler struct {
	transactionService services.TransactionServicer
	auditService       services.AuditServicer
}

// NewTransactionHandler creates a new TransactionHandler.
func NewTransactionHandler(transactionService services.TransactionServicer, auditService services.AuditServicer) *TransactionHandler {
	return &TransactionHandler{transactionService: transactionService, auditService: auditService}
}

// CreateTransactionRequest represents the request payload for creating a transaction
type CreateTransactionRequest struct {
	AccountID   string                 `json:"account_id" binding:"required"`
	CategoryID  *string                `json:"category_id"`
	Type        models.TransactionType `json:"type" binding:"required,transaction_type"`
	Amount      int64                  `json:"amount" binding:"required,gt=0"`
	Description string                 `json:"description" binding:"max=500"`
	Date        *string                `json:"date"`
}

// TransactionResponse represents a transaction in the response
type TransactionResponse struct {
	ID          uint                   `json:"id"`
	UserID      uint                   `json:"user_id"`
	AccountID   *string                `json:"account_id"`
	CategoryID  string                 `json:"category_id,omitempty"`
	Type        models.TransactionType `json:"type"`
	Amount      int64                  `json:"amount"`
	Description string                 `json:"description"`
	Date        time.Time              `json:"date"`
}

// CreateTransaction handles the creation of a new transaction
// @Summary     Create a transaction
// @Description Create a new transaction (income or expense) for an account
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateTransactionRequest true "Transaction details"
// @Success     201 {object} TransactionResponse "Transaction created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions [post]
func (h *TransactionHandler) CreateTransaction(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	transactionDate := time.Now()
	if req.Date != nil && *req.Date != "" {
		parsed, parseErr := parseFlexibleTime(*req.Date)
		if parseErr != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
			return
		}
		transactionDate = parsed
	}

	transaction, err := h.transactionService.CreateTransaction(
		userID,
		req.AccountID,
		req.CategoryID,
		req.Type,
		req.Amount,
		req.Description,
		transactionDate,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_TRANSACTION", "transaction", transaction.ID, c.ClientIP(),
		map[string]interface{}{"type": req.Type, "amount": req.Amount, "account_id": req.AccountID})

	c.JSON(http.StatusCreated, gin.H{"transaction": transaction})
}

// CreateTransferRequest represents the request payload for creating a transfer
type CreateTransferRequest struct {
	FromAccountID string  `json:"from_account_id" binding:"required"`
	ToAccountID   string  `json:"to_account_id" binding:"required"`
	Amount        int64   `json:"amount" binding:"required,gt=0"`
	Description   string  `json:"description" binding:"max=500"`
	Date          *string `json:"date"`
}

// CreateTransfer handles the creation of a transfer between two accounts
// @Summary     Create a transfer
// @Description Transfer funds from one account to another
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateTransferRequest true "Transfer details"
// @Success     201 {object} TransactionResponse "Transfer created"
// @Failure     400 {object} ErrorResponse "Invalid input or insufficient balance"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/transfer [post]
func (h *TransactionHandler) CreateTransfer(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	transferDate := time.Now()
	if req.Date != nil && *req.Date != "" {
		parsed, parseErr := parseFlexibleTime(*req.Date)
		if parseErr != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
			return
		}
		transferDate = parsed
	}

	transaction, err := h.transactionService.CreateTransfer(
		userID,
		req.FromAccountID,
		req.ToAccountID,
		req.Amount,
		req.Description,
		transferDate,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_TRANSFER", "transaction", transaction.ID, c.ClientIP(),
		map[string]interface{}{
			"from_account_id": req.FromAccountID,
			"to_account_id":   req.ToAccountID,
			"amount":          req.Amount,
		})

	c.JSON(http.StatusCreated, gin.H{"transaction": transaction})
}

// GetAccountTransactions handles the retrieval of transactions for a specific account
// @Summary     Get account transactions
// @Description Get a paginated list of transactions for a specific account with optional filters
// @Tags        accounts,transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id          path  int    true  "Account ID"
// @Param       page        query int    false "Page number (default 1)"
// @Param       page_size   query int    false "Items per page (default 20, max 100)"
// @Param       from_date   query string false "Filter by start date (RFC3339 e.g. 2024-01-01T00:00:00Z, or YYYY-MM-DD)"
// @Param       to_date     query string false "Filter by end date (RFC3339 or YYYY-MM-DD)"
// @Param       type        query string false "Filter by transaction type (income, expense, transfer, investment)"
// @Param       category_id query int    false "Filter by category ID"
// @Param       min_amount  query int    false "Filter by minimum amount (cents)"
// @Param       max_amount  query int    false "Filter by maximum amount (cents)"
// @Success     200 {object} pagination.PageResponse[models.Transaction] "Paginated transactions"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/{id}/transactions [get]
func (h *TransactionHandler) GetAccountTransactions(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	accountID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	filter, err := parseTransactionFilter(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	result, err := h.transactionService.GetAccountTransactions(userID, accountID, page, filter)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetUserTransactions handles the retrieval of all transactions for the authenticated user
// @Summary     Get user transactions
// @Description Get a paginated list of all transactions for the authenticated user with optional filters
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       page        query int    false "Page number (default 1)"
// @Param       page_size   query int    false "Items per page (default 20, max 100)"
// @Param       account_id  query int    false "Filter by account ID"
// @Param       from_date   query string false "Filter by start date (RFC3339 e.g. 2024-01-01T00:00:00Z, or YYYY-MM-DD)"
// @Param       to_date     query string false "Filter by end date (RFC3339 or YYYY-MM-DD)"
// @Param       type        query string false "Filter by transaction type (income, expense, transfer, investment)"
// @Param       category_id query int    false "Filter by category ID"
// @Param       min_amount  query int    false "Filter by minimum amount (cents)"
// @Param       max_amount  query int    false "Filter by maximum amount (cents)"
// @Success     200 {object} pagination.PageResponse[models.Transaction] "Paginated transactions"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions [get]
func (h *TransactionHandler) GetUserTransactions(c *gin.Context) {
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

	filter, err := parseTransactionFilter(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if v := c.Query("account_id"); v != "" {
		filter.AccountID = &v
	}

	result, err := h.transactionService.GetUserTransactions(userID, page, filter)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func parseTransactionFilter(c *gin.Context) (services.TransactionFilter, error) {
	var filter services.TransactionFilter

	if v := c.Query("from_date"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			return filter, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid from_date format, use RFC3339 or YYYY-MM-DD")
		}
		filter.FromDate = &t
	}

	if v := c.Query("to_date"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			return filter, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid to_date format, use RFC3339 or YYYY-MM-DD")
		}
		filter.ToDate = &t
	}

	if v := c.Query("type"); v != "" {
		txType := models.TransactionType(v)
		switch txType {
		case models.TransactionTypeIncome, models.TransactionTypeExpense,
			models.TransactionTypeTransfer, models.TransactionTypeInvestment:
			filter.Type = &txType
		default:
			return filter, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid type, must be income, expense, transfer, or investment")
		}
	}

	if v := c.Query("category_id"); v != "" {
		filter.CategoryID = &v
	}

	if v := c.Query("min_amount"); v != "" {
		amt, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return filter, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid min_amount")
		}
		filter.MinAmount = &amt
	}

	if v := c.Query("max_amount"); v != "" {
		amt, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return filter, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid max_amount")
		}
		filter.MaxAmount = &amt
	}

	return filter, nil
}

// GetTransactionByID handles the retrieval of a specific transaction
// @Summary     Get transaction by ID
// @Description Get a specific transaction by ID
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Transaction ID"
// @Success     200 {object} TransactionResponse "Transaction details"
// @Failure     400 {object} ErrorResponse "Invalid transaction ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Transaction not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id} [get]
func (h *TransactionHandler) GetTransactionByID(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	transactionID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	transaction, err := h.transactionService.GetTransactionByID(userID, transactionID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"transaction": transaction})
}

// UpdateTransactionRequest represents the request payload for updating a transaction.
type UpdateTransactionRequest struct {
	AccountID   *string                 `json:"account_id"`
	CategoryID  *string                 `json:"category_id"`
	Type        *models.TransactionType `json:"type" binding:"omitempty,transaction_type"`
	Amount      *int64                  `json:"amount" binding:"omitempty,gt=0"`
	Description *string                 `json:"description" binding:"omitempty,max=500"`
	Date        *string                 `json:"date"`
}

// UpdateTransaction handles updating an existing transaction
// @Summary     Update transaction
// @Description Update an existing transaction. Only income/expense transactions can be edited. Transfer and investment transactions cannot be modified.
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int                     true "Transaction ID"
// @Param       request body UpdateTransactionRequest true "Fields to update"
// @Success     200 {object} TransactionResponse "Updated transaction"
// @Failure     400 {object} ErrorResponse "Invalid input or non-editable transaction"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Transaction not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id} [put]
func (h *TransactionHandler) UpdateTransaction(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	txID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	updateFields := services.TransactionUpdateFields{
		AccountID:   req.AccountID,
		Type:        req.Type,
		Amount:      req.Amount,
		Description: req.Description,
	}

	// Handle CategoryID: nil in JSON = don't change; empty string = clear; non-empty = set
	if req.CategoryID != nil {
		if *req.CategoryID == "" {
			var nilStr *string
			updateFields.CategoryID = &nilStr
		} else {
			updateFields.CategoryID = &req.CategoryID
		}
	}

	// Parse date if provided
	if req.Date != nil && *req.Date != "" {
		parsed, parseErr := parseFlexibleTime(*req.Date)
		if parseErr != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
			return
		}
		updateFields.Date = &parsed
	}

	transaction, err := h.transactionService.UpdateTransaction(userID, txID, updateFields)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_TRANSACTION", "transaction", txID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"transaction": transaction})
}

// DeleteTransaction handles the deletion of a transaction
// @Summary     Delete transaction
// @Description Delete a transaction by ID
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Transaction ID"
// @Success     200 {object} MessageResponse "Transaction deleted"
// @Failure     400 {object} ErrorResponse "Invalid transaction ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Transaction not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id} [delete]
func (h *TransactionHandler) DeleteTransaction(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	transactionID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.transactionService.DeleteTransaction(userID, transactionID); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_TRANSACTION", "transaction", transactionID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"message": "Transaction deleted successfully"})
}

// GetSpendingByCategory handles the retrieval of expense totals grouped by category
// @Summary     Get spending by category
// @Description Get expense totals grouped by category for a date range
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true "End date (RFC3339 or YYYY-MM-DD)"
// @Success     200 {object} services.SpendingByCategory "Spending breakdown by category"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/spending-by-category [get]
func (h *TransactionHandler) GetSpendingByCategory(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}

	fromTime, parseErr := parseFlexibleTime(fromStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	toTime, parseErr := parseFlexibleTime(toStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	result, err := h.transactionService.GetSpendingByCategory(userID, fromTime, toTime)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetCashflow handles the retrieval of income and expense totals grouped by
// category, for rendering the cashflow Sankey diagram.
// @Summary     Get cashflow breakdown
// @Description Get income and expense totals grouped by category for a date range (excludes transfers and investments)
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true "End date (RFC3339 or YYYY-MM-DD)"
// @Success     200 {object} services.Cashflow "Cashflow breakdown by category"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/cashflow [get]
func (h *TransactionHandler) GetCashflow(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}

	fromTime, parseErr := parseFlexibleTime(fromStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	toTime, parseErr := parseFlexibleTime(toStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	result, err := h.transactionService.GetCashflow(userID, fromTime, toTime)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetMonthlySummary handles the retrieval of monthly income and expense totals
// @Summary     Get monthly income and expense summary
// @Description Get monthly income and expense totals for the last N months
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       months query int false "Number of months back (default 6, min 1, max 24)"
// @Success     200 {object} map[string]interface{} "Monthly summary data"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/monthly-summary [get]
func (h *TransactionHandler) GetMonthlySummary(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	months := 6
	if v := c.Query("months"); v != "" {
		parsed, parseErr := strconv.Atoi(v)
		if parseErr == nil {
			months = parsed
		}
	}
	if months < 1 {
		months = 1
	}
	if months > 24 {
		months = 24
	}

	result, err := h.transactionService.GetMonthlySummary(userID, months)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetDailySpending handles the retrieval of daily expense totals
// @Summary     Get daily spending
// @Description Get daily expense totals for a date range
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true "End date (RFC3339 or YYYY-MM-DD)"
// @Success     200 {object} map[string]interface{} "Daily spending data"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/daily-spending [get]
func (h *TransactionHandler) GetDailySpending(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}

	fromTime, parseErr := parseFlexibleTime(fromStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	toTime, parseErr := parseFlexibleTime(toStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	if toTime.Sub(fromTime).Hours() > 366*24 {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "date range cannot exceed 366 days"))
		return
	}

	result, err := h.transactionService.GetDailySpending(userID, fromTime, toTime)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetDailySummary handles the retrieval of daily income and expense totals
// @Summary     Get daily income and expense summary
// @Description Get daily income and expense totals for a date range
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true "End date (RFC3339 or YYYY-MM-DD)"
// @Success     200 {object} map[string]interface{} "Daily summary data"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/daily-summary [get]
func (h *TransactionHandler) GetDailySummary(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}

	fromTime, parseErr := parseFlexibleTime(fromStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	toTime, parseErr := parseFlexibleTime(toStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	if toTime.Sub(fromTime).Hours() > 366*24 {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "date range cannot exceed 366 days"))
		return
	}

	result, err := h.transactionService.GetDailySummary(userID, fromTime, toTime)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetTopExpenses handles the retrieval of the largest expense transactions in a date range
// @Summary     Get top expenses
// @Description Get the largest expense transactions for a date range
// @Tags        transactions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true  "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true  "End date (RFC3339 or YYYY-MM-DD)"
// @Param       limit       query int    false "Max results (default 10, min 1, max 100)"
// @Param       category_id query string false "Filter to a single category"
// @Success     200 {object} services.TopExpenses "Top expense transactions"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/top-expenses [get]
func (h *TransactionHandler) GetTopExpenses(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}

	fromTime, parseErr := parseFlexibleTime(fromStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	toTime, parseErr := parseFlexibleTime(toStr)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, parseErr.Error()))
		return
	}

	limit := 10
	if v := c.Query("limit"); v != "" {
		parsed, convErr := strconv.Atoi(v)
		if convErr == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	var categoryID *string
	if v := c.Query("category_id"); v != "" {
		categoryID = &v
	}

	result, err := h.transactionService.GetTopExpenses(userID, fromTime, toTime, limit, categoryID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// MessageResponse represents a simple message response
type MessageResponse struct {
	Message string `json:"message"`
}
