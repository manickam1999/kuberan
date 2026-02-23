package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// TelegramHandler handles Telegram-related requests.
type TelegramHandler struct {
	telegramService services.TelegramServicer
	auditService    services.AuditServicer
}

// NewTelegramHandler creates a new TelegramHandler.
func NewTelegramHandler(telegramService services.TelegramServicer, auditService services.AuditServicer) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
		auditService:    auditService,
	}
}

// CompleteLinkRequest represents the request to complete linking
type CompleteLinkRequest struct {
	LinkCode          string `json:"link_code" binding:"required,len=6"`
	TelegramUserID    int64  `json:"telegram_user_id" binding:"required"`
	TelegramUsername  string `json:"telegram_username"`
	TelegramFirstName string `json:"telegram_first_name"`
	DefaultCurrency   string `json:"default_currency" binding:"omitempty,iso4217"`
}

// GetLink retrieves the user's Telegram link status
// @Summary     Get Telegram link status
// @Description Get the current Telegram link for the authenticated user
// @Tags        telegram
// @Accept      json
// @Produce     json
// @Success     200 {object} object "Link information"
// @Failure     404 {object} ErrorResponse "Not found"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /telegram/link [get]
// @Security    BearerAuth
func (h *TelegramHandler) GetLink(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	link, err := h.telegramService.GetLinkByUserID(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"link": link,
	})
}

// GenerateCode generates a new link code for the user
// @Summary     Generate link code
// @Description Generate a new 6-character link code for linking Telegram account
// @Tags        telegram
// @Accept      json
// @Produce     json
// @Success     200 {object} object "Link code generated"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /telegram/generate-code [post]
// @Security    BearerAuth
func (h *TelegramHandler) GenerateCode(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	link, err := h.telegramService.GenerateLinkCode(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "GENERATE_TELEGRAM_CODE", "telegram_link", link.ID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{
		"link_code": link.LinkCode,
		"expires_at": link.LinkCodeExpiresAt,
	})
}

// Unlink unlinks the user's Telegram account
// @Summary     Unlink Telegram account
// @Description Remove the link between Telegram and Kuberan account
// @Tags        telegram
// @Accept      json
// @Produce     json
// @Success     200 {object} object "Success message"
// @Failure     404 {object} ErrorResponse "Not found"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /telegram/unlink [delete]
// @Security    BearerAuth
func (h *TelegramHandler) Unlink(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.telegramService.UnlinkAccount(userID); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UNLINK_TELEGRAM", "telegram_link", "", c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{
		"message": "Telegram account unlinked successfully",
	})
}

// CompleteLink completes the linking process (called by bot service)
// @Summary     Complete Telegram linking
// @Description Complete the linking process by verifying the link code (internal endpoint)
// @Tags        internal
// @Accept      json
// @Produce     json
// @Param       request body CompleteLinkRequest true "Link completion data"
// @Success     200 {object} object "Success message"
// @Failure     400 {object} ErrorResponse "Invalid code or expired"
// @Failure     409 {object} ErrorResponse "Telegram already linked"
// @Router      /internal/telegram/complete-link [post]
// @Security    InternalSecret
func (h *TelegramHandler) CompleteLink(c *gin.Context) {
	var req CompleteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	if err := h.telegramService.CompleteLink(req.LinkCode, req.TelegramUserID, req.TelegramUsername, req.TelegramFirstName, req.DefaultCurrency); err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Telegram account linked successfully",
	})
}

// ResolveUser resolves a Telegram user ID to Kuberan user with auth token (called by bot service)
// @Summary     Resolve Telegram user
// @Description Get Kuberan user info and auth token from Telegram user ID (internal endpoint)
// @Tags        internal
// @Accept      json
// @Produce     json
// @Param       telegram_user_id path int true "Telegram user ID"
// @Success     200 {object} object "User info with auth token"
// @Failure     404 {object} ErrorResponse "Not found"
// @Router      /internal/telegram/resolve/{telegram_user_id} [get]
// @Security    InternalSecret
func (h *TelegramHandler) ResolveUser(c *gin.Context) {
	telegramUserIDStr := c.Param("telegram_user_id")
	telegramUserID, err := strconv.ParseInt(telegramUserIDStr, 10, 64)
	if err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid Telegram user ID"))
		return
	}

	userData, err := h.telegramService.GetUserWithAuthToken(telegramUserID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, userData)
}

// RecordActivity records bot activity for a Telegram user (called by bot service)
// @Summary     Record bot activity
// @Description Update last message timestamp and increment message count (internal endpoint)
// @Tags        internal
// @Accept      json
// @Produce     json
// @Param       telegram_user_id path int true "Telegram user ID"
// @Success     200 {object} object "Success message"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Router      /internal/telegram/activity/{telegram_user_id} [post]
// @Security    InternalSecret
func (h *TelegramHandler) RecordActivity(c *gin.Context) {
	telegramUserIDStr := c.Param("telegram_user_id")
	telegramUserID, err := strconv.ParseInt(telegramUserIDStr, 10, 64)
	if err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid Telegram user ID"))
		return
	}

	if err := h.telegramService.RecordActivity(telegramUserID); err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Activity recorded",
	})
}
