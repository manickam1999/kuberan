package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/services"
)

// exportWriteWindow is how long a single export may keep streaming. It
// overrides the server-wide WriteTimeout for this response only.
const exportWriteWindow = 10 * time.Minute

// ExportHandler streams the user's data export and reports system status.
type ExportHandler struct {
	exportService services.ExportServicer
	statusService services.SystemStatusServicer
	auditService  services.AuditServicer
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exportService services.ExportServicer, statusService services.SystemStatusServicer, auditService services.AuditServicer) *ExportHandler {
	return &ExportHandler{exportService: exportService, statusService: statusService, auditService: auditService}
}

// Export streams a zip of CSV files with all of the user's data.
// @Summary     Export all data
// @Description Streams a zip archive with one CSV per table (accounts, categories, transactions, budgets, investments, investment_transactions, securities, receivables, receivable_settlements, rules). Scoped to the caller; soft-deleted rows are included with deleted_at. Money columns carry the integer cents, a formatted decimal and the currency, plus the base-currency (_base) and FX columns. Limited to 1 request per minute per user.
// @Tags        export
// @Produce     application/zip
// @Security    BearerAuth
// @Success     200 {file} file "kuberan-export-YYYY-MM-DD.zip"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     429 {object} ErrorResponse "Rate limited (Retry-After seconds)"
// @Failure     500 {object} ErrorResponse "Internal server error"
// @Router      /export [get]
func (h *ExportHandler) Export(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	// The server's WriteTimeout (120s) would cut a large export short; give this
	// response its own window. Unsupported writers (tests) just keep the default.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(exportWriteWindow))

	now := time.Now()
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="`+services.ExportFileName(now)+`"`)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	if err := h.exportService.WriteExport(userID, c.Writer); err != nil {
		logger.Get().Errorw("data export failed", "error", err, "user_id", userID)
		if !c.Writer.Written() {
			// Nothing sent yet: replace the download headers with a normal error.
			c.Writer.Header().Del("Content-Disposition")
			c.Writer.Header().Del("Content-Type")
			c.Writer.Header().Del("Cache-Control")
			respondWithError(c, err)
			return
		}
		// Mid-stream failure: the status line is gone, so the truncated zip
		// (no central directory) is the client's signal that it is incomplete.
		c.Abort()
		return
	}

	h.auditService.Log(userID, "EXPORT_DATA", "export", "", c.ClientIP(), nil)
}

// SystemStatusResponse mirrors services.SystemStatus for Swagger.
type SystemStatusResponse struct {
	LatestPriceAt    *time.Time `json:"latest_price_at"`
	LatestFXRateAt   *time.Time `json:"latest_fx_rate_at"`
	LatestSnapshotAt *time.Time `json:"latest_snapshot_at"`
	LastBackupAt     *time.Time `json:"last_backup_at"`
}

// GetSystemStatus reports when prices, FX rates, snapshots and backups last ran.
// @Summary     System status
// @Description Freshness timestamps for the host-status chip. Each field is an RFC 3339 time or null when it has never happened. latest_snapshot_at is the caller's own latest portfolio snapshot.
// @Tags        system
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} SystemStatusResponse
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Internal server error"
// @Router      /system/status [get]
func (h *ExportHandler) GetSystemStatus(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	st, err := h.statusService.GetStatus(userID)
	if err != nil {
		respondWithError(c, apperrors.Wrap(apperrors.ErrInternalServer, err))
		return
	}
	c.JSON(http.StatusOK, SystemStatusResponse(*st))
}
