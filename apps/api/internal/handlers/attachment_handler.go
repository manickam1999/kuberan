package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// AttachmentHandler handles transaction receipt attachment requests
// (upload, list, download, delete). See plans/017-transaction-receipts.
type AttachmentHandler struct {
	attachmentService services.AttachmentServicer
	auditService      services.AuditServicer
	maxUploadBytes    int64
}

// NewAttachmentHandler creates a new AttachmentHandler. maxUploadBytes bounds
// the request body before it is parsed so an oversized upload is rejected
// without buffering it into memory.
func NewAttachmentHandler(attachmentService services.AttachmentServicer, auditService services.AuditServicer, maxUploadBytes int64) *AttachmentHandler {
	return &AttachmentHandler{
		attachmentService: attachmentService,
		auditService:      auditService,
		maxUploadBytes:    maxUploadBytes,
	}
}

// Upload attaches a receipt (image or PDF) to a transaction
// @Summary     Upload a transaction attachment
// @Description Upload a receipt image (JPEG, PNG, WebP) or PDF for a transaction. The image is re-encoded server-side to strip EXIF metadata.
// @Tags        transactions,attachments
// @Accept      multipart/form-data
// @Produce     json
// @Security    BearerAuth
// @Param       id   path     string true "Transaction ID"
// @Param       file formData file   true "Receipt file (max 10 MiB)"
// @Success     201 {object} map[string]models.TransactionAttachment "Attachment created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Transaction not found"
// @Failure     409 {object} ErrorResponse "Attachment limit reached"
// @Failure     413 {object} ErrorResponse "File too large"
// @Failure     415 {object} ErrorResponse "Unsupported file type"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id}/attachments [post]
func (h *AttachmentHandler) Upload(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	txID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	// Cap the request body before parsing so an oversized upload can't exhaust
	// memory. Give one byte of headroom over the service cap so the byte-count
	// check there produces a clean 413 rather than tripping this reader first.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+1)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondWithError(c, apperrors.ErrPayloadTooLarge)
			return
		}
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "missing or invalid 'file' upload"))
		return
	}
	defer func() { _ = file.Close() }()

	att, err := h.attachmentService.Upload(
		userID,
		txID,
		header.Filename,
		header.Header.Get("Content-Type"),
		header.Size,
		file,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPLOAD_ATTACHMENT", "attachment", att.ID, c.ClientIP(),
		map[string]interface{}{"transaction_id": txID, "content_type": att.ContentType, "byte_size": att.ByteSize})

	c.JSON(http.StatusCreated, gin.H{"attachment": att})
}

// List returns the attachment metadata for a transaction
// @Summary     List transaction attachments
// @Description List the receipt attachments for a transaction (metadata only, no bytes)
// @Tags        transactions,attachments
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Transaction ID"
// @Success     200 {object} map[string][]models.TransactionAttachment "Attachments"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id}/attachments [get]
func (h *AttachmentHandler) List(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	txID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	atts, err := h.attachmentService.List(userID, txID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"attachments": atts})
}

// Download streams an attachment's bytes with hardened response headers
// @Summary     Download a transaction attachment
// @Description Stream the raw bytes of a receipt attachment. Ownership is enforced.
// @Tags        transactions,attachments
// @Produce     octet-stream
// @Security    BearerAuth
// @Param       id  path string true "Transaction ID"
// @Param       aid path string true "Attachment ID"
// @Success     200 "Raw attachment bytes"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Attachment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id}/attachments/{aid} [get]
func (h *AttachmentHandler) Download(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if _, err := parsePathID(c, "id"); err != nil {
		respondWithError(c, err)
		return
	}

	aid, err := parsePathID(c, "aid")
	if err != nil {
		respondWithError(c, err)
		return
	}

	att, stream, err := h.attachmentService.Open(userID, aid)
	if err != nil {
		respondWithError(c, err)
		return
	}
	defer func() { _ = stream.Close() }()

	// Serve the sniffed content type only, inline, with a strict CSP so a
	// malicious payload cannot be interpreted as active content by the browser.
	c.Header("Content-Type", att.ContentType)
	c.Header("Content-Disposition", "inline")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, no-store")

	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		// Headers/status are already committed; nothing left but to log.
		_ = c.Error(err)
	}
}

// Delete removes an attachment from a transaction
// @Summary     Delete a transaction attachment
// @Description Delete a receipt attachment (soft-deletes metadata and removes the stored object)
// @Tags        transactions,attachments
// @Produce     json
// @Security    BearerAuth
// @Param       id  path string true "Transaction ID"
// @Param       aid path string true "Attachment ID"
// @Success     204 "Attachment deleted"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Attachment not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id}/attachments/{aid} [delete]
func (h *AttachmentHandler) Delete(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if _, err := parsePathID(c, "id"); err != nil {
		respondWithError(c, err)
		return
	}

	aid, err := parsePathID(c, "aid")
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.attachmentService.Delete(userID, aid); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_ATTACHMENT", "attachment", aid, c.ClientIP(), nil)

	c.Status(http.StatusNoContent)
}
