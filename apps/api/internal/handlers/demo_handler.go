package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// DemoHandler serves the sample-household endpoints (plan 023 5.7).
type DemoHandler struct {
	demoService  services.DemoServicer
	auditService services.AuditServicer
	enabled      bool
}

// NewDemoHandler creates a DemoHandler. When enabled is false (DEMO_ENABLED
// unset) every route answers 404, as if it did not exist.
func NewDemoHandler(demoService services.DemoServicer, auditService services.AuditServicer, enabled bool) *DemoHandler {
	return &DemoHandler{demoService: demoService, auditService: auditService, enabled: enabled}
}

// LoadDemo creates the sample household.
// @Summary     Load the sample household
// @Description Creates sample accounts, categories, about three months of transactions, a budget, holdings with prices and receivables, listing every row in demo_rows. Only when the user has no transactions of their own (409 DEMO_HAS_DATA) and has not loaded it yet (409 DEMO_ALREADY_LOADED). 404 unless DEMO_ENABLED is set.
// @Tags        demo
// @Produce     json
// @Security    BearerAuth
// @Success     201 {object} services.DemoResult
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Demo data is not enabled"
// @Failure     409 {object} ErrorResponse "DEMO_ALREADY_LOADED, DEMO_HAS_DATA or DEMO_BUSY"
// @Router      /demo/load [post]
func (h *DemoHandler) LoadDemo(c *gin.Context) {
	if !h.enabled {
		respondWithError(c, apperrors.ErrNotFound)
		return
	}
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	res, err := h.demoService.Load(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}
	h.auditService.Log(userID, "DEMO_LOAD", "demo", "", c.ClientIP(), map[string]interface{}{"rows": res.Total})
	c.JSON(http.StatusCreated, res)
}

// RemoveDemo hard-deletes the sample household.
// @Summary     Remove the sample household
// @Description Permanently deletes exactly the rows recorded in demo_rows, children first, in one transaction. Idempotent: with nothing loaded it returns an empty result. 409 DEMO_IN_USE when the user's own entries still use the sample accounts. 404 unless DEMO_ENABLED is set.
// @Tags        demo
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} services.DemoResult
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Demo data is not enabled"
// @Failure     409 {object} ErrorResponse "DEMO_IN_USE or DEMO_BUSY"
// @Router      /demo [delete]
func (h *DemoHandler) RemoveDemo(c *gin.Context) {
	if !h.enabled {
		respondWithError(c, apperrors.ErrNotFound)
		return
	}
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	res, err := h.demoService.Remove(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}
	h.auditService.Log(userID, "DEMO_REMOVE", "demo", "", c.ClientIP(), map[string]interface{}{"rows": res.Total})
	c.JSON(http.StatusOK, res)
}
