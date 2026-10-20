package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// FixedDepositPipelineHandler handles pipeline-triggered fixed deposit processing.
type FixedDepositPipelineHandler struct {
	fdService services.FixedDepositServicer
}

// NewFixedDepositPipelineHandler creates a new FixedDepositPipelineHandler.
func NewFixedDepositPipelineHandler(fdService services.FixedDepositServicer) *FixedDepositPipelineHandler {
	return &FixedDepositPipelineHandler{fdService: fdService}
}

// ProcessFixedDepositsRequest is the optional request payload for triggering FD processing.
// If AsOf is omitted, the server uses the current UTC time.
type ProcessFixedDepositsRequest struct {
	AsOf *time.Time `json:"as_of"`
}

// ProcessFixedDeposits handles the pipeline endpoint that runs scheduled FD processing.
// @Summary     Process fixed deposits
// @Description Run scheduled fixed-deposit processing: post periodic interest payouts and flip matured FDs (pipeline endpoint)
// @Tags        pipeline
// @Accept      json
// @Produce     json
// @Security    ApiKeyAuth
// @Param       request body ProcessFixedDepositsRequest false "Processing parameters (as_of defaults to now)"
// @Success     200 {object} map[string]int "payouts_posted and fds_matured counts"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid API key"
// @Router      /pipeline/fixed-deposits/process [post]
func (h *FixedDepositPipelineHandler) ProcessFixedDeposits(c *gin.Context) {
	var req ProcessFixedDepositsRequest
	// Body is optional - ignore bind errors and fall back to time.Now().
	_ = c.ShouldBindJSON(&req)

	asOf := time.Now().UTC()
	if req.AsOf != nil {
		asOf = *req.AsOf
	}

	result, err := h.fdService.ProcessFixedDeposits(asOf)
	if err != nil {
		respondWithError(c, apperrors.Wrap(apperrors.ErrInternalServer, err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payouts_posted": result.PayoutsPosted,
		"fds_matured":    result.FDsMatured,
	})
}
