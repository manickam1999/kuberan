package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	_ "kuberan/internal/models" // swagger type references
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// InvestmentHandler handles investment-related requests.
type InvestmentHandler struct {
	investmentService services.InvestmentServicer
	auditService      services.AuditServicer
}

// NewInvestmentHandler creates a new InvestmentHandler.
func NewInvestmentHandler(investmentService services.InvestmentServicer, auditService services.AuditServicer) *InvestmentHandler {
	return &InvestmentHandler{investmentService: investmentService, auditService: auditService}
}

// AddInvestmentRequest represents the request payload for adding an investment.
type AddInvestmentRequest struct {
	AccountID     string     `json:"account_id" binding:"required"`
	SecurityID    string     `json:"security_id" binding:"required"`
	Quantity      float64    `json:"quantity" binding:"required,gt=0"`
	PurchasePrice int64      `json:"purchase_price" binding:"required,gt=0"`
	WalletAddress string     `json:"wallet_address,omitempty"`
	Date          *time.Time `json:"date"`                    // optional, defaults to now
	Fee           int64      `json:"fee" binding:"gte=0"`     // optional, defaults to 0
	Notes         string     `json:"notes" binding:"max=500"` // optional, defaults to "Initial purchase"
}

// RecordBuyRequest represents the request payload for recording a buy transaction.
type RecordBuyRequest struct {
	Date         time.Time `json:"date" binding:"required"`
	Quantity     float64   `json:"quantity" binding:"required,gt=0"`
	PricePerUnit int64     `json:"price_per_unit" binding:"required,gt=0"`
	Fee          int64     `json:"fee" binding:"gte=0"`
	Notes        string    `json:"notes" binding:"max=500"`
}

// RecordSellRequest represents the request payload for recording a sell transaction.
type RecordSellRequest struct {
	Date         time.Time `json:"date" binding:"required"`
	Quantity     float64   `json:"quantity" binding:"required,gt=0"`
	PricePerUnit int64     `json:"price_per_unit" binding:"required,gt=0"`
	Fee          int64     `json:"fee" binding:"gte=0"`
	Notes        string    `json:"notes" binding:"max=500"`
}

// RecordDividendRequest represents the request payload for recording a dividend.
type RecordDividendRequest struct {
	Date         time.Time `json:"date" binding:"required"`
	Amount       int64     `json:"amount" binding:"required,gt=0"`
	DividendType string    `json:"dividend_type" binding:"max=50"`
	Notes        string    `json:"notes" binding:"max=500"`
}

// RecordSplitRequest represents the request payload for recording a stock split.
type RecordSplitRequest struct {
	Date       time.Time `json:"date" binding:"required"`
	SplitRatio float64   `json:"split_ratio" binding:"required,gt=0"`
	Notes      string    `json:"notes" binding:"max=500"`
}

// GetAllInvestments handles listing all investments across all investment accounts.
// @Summary     Get all investments
// @Description Get a paginated list of all investments across all active investment accounts
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       page      query int false "Page number (default 1)"
// @Param       page_size query int false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Investment] "Paginated investments"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments [get]
func (h *InvestmentHandler) GetAllInvestments(c *gin.Context) {
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

	result, err := h.investmentService.GetAllInvestments(userID, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// AddInvestment handles adding a new investment holding.
// @Summary     Add investment
// @Description Add a new investment holding to an investment account
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body AddInvestmentRequest true "Investment details"
// @Success     201 {object} models.Investment "Investment created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments [post]
func (h *InvestmentHandler) AddInvestment(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req AddInvestmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	investment, err := h.investmentService.AddInvestment(
		userID, req.AccountID, req.SecurityID, req.Quantity, req.PurchasePrice, req.WalletAddress, req.Date, req.Fee, req.Notes,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_INVESTMENT", "investment", investment.ID, c.ClientIP(),
		map[string]interface{}{"security_id": req.SecurityID, "quantity": req.Quantity})

	c.JSON(http.StatusCreated, gin.H{"investment": investment})
}

// GetAccountInvestments handles listing investments for an account.
// @Summary     Get account investments
// @Description Get a paginated list of investments for an account
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id        path  int false "Account ID"
// @Param       page      query int false "Page number (default 1)"
// @Param       page_size query int false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Investment] "Paginated investments"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /accounts/{id}/investments [get]
func (h *InvestmentHandler) GetAccountInvestments(c *gin.Context) {
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

	result, err := h.investmentService.GetAccountInvestments(userID, accountID, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetInvestment handles retrieving a specific investment.
// @Summary     Get investment by ID
// @Description Get a specific investment by ID
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Investment ID"
// @Success     200 {object} models.Investment "Investment details"
// @Failure     400 {object} ErrorResponse "Invalid investment ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id} [get]
func (h *InvestmentHandler) GetInvestment(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	investment, err := h.investmentService.GetInvestmentByID(userID, investmentID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"investment": investment})
}

// GetPortfolio handles retrieving the aggregated portfolio summary.
// @Summary     Get portfolio summary
// @Description Get an aggregated portfolio summary across all investment accounts
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} services.PortfolioSummary "Portfolio summary"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/portfolio [get]
func (h *InvestmentHandler) GetPortfolio(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	summary, err := h.investmentService.GetPortfolio(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"portfolio": summary})
}

// RecordBuy handles recording a buy transaction for an investment.
// @Summary     Record buy transaction
// @Description Record a buy transaction for an investment holding
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int              true "Investment ID"
// @Param       request body RecordBuyRequest  true "Buy details"
// @Success     201 {object} models.InvestmentTransaction "Transaction created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id}/buy [post]
func (h *InvestmentHandler) RecordBuy(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req RecordBuyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	invTx, err := h.investmentService.RecordBuy(userID, investmentID, req.Date, req.Quantity, req.PricePerUnit, req.Fee, req.Notes)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "INVESTMENT_BUY", "investment", investmentID, c.ClientIP(),
		map[string]interface{}{"quantity": req.Quantity, "price_per_unit": req.PricePerUnit})

	c.JSON(http.StatusCreated, gin.H{"transaction": invTx})
}

// RecordSell handles recording a sell transaction for an investment.
// @Summary     Record sell transaction
// @Description Record a sell transaction for an investment holding
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int               true "Investment ID"
// @Param       request body RecordSellRequest  true "Sell details"
// @Success     201 {object} models.InvestmentTransaction "Transaction created"
// @Failure     400 {object} ErrorResponse "Invalid input or insufficient shares"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id}/sell [post]
func (h *InvestmentHandler) RecordSell(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req RecordSellRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	invTx, err := h.investmentService.RecordSell(userID, investmentID, req.Date, req.Quantity, req.PricePerUnit, req.Fee, req.Notes)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "INVESTMENT_SELL", "investment", investmentID, c.ClientIP(),
		map[string]interface{}{"quantity": req.Quantity, "price_per_unit": req.PricePerUnit})

	c.JSON(http.StatusCreated, gin.H{"transaction": invTx})
}

// RecordDividend handles recording a dividend for an investment.
// @Summary     Record dividend
// @Description Record a dividend payment for an investment holding
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int                    true "Investment ID"
// @Param       request body RecordDividendRequest   true "Dividend details"
// @Success     201 {object} models.InvestmentTransaction "Transaction created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id}/dividend [post]
func (h *InvestmentHandler) RecordDividend(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req RecordDividendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	invTx, err := h.investmentService.RecordDividend(userID, investmentID, req.Date, req.Amount, req.DividendType, req.Notes)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "INVESTMENT_DIVIDEND", "investment", investmentID, c.ClientIP(),
		map[string]interface{}{"amount": req.Amount, "dividend_type": req.DividendType})

	c.JSON(http.StatusCreated, gin.H{"transaction": invTx})
}

// RecordSplit handles recording a stock split for an investment.
// @Summary     Record stock split
// @Description Record a stock split for an investment holding
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path int                 true "Investment ID"
// @Param       request body RecordSplitRequest   true "Split details"
// @Success     201 {object} models.InvestmentTransaction "Transaction created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id}/split [post]
func (h *InvestmentHandler) RecordSplit(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req RecordSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	invTx, err := h.investmentService.RecordSplit(userID, investmentID, req.Date, req.SplitRatio, req.Notes)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "INVESTMENT_SPLIT", "investment", investmentID, c.ClientIP(),
		map[string]interface{}{"split_ratio": req.SplitRatio})

	c.JSON(http.StatusCreated, gin.H{"transaction": invTx})
}

// GetInvestmentTransactions handles listing transactions for an investment.
// @Summary     Get investment transactions
// @Description Get a paginated list of transactions for an investment
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id        path  int true "Investment ID"
// @Param       page      query int false "Page number (default 1)"
// @Param       page_size query int false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.InvestmentTransaction] "Paginated transactions"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Investment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /investments/{id}/transactions [get]
func (h *InvestmentHandler) GetInvestmentTransactions(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	investmentID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	result, err := h.investmentService.GetInvestmentTransactions(userID, investmentID, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}
