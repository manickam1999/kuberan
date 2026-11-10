package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// AccountHistoryHandler serves per-account balance history and sparklines
// (plan 023, 5.1).
type AccountHistoryHandler struct {
	service services.AccountBalanceHistoryServicer
}

// NewAccountHistoryHandler creates a new AccountHistoryHandler.
func NewAccountHistoryHandler(service services.AccountBalanceHistoryServicer) *AccountHistoryHandler {
	return &AccountHistoryHandler{service: service}
}

// GetBalanceHistory returns one account's daily balances.
// @Summary     Get account balance history
// @Description Daily balances of one account in the account's own currency, one point per recorded UTC day (written by the snapshot pipeline). from defaults to 90 days before to, to defaults to today; at most 3700 days.
// @Tags        accounts
// @Produce     json
// @Security    BearerAuth
// @Param       id   path  string true  "Account ID"
// @Param       from query string false "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to   query string false "End date (RFC3339 or YYYY-MM-DD)"
// @Success     200 {object} map[string]services.AccountBalanceHistory "Balance history"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account not found"
// @Router      /accounts/{id}/balance-history [get]
func (h *AccountHistoryHandler) GetBalanceHistory(c *gin.Context) {
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
	from, err := parseQueryDate(c, "from")
	if err != nil {
		respondWithError(c, err)
		return
	}
	to, err := parseQueryDate(c, "to")
	if err != nil {
		respondWithError(c, err)
		return
	}
	history, err := h.service.GetBalanceHistory(userID, accountID, from, to)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": history})
}

// GetSparklines returns the recent balances of every active account.
// @Summary     Get account sparklines
// @Description Daily balances over the last N days (default 30, 2 to 365) for each of the user's active accounts, in each account's own currency. Accounts with no recorded history have an empty points list.
// @Tags        accounts
// @Produce     json
// @Security    BearerAuth
// @Param       days query int false "Number of days (default 30, min 2, max 365)"
// @Success     200 {object} map[string][]services.AccountSparkline "Sparklines"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /accounts/sparklines [get]
func (h *AccountHistoryHandler) GetSparklines(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	days := 0
	if v := c.Query("days"); v != "" {
		days, err = strconv.Atoi(v)
		if err != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "days must be an integer"))
			return
		}
	}
	items, err := h.service.GetSparklines(userID, days)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// parseQueryDate parses an optional flexible date query parameter; the zero
// time means it was not provided.
func parseQueryDate(c *gin.Context, name string) (time.Time, error) {
	v := c.Query(name)
	return parseOptionalDate(&v)
}
