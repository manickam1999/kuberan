package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// AnalyticsHandler serves the Insights summary (plan 023, 5.4).
type AnalyticsHandler struct {
	typical services.TypicalServicer
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(typical services.TypicalServicer) *AnalyticsHandler {
	return &AnalyticsHandler{typical: typical}
}

// GetTypical returns the user's typical month.
// @Summary     Get typical month
// @Description Averages over the last N complete months (default 6, 1 to 24), in the base currency: monthly income, spending and saved percentage, the average cumulative daily spending curve (31 points) and average spending by category. months_used says how many months the averages cover (0 when there is no history).
// @Tags        analytics
// @Produce     json
// @Security    BearerAuth
// @Param       months query int false "Number of complete months (default 6, min 1, max 24)"
// @Success     200 {object} map[string]services.Typical "Typical month"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /analytics/typical [get]
func (h *AnalyticsHandler) GetTypical(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	months := 6
	if v := c.Query("months"); v != "" {
		months, err = strconv.Atoi(v)
		if err != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "months must be an integer"))
			return
		}
	}
	typical, err := h.typical.GetTypical(userID, months)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": typical})
}
