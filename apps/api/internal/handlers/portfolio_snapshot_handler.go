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

// PortfolioSnapshotHandler handles portfolio snapshot requests.
type PortfolioSnapshotHandler struct {
	snapshotService services.PortfolioSnapshotServicer
	auditService    services.AuditServicer
}

// NewPortfolioSnapshotHandler creates a new PortfolioSnapshotHandler.
func NewPortfolioSnapshotHandler(snapshotService services.PortfolioSnapshotServicer, auditService services.AuditServicer) *PortfolioSnapshotHandler {
	return &PortfolioSnapshotHandler{snapshotService: snapshotService, auditService: auditService}
}

// snapshotPageRequest allows a larger page size for snapshot queries (up to 3650
// for ~10 years of daily snapshots) while keeping the shared PageRequest at 100.
type snapshotPageRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=3650"`
}

// ComputeSnapshotsRequest represents the request payload for computing snapshots.
type ComputeSnapshotsRequest struct {
	RecordedAt time.Time `json:"recorded_at" binding:"required"`
}

// ComputeSnapshots handles computing and recording portfolio snapshots.
// @Summary     Compute portfolio snapshots
// @Description Compute and record portfolio snapshots for all users (pipeline endpoint)
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       request body ComputeSnapshotsRequest true "Snapshot parameters"
// @Success     200 {object} map[string]int "Snapshots recorded count"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Failure     503 {object} ErrorResponse "Pipeline not configured"
// @Router      /pipeline/snapshots [post]
func (h *PortfolioSnapshotHandler) ComputeSnapshots(c *gin.Context) {
	var req ComputeSnapshotsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	count, err := h.snapshotService.ComputeAndRecordSnapshots(req.RecordedAt)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"snapshots_recorded": count})
}

// GetSnapshots handles retrieving portfolio snapshots for the authenticated user.
// @Summary     Get portfolio snapshots
// @Description Get paginated portfolio snapshots for a date range
// @Tags        investments
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       from_date query string true  "Start date (RFC3339 or YYYY-MM-DD)"
// @Param       to_date   query string true  "End date (RFC3339 or YYYY-MM-DD)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Items per page (default 20, max 3650)"
// @Success     200 {object} pagination.PageResponse[models.PortfolioSnapshot] "Paginated snapshots"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /investments/snapshots [get]
func (h *PortfolioSnapshotHandler) GetSnapshots(c *gin.Context) {
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

	var sp snapshotPageRequest
	if err := c.ShouldBindQuery(&sp); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	page := pagination.PageRequest{Page: sp.Page, PageSize: sp.PageSize}

	result, err := h.snapshotService.GetSnapshots(userID, from, to, page)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}
