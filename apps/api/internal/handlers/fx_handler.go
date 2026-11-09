package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/services"
)

const (
	// fxDefaultRangeDays is the history window returned when from/to are omitted.
	fxDefaultRangeDays = 30
	// fxMaxRangeDays caps the history window of a single request.
	fxMaxRangeDays = 3700
)

// FXHandler handles exchange-rate requests: the oracle's pipeline endpoints
// and the authenticated read endpoints.
type FXHandler struct {
	fxService    services.FXServicer
	auditService services.AuditServicer
}

// NewFXHandler creates a new FXHandler.
func NewFXHandler(fxService services.FXServicer, auditService services.AuditServicer) *FXHandler {
	return &FXHandler{fxService: fxService, auditService: auditService}
}

// RecordFXRatesRequest is the payload for bulk recording of exchange rates.
type RecordFXRatesRequest struct {
	Rates []FXRateEntry `json:"rates" binding:"required,min=1,max=5000,dive"`
}

// FXRateEntry is one exchange-rate observation in a bulk request. Rate is a
// decimal string (never a JSON number) so no precision is lost in transit.
type FXRateEntry struct {
	Base       string    `json:"base" binding:"required,iso4217"`
	Quote      string    `json:"quote" binding:"required,iso4217"`
	Rate       string    `json:"rate" binding:"required"`
	RecordedAt time.Time `json:"recorded_at" binding:"required"`
	Source     string    `json:"source" binding:"omitempty,max=32"`
}

// FXPairResponse names a currency pair.
type FXPairResponse struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
}

// FXPairsResponse lists the pairs the oracle should publish and the users' base currencies.
type FXPairsResponse struct {
	Pairs          []FXPairResponse `json:"pairs"`
	BaseCurrencies []string         `json:"base_currencies"`
}

// FXRatePoint is one stored observation of a pair.
type FXRatePoint struct {
	Rate       fx.Rate   `json:"rate" swaggertype:"string" example:"4.4712"`
	RecordedAt time.Time `json:"recorded_at"`
}

// FXRateHistoryResponse is the direct-pair history of an exchange rate.
type FXRateHistoryResponse struct {
	Base  string        `json:"base"`
	Quote string        `json:"quote"`
	Rates []FXRatePoint `json:"rates"`
}

// FXLatestResponse is the current rate of a pair, however it was resolved.
type FXLatestResponse struct {
	Base       string    `json:"base"`
	Quote      string    `json:"quote"`
	Rate       fx.Rate   `json:"rate" swaggertype:"string" example:"4.4712"`
	RecordedAt time.Time `json:"recorded_at"`
	Stale      bool      `json:"stale"`
	Path       string    `json:"path" enums:"identity,direct,inverse,pivot:USD"`
}

// fxPairQuery holds the query parameters naming a pair.
type fxPairQuery struct {
	Base  string `form:"base" binding:"required,iso4217"`
	Quote string `form:"quote" binding:"required,iso4217"`
}

// RecordRates handles bulk recording of exchange rates.
// @Summary     Record FX rates
// @Description Bulk upsert exchange rates on (base, quote, recorded_at) (pipeline endpoint). Rates are decimal strings with at most 10 fractional digits.
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       request body RecordFXRatesRequest true "Rate entries"
// @Success     200 {object} map[string]int "Rates recorded count"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/fx-rates [post]
func (h *FXHandler) RecordRates(c *gin.Context) {
	var req RecordFXRatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	inputs := make([]services.FXRateInput, len(req.Rates))
	for i, r := range req.Rates {
		rate, err := fx.ParseRate(r.Rate)
		if err != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput,
				fmt.Sprintf("rates[%d].rate: %s", i, err.Error())))
			return
		}
		inputs[i] = services.FXRateInput{
			Base:       r.Base,
			Quote:      r.Quote,
			Rate:       rate,
			RecordedAt: r.RecordedAt,
			Source:     r.Source,
		}
	}

	count, err := h.fxService.RecordRates(inputs)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log("", "PIPELINE_RECORD_FX_RATES", "fx_rate", "", c.ClientIP(),
		map[string]interface{}{"rates_recorded": count})

	c.JSON(http.StatusOK, gin.H{"recorded": count})
}

// GetPairs lists the currency pairs the oracle should publish.
// @Summary     List FX pairs to publish
// @Description Pairs of (account/security currency, user base currency), plus each base against USD, and the distinct user base currencies (pipeline endpoint)
// @Tags        pipeline
// @Produce     json
// @Security    ApiKeyAuth
// @Success     200 {object} FXPairsResponse "Pairs and base currencies"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     500 {object} ErrorResponse "Server error"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/fx-pairs [get]
func (h *FXHandler) GetPairs(c *gin.Context) {
	pairs, bases, err := h.fxService.NeededPairs()
	if err != nil {
		respondWithError(c, err)
		return
	}

	resp := FXPairsResponse{
		Pairs:          make([]FXPairResponse, len(pairs)),
		BaseCurrencies: bases,
	}
	if resp.BaseCurrencies == nil {
		resp.BaseCurrencies = []string{}
	}
	for i, p := range pairs {
		resp.Pairs[i] = FXPairResponse{Base: p.Base, Quote: p.Quote}
	}
	c.JSON(http.StatusOK, resp)
}

// ListRates returns the stored history of a currency pair.
// @Summary     FX rate history
// @Description Stored observations of the direct pair (no inverse or pivot), oldest first. Defaults to the last 30 days; the range is capped at 3700 days.
// @Tags        fx
// @Produce     json
// @Security    BearerAuth
// @Param       base  query string true  "Base currency (ISO 4217)"
// @Param       quote query string true  "Quote currency (ISO 4217)"
// @Param       from  query string false "Start (RFC3339 or YYYY-MM-DD)"
// @Param       to    query string false "End (RFC3339 or YYYY-MM-DD, inclusive)"
// @Success     200 {object} FXRateHistoryResponse "Rate history"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /fx-rates [get]
func (h *FXHandler) ListRates(c *gin.Context) {
	var q fxPairQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	from, to, err := parseFXRange(c.Query("from"), c.Query("to"), time.Now())
	if err != nil {
		respondWithError(c, err)
		return
	}

	rates, err := h.fxService.ListRates(q.Base, q.Quote, from, to)
	if err != nil {
		respondWithError(c, err)
		return
	}

	points := make([]FXRatePoint, len(rates))
	for i, r := range rates {
		points[i] = FXRatePoint{Rate: r.Rate, RecordedAt: r.RecordedAt}
	}
	c.JSON(http.StatusOK, FXRateHistoryResponse{Base: q.Base, Quote: q.Quote, Rates: points})
}

// LatestRate returns the current rate of a currency pair.
// @Summary     Latest FX rate
// @Description Current rate resolved directly, by inverse, or through USD, with its age flag. 503 when no rate exists.
// @Tags        fx
// @Produce     json
// @Security    BearerAuth
// @Param       base  query string true "Base currency (ISO 4217)"
// @Param       quote query string true "Quote currency (ISO 4217)"
// @Success     200 {object} FXLatestResponse "Latest rate"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     503 {object} ErrorResponse "Exchange rate unavailable"
// @Router      /fx-rates/latest [get]
func (h *FXHandler) LatestRate(c *gin.Context) {
	var q fxPairQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	quote, err := h.fxService.Rate(q.Base, q.Quote, time.Now())
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, FXLatestResponse{
		Base:       quote.Base,
		Quote:      quote.Quote,
		Rate:       quote.Rate,
		RecordedAt: quote.RecordedAt,
		Stale:      quote.Stale,
		Path:       quote.Path,
	})
}

// parseFXRange resolves the optional from/to query values. Missing values
// default to the 30 days up to now; a date-only "to" includes that whole day.
// It rejects an inverted range and one longer than fxMaxRangeDays.
func parseFXRange(fromStr, toStr string, now time.Time) (from, to time.Time, err error) {
	to = now
	if toStr != "" {
		to, err = parseFlexibleTime(toStr)
		if err != nil {
			return time.Time{}, time.Time{}, apperrors.WithMessage(apperrors.ErrInvalidInput, "to: "+err.Error())
		}
		if _, rfcErr := time.Parse(time.RFC3339, toStr); rfcErr != nil {
			to = to.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
	}
	from = to.AddDate(0, 0, -fxDefaultRangeDays)
	if fromStr != "" {
		from, err = parseFlexibleTime(fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, apperrors.WithMessage(apperrors.ErrInvalidInput, "from: "+err.Error())
		}
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, apperrors.WithMessage(apperrors.ErrInvalidInput, "from must not be after to")
	}
	if to.Sub(from) > fxMaxRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("range must not exceed %d days", fxMaxRangeDays))
	}
	return from, to, nil
}
