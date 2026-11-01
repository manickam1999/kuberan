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
// @Success     200 {object} map[string]int "Prices recorded count"
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
		}
	}

	count, err := h.securityService.RecordPrices(inputs)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log("", "PIPELINE_RECORD_PRICES", "security_price", "", c.ClientIP(),
		map[string]interface{}{"prices_recorded": count})

	c.JSON(http.StatusOK, gin.H{"prices_recorded": count})
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
