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

// SecurityHandler handles security-related requests.
type SecurityHandler struct {
	securityService services.SecurityServicer
	auditService    services.AuditServicer
}

// NewSecurityHandler creates a new SecurityHandler.
func NewSecurityHandler(securityService services.SecurityServicer, auditService services.AuditServicer) *SecurityHandler {
	return &SecurityHandler{securityService: securityService, auditService: auditService}
}

// CreateSecurityRequest represents the request payload for creating a security.
type CreateSecurityRequest struct {
	Symbol          string           `json:"symbol" binding:"required,min=1,max=20"`
	Name            string           `json:"name" binding:"required,min=1,max=200"`
	AssetType       models.AssetType `json:"asset_type" binding:"required,asset_type"`
	Currency        string           `json:"currency" binding:"omitempty,iso4217"`
	Exchange        string           `json:"exchange,omitempty"`
	ProviderSymbol  string           `json:"provider_symbol,omitempty"`
	MaturityDate    *time.Time       `json:"maturity_date,omitempty"`
	YieldToMaturity float64          `json:"yield_to_maturity,omitempty"`
	CouponRate      float64          `json:"coupon_rate,omitempty"`
	Network         string           `json:"network,omitempty"`
	PropertyType    string           `json:"property_type,omitempty"`
}

// RecordPricesRequest represents the request payload for bulk price recording.
type RecordPricesRequest struct {
	Prices []RecordPriceEntry `json:"prices" binding:"required,min=1,max=5000,dive"`
}

// RecordPriceEntry represents a single price entry in a bulk request.
type RecordPriceEntry struct {
	SecurityID string    `json:"security_id" binding:"required"`
	Price      int64     `json:"price" binding:"required,gt=0"`
	RecordedAt time.Time `json:"recorded_at" binding:"required"`
	// Currency is required per entry. It is validated by the service so a missing
	// one rejects only that entry (reason currency_required), as does a value that
	// differs from the security's currency (currency_mismatch).
	Currency string `json:"currency" binding:"omitempty,iso4217"`
}

// UpdateSecurityRequest is the payload for correcting a security. All fields
// are optional but at least one is required.
type UpdateSecurityRequest struct {
	Currency       *string `json:"currency" binding:"omitempty,iso4217"`
	Exchange       *string `json:"exchange"`
	ProviderSymbol *string `json:"provider_symbol"`
	Name           *string `json:"name" binding:"omitempty,max=200"`
	// ResetPrices deletes the security's prices; required to change the
	// currency of a security that has prices.
	ResetPrices bool `json:"reset_prices"`
}

// RecordPricesResponse is the response of the price recording endpoint.
type RecordPricesResponse struct {
	Recorded       int                      `json:"recorded"`
	PricesRecorded int                      `json:"prices_recorded"`
	Rejected       []services.RejectedPrice `json:"rejected"`
}

// UpdateSecurityResponse is the response of the security correction endpoint.
type UpdateSecurityResponse struct {
	Security      models.Security `json:"security"`
	PricesDeleted int64           `json:"prices_deleted"`
}

// CreateSecurity handles creating a new security.
// @Summary     Create security
// @Description Create a new security (pipeline endpoint)
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       request body CreateSecurityRequest true "Security details"
// @Success     201 {object} models.Security "Security created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     409 {object} ErrorResponse "Duplicate security"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/securities [post]
func (h *SecurityHandler) CreateSecurity(c *gin.Context) {
	var req CreateSecurityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	extraFields := buildSecurityExtraFields(req)

	security, err := h.securityService.CreateSecurity(
		req.Symbol, req.Name, req.AssetType, req.Currency, req.Exchange, extraFields,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log("", "CREATE_SECURITY", "security", security.ID, c.ClientIP(),
		map[string]interface{}{"symbol": req.Symbol, "asset_type": string(req.AssetType)})

	c.JSON(http.StatusCreated, gin.H{"security": security})
}

// UpdateSecurity handles correcting a security (pipeline endpoint).
// @Summary     Update security
// @Description Correct a security's currency, exchange, provider symbol or name (pipeline endpoint). Changing the currency of a security that has prices requires reset_prices, which deletes them in the same transaction.
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       id      path string                true "Security ID"
// @Param       request body UpdateSecurityRequest true "Fields to change"
// @Success     200 {object} UpdateSecurityResponse "Updated security and number of prices deleted"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     404 {object} ErrorResponse "Security not found"
// @Failure     409 {object} ErrorResponse "Currency locked by existing prices, or duplicate symbol and exchange"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/securities/{id} [patch]
func (h *SecurityHandler) UpdateSecurity(c *gin.Context) {
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateSecurityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	if req.Currency == nil && req.Exchange == nil && req.ProviderSymbol == nil && req.Name == nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "At least one field is required"))
		return
	}

	result, err := h.securityService.UpdateSecurity(id, services.SecurityUpdate{
		Currency:       req.Currency,
		Exchange:       req.Exchange,
		ProviderSymbol: req.ProviderSymbol,
		Name:           req.Name,
		ResetPrices:    req.ResetPrices,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log("", "PIPELINE_UPDATE_SECURITY", "security", id, c.ClientIP(),
		map[string]interface{}{"prices_deleted": result.PricesDeleted, "reset_prices": req.ResetPrices})

	c.JSON(http.StatusOK, gin.H{"security": result.Security, "prices_deleted": result.PricesDeleted})
}

// ListAllSecurities handles listing all securities for the pipeline.
// @Summary     List all securities (pipeline)
// @Description Get all active securities without pagination (pipeline endpoint)
// @Tags        pipeline
// @Produce     json
// @Security    ApiKeyAuth
// @Success     200 {object} map[string][]models.Security "All securities"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     500 {object} ErrorResponse "Server error"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/securities [get]
func (h *SecurityHandler) ListAllSecurities(c *gin.Context) {
	securities, err := h.securityService.ListAllSecurities()
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"securities": securities})
}

// ListSecurities handles listing all securities.
// @Summary     List securities
// @Description Get a paginated list of all securities, optionally filtered by search term
// @Tags        securities
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       search    query string false "Search by symbol or name (case-insensitive)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Security] "Paginated securities"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /securities [get]
func (h *SecurityHandler) ListSecurities(c *gin.Context) {
	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	search := c.Query("search")

	result, err := h.securityService.ListSecurities(search, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetSecurity handles retrieving a specific security.
// @Summary     Get security by ID
// @Description Get a specific security by ID
// @Tags        securities
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Security ID"
// @Success     200 {object} models.Security "Security details"
// @Failure     400 {object} ErrorResponse "Invalid security ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Security not found"
// @Router      /securities/{id} [get]
func (h *SecurityHandler) GetSecurity(c *gin.Context) {
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	security, err := h.securityService.GetSecurityByID(id)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"security": security})
}

// RecordPrices handles bulk price recording for securities.
// @Summary     Record prices
// @Description Bulk record prices for securities (pipeline endpoint)
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       request body RecordPricesRequest true "Price entries"
// @Description Entries for an unknown security, without a currency, or whose currency differs from the security's, are skipped and listed in `rejected` (the request still returns 200).
// @Success     200 {object} RecordPricesResponse "Recorded count and rejected entries"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/securities/prices [post]
func (h *SecurityHandler) RecordPrices(c *gin.Context) {
	var req RecordPricesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	inputs := make([]services.SecurityPriceInput, len(req.Prices))
	for i, p := range req.Prices {
		inputs[i] = services.SecurityPriceInput{
			SecurityID: p.SecurityID,
			Price:      p.Price,
			RecordedAt: p.RecordedAt,
			Currency:   p.Currency,
		}
	}

	result, err := h.securityService.RecordPrices(inputs)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log("", "PIPELINE_RECORD_PRICES", "security_price", "", c.ClientIP(),
		map[string]interface{}{"prices_recorded": result.Recorded, "prices_rejected": len(result.Rejected)})

	// prices_recorded is kept alongside recorded for clients that predate plan 024.
	c.JSON(http.StatusOK, gin.H{
		"recorded":        result.Recorded,
		"prices_recorded": result.Recorded,
		"rejected":        result.Rejected,
	})
}

// GetPriceHistory handles retrieving price history for a security.
// @Summary     Get price history
// @Description Get price history for a security (paginated)
// @Tags        securities
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id        path  int    true "Security ID"
// @Param       from_date query string true "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true "End date (RFC3339 or YYYY-MM-DD)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.SecurityPrice] "Paginated prices"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /securities/{id}/prices [get]
func (h *SecurityHandler) GetPriceHistory(c *gin.Context) {
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	fromStr := c.Query("from_date")
	if fromStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "from_date is required"))
		return
	}
	from, err := parseFlexibleTime(fromStr)
	if err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	toStr := c.Query("to_date")
	if toStr == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "to_date is required"))
		return
	}
	to, err := parseFlexibleTime(toStr)
	if err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	result, err := h.securityService.GetPriceHistory(id, from, to, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// buildSecurityExtraFields extracts asset-type-specific fields from the request into a map.
func buildSecurityExtraFields(req CreateSecurityRequest) map[string]interface{} {
	fields := make(map[string]interface{})
	if req.MaturityDate != nil {
		fields["maturity_date"] = req.MaturityDate
	}
	if req.YieldToMaturity != 0 {
		fields["yield_to_maturity"] = req.YieldToMaturity
	}
	if req.CouponRate != 0 {
		fields["coupon_rate"] = req.CouponRate
	}
	if req.Network != "" {
		fields["network"] = req.Network
	}
	if req.PropertyType != "" {
		fields["property_type"] = req.PropertyType
	}
	if req.ProviderSymbol != "" {
		fields["provider_symbol"] = req.ProviderSymbol
	}
	return fields
}
