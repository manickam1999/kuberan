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

// AccountHandler handles account-related requests.
type AccountHandler struct {
	accountService services.AccountServicer
	auditService   services.AuditServicer
}

// NewAccountHandler creates a new AccountHandler.
func NewAccountHandler(accountService services.AccountServicer, auditService services.AuditServicer) *AccountHandler {
	return &AccountHandler{accountService: accountService, auditService: auditService}
}

// CreateCashAccountRequest represents the request payload for creating a cash account
type CreateCashAccountRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Description    string `json:"description" binding:"max=500"`
	Currency       string `json:"currency" binding:"omitempty,iso4217"`
	InitialBalance int64  `json:"initial_balance" binding:"gte=0"`
}

// CreateInvestmentAccountRequest represents the request payload for creating an investment account.
type CreateInvestmentAccountRequest struct {
	Name          string `json:"name" binding:"required,min=1,max=100"`
	Description   string `json:"description" binding:"max=500"`
	Currency      string `json:"currency" binding:"omitempty,iso4217"`
	Broker        string `json:"broker" binding:"max=100"`
	AccountNumber string `json:"account_number" binding:"max=50"`
}

// CreateCreditCardAccountRequest represents the request payload for creating a credit card account.
type CreateCreditCardAccountRequest struct {
	Name         string  `json:"name" binding:"required,min=1,max=100"`
	Description  string  `json:"description" binding:"max=500"`
	Currency     string  `json:"currency" binding:"omitempty,iso4217"`
	CreditLimit  int64   `json:"credit_limit" binding:"gte=0"`
	InterestRate float64 `json:"interest_rate" binding:"gte=0,lte=100"`
	DueDate      *string `json:"due_date"`
}

// UpdateAccountRequest represents the request payload for updating an account.
// Accepts common fields for all account types and type-specific optional fields.
type UpdateAccountRequest struct {
	Name          *string  `json:"name" binding:"omitempty,min=1,max=100"`
	Description   *string  `json:"description" binding:"omitempty,max=500"`
	IsActive      *bool    `json:"is_active"`
	IsPinned      *bool    `json:"is_pinned"`
	Broker        *string  `json:"broker" binding:"omitempty,max=100"`
	AccountNumber *string  `json:"account_number" binding:"omitempty,max=50"`
	InterestRate  *float64 `json:"interest_rate" binding:"omitempty,gte=0,lte=100"`
	DueDate       *string  `json:"due_date"`
	CreditLimit   *int64   `json:"credit_limit" binding:"omitempty,gte=0"`

	// Fixed deposit mutable fields (plan 020). Principal and start_date are
	// immutable after creation and are intentionally absent here.
	MaturityDate    *string `json:"maturity_date"`
	PayoutType      *string `json:"payout_type" binding:"omitempty,oneof=cumulative periodic"`
	PayoutFrequency *string `json:"payout_frequency" binding:"omitempty,oneof=monthly quarterly semiannual annual"`
	Compounding     *string `json:"compounding" binding:"omitempty,oneof=monthly quarterly semiannual annual"`
	LinkedAccountID *string `json:"linked_account_id"`
}

// CreateFixedDepositAccountRequest represents the request payload for creating a fixed deposit account.
type CreateFixedDepositAccountRequest struct {
	Name            string  `json:"name" binding:"required,min=1,max=100"`
	Description     string  `json:"description" binding:"max=500"`
	Currency        string  `json:"currency" binding:"omitempty,iso4217"`
	SourceAccountID string  `json:"source_account_id" binding:"required"`
	Principal       int64   `json:"principal" binding:"required,gt=0"`
	InterestRate    float64 `json:"interest_rate" binding:"gte=0,lte=100"`
	StartDate       string  `json:"start_date" binding:"required"`
	MaturityDate    string  `json:"maturity_date" binding:"required"`
	PayoutType      string  `json:"payout_type" binding:"required,oneof=cumulative periodic"`
	PayoutFrequency *string `json:"payout_frequency" binding:"omitempty,oneof=monthly quarterly semiannual annual"`
	Compounding     string  `json:"compounding" binding:"required,oneof=monthly quarterly semiannual annual"`
	LinkedAccountID *string `json:"linked_account_id"`
}

// CloseFixedDepositRequest represents the request payload for closing a fixed deposit account.
type CloseFixedDepositRequest struct {
	DestinationAccountID *string `json:"destination_account_id"`
	InterestOverride     *int64  `json:"interest_override"`
	Penalty              int64   `json:"penalty" binding:"gte=0"`
}

// AccountResponse represents an account in the response
type AccountResponse struct {
	ID          uint               `json:"id"`
	UserID      uint               `json:"user_id"`
	Name        string             `json:"name"`
	Type        models.AccountType `json:"type"`
	Description string             `json:"description"`
	Balance     int64              `json:"balance"`
	Currency    string             `json:"currency"`
	IsActive    bool               `json:"is_active"`
}

// CreateCashAccount handles the creation of a new cash account
// @Summary     Create a cash account
// @Description Create a new cash account for the authenticated user
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateCashAccountRequest true "Cash account details"
// @Success     201 {object} AccountResponse "Account created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/cash [post]
func (h *AccountHandler) CreateCashAccount(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateCashAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	account, err := h.accountService.CreateCashAccount(
		userID,
		req.Name,
		req.Description,
		req.Currency,
		req.InitialBalance,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_ACCOUNT", "account", account.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "currency": req.Currency})

	c.JSON(http.StatusCreated, gin.H{"account": account})
}

// CreateInvestmentAccount handles the creation of a new investment account.
// @Summary     Create an investment account
// @Description Create a new investment account for the authenticated user
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateInvestmentAccountRequest true "Investment account details"
// @Success     201 {object} AccountResponse "Account created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/investment [post]
func (h *AccountHandler) CreateInvestmentAccount(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateInvestmentAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	account, err := h.accountService.CreateInvestmentAccount(
		userID,
		req.Name,
		req.Description,
		req.Currency,
		req.Broker,
		req.AccountNumber,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_ACCOUNT", "account", account.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "type": "investment", "broker": req.Broker})

	c.JSON(http.StatusCreated, gin.H{"account": account})
}

// CreateCreditCardAccount handles the creation of a new credit card account.
// @Summary     Create a credit card account
// @Description Create a new credit card account for the authenticated user
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateCreditCardAccountRequest true "Credit card account details"
// @Success     201 {object} AccountResponse "Account created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/credit-card [post]
func (h *AccountHandler) CreateCreditCardAccount(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateCreditCardAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		parsed, parseErr := time.Parse(time.RFC3339, *req.DueDate)
		if parseErr != nil {
			parsed, parseErr = time.Parse("2006-01-02", *req.DueDate)
			if parseErr != nil {
				respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid due_date format"))
				return
			}
		}
		dueDate = &parsed
	}

	account, err := h.accountService.CreateCreditCardAccount(
		userID,
		req.Name,
		req.Description,
		req.Currency,
		req.CreditLimit,
		req.InterestRate,
		dueDate,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_ACCOUNT", "account", account.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "type": "credit_card", "credit_limit": req.CreditLimit})

	c.JSON(http.StatusCreated, gin.H{"account": account})
}

// GetUserAccounts handles the retrieval of accounts for a user
// @Summary     Get user accounts
// @Description Get a paginated list of accounts for the authenticated user
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       page      query int false "Page number (default 1)"
// @Param       page_size query int false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Account] "Paginated accounts"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts [get]
func (h *AccountHandler) GetUserAccounts(c *gin.Context) {
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

	result, err := h.accountService.GetUserAccounts(userID, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAccountByID handles the retrieval of a specific account for a user
// @Summary     Get account by ID
// @Description Get a specific account by ID for the authenticated user
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Account ID"
// @Success     200 {object} AccountResponse "Account details"
// @Failure     400 {object} ErrorResponse "Invalid account ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/{id} [get]
func (h *AccountHandler) GetAccountByID(c *gin.Context) {
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

	account, err := h.accountService.GetAccountByID(userID, accountID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"account": account})
}

// UpdateAccount handles updating an account of any type.
// @Summary     Update account
// @Description Update an existing account for the authenticated user. Accepts common fields for all account types and type-specific fields.
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Account ID"
// @Param       request body UpdateAccountRequest true "Updated account details"
// @Success     200 {object} AccountResponse "Updated account"
// @Failure     400 {object} ErrorResponse "Invalid input or account ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/{id} [put]
func (h *AccountHandler) UpdateAccount(c *gin.Context) {
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

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	updateFields := services.AccountUpdateFields{
		Name:            req.Name,
		Description:     req.Description,
		IsActive:        req.IsActive,
		IsPinned:        req.IsPinned,
		Broker:          req.Broker,
		AccountNumber:   req.AccountNumber,
		InterestRate:    req.InterestRate,
		CreditLimit:     req.CreditLimit,
		LinkedAccountID: req.LinkedAccountID,
	}

	if req.DueDate != nil && *req.DueDate != "" {
		parsed, parseErr := parseFlexibleTime(*req.DueDate)
		if parseErr != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid due_date format"))
			return
		}
		updateFields.DueDate = &parsed
	}

	if req.MaturityDate != nil && *req.MaturityDate != "" {
		parsed, parseErr := parseFlexibleTime(*req.MaturityDate)
		if parseErr != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid maturity_date format"))
			return
		}
		updateFields.MaturityDate = &parsed
	}

	if req.PayoutType != nil {
		pt := models.PayoutType(*req.PayoutType)
		updateFields.PayoutType = &pt
	}

	if req.PayoutFrequency != nil {
		pf := models.PayoutFrequency(*req.PayoutFrequency)
		updateFields.PayoutFrequency = &pf
	}

	if req.Compounding != nil {
		comp := models.Compounding(*req.Compounding)
		updateFields.Compounding = &comp
	}

	account, err := h.accountService.UpdateAccount(userID, accountID, updateFields)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_ACCOUNT", "account", accountID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"account": account})
}

// CreateFixedDepositAccount handles the creation of a new fixed deposit account.
// @Summary     Create a fixed deposit account
// @Description Open a new fixed deposit account. Principal is transferred from the specified source cash account atomically.
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateFixedDepositAccountRequest true "Fixed deposit account details"
// @Success     201 {object} AccountResponse "Account created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/fixed-deposit [post]
func (h *AccountHandler) CreateFixedDepositAccount(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateFixedDepositAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	startDate, parseErr := parseFlexibleTime(req.StartDate)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid start_date format"))
		return
	}

	maturityDate, parseErr := parseFlexibleTime(req.MaturityDate)
	if parseErr != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid maturity_date format"))
		return
	}

	if !maturityDate.After(startDate) {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "maturity_date must be after start_date"))
		return
	}

	var payoutFrequency *models.PayoutFrequency
	if req.PayoutFrequency != nil {
		pf := models.PayoutFrequency(*req.PayoutFrequency)
		payoutFrequency = &pf
	}

	params := services.CreateFixedDepositParams{
		Name:            req.Name,
		Description:     req.Description,
		Currency:        req.Currency,
		SourceAccountID: req.SourceAccountID,
		Principal:       req.Principal,
		InterestRate:    req.InterestRate,
		StartDate:       startDate,
		MaturityDate:    maturityDate,
		PayoutType:      models.PayoutType(req.PayoutType),
		PayoutFrequency: payoutFrequency,
		Compounding:     models.Compounding(req.Compounding),
		LinkedAccountID: req.LinkedAccountID,
	}

	account, err := h.accountService.CreateFixedDepositAccount(userID, params)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_ACCOUNT", "account", account.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "type": "fixed_deposit", "principal": req.Principal})

	c.JSON(http.StatusCreated, gin.H{"account": account})
}

// CloseFixedDeposit handles realizing a fixed deposit at maturity or closing it early.
// @Summary     Close a fixed deposit
// @Description Realize a fixed deposit: return principal to the destination account and post interest income. Handles both maturity realization and premature withdrawal.
// @Tags        accounts
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path     string                  true "Fixed deposit account ID"
// @Param       request body CloseFixedDepositRequest true "Close parameters"
// @Success     200 {object} AccountResponse "Closed account"
// @Failure     400 {object} ErrorResponse "Invalid input or deposit state"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     409 {object} ErrorResponse "Deposit already closed"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/{id}/fixed-deposit/close [post]
func (h *AccountHandler) CloseFixedDeposit(c *gin.Context) {
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

	var req CloseFixedDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	params := services.CloseFixedDepositParams{
		DestinationAccountID: req.DestinationAccountID,
		InterestOverride:     req.InterestOverride,
		Penalty:              req.Penalty,
	}

	account, err := h.accountService.CloseFixedDeposit(userID, accountID, params)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CLOSE_FIXED_DEPOSIT", "account", accountID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"account": account})
}
