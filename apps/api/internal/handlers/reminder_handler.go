package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

// ReminderHandler serves reminders: the user's own view and switches, and the
// internal endpoints the Telegram bot polls (plan 023, 5.6).
type ReminderHandler struct {
	reminderService services.ReminderServicer
	auditService    services.AuditServicer
	now             func() time.Time
}

// NewReminderHandler creates a new ReminderHandler.
func NewReminderHandler(reminderService services.ReminderServicer, auditService services.AuditServicer) *ReminderHandler {
	return &ReminderHandler{reminderService: reminderService, auditService: auditService, now: time.Now}
}

// SetReminderEnabledRequest switches one account's reminder on or off.
type SetReminderEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// List returns the user's upcoming reminders.
// @Summary     List reminders
// @Description Upcoming (unsent or not yet due) reminders of the authenticated user, soonest first; ref_id limits it to one account
// @Tags        reminders
// @Produce     json
// @Param       ref_id query string false "Account ID"
// @Success     200 {object} object "Reminders"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /reminders [get]
// @Security    BearerAuth
func (h *ReminderHandler) List(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	reminders, err := h.reminderService.ListUpcoming(userID, c.Query("ref_id"), h.now())
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reminders": reminders})
}

// SetEnabled switches an account's reminder on or off.
// @Summary     Switch a reminder on or off
// @Description Enable or disable the reminder (all occurrences of its kind for that account)
// @Tags        reminders
// @Accept      json
// @Produce     json
// @Param       id path string true "Reminder ID"
// @Param       request body SetReminderEnabledRequest true "Switch"
// @Success     200 {object} object "Reminder"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Not found"
// @Router      /reminders/{id} [patch]
// @Security    BearerAuth
func (h *ReminderHandler) SetEnabled(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	var req SetReminderEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	reminder, err := h.reminderService.SetEnabled(userID, c.Param("id"), *req.Enabled)
	if err != nil {
		respondWithError(c, err)
		return
	}
	h.auditService.Log(userID, "UPDATE_REMINDER", "reminder", reminder.ID, c.ClientIP(),
		map[string]interface{}{"enabled": *req.Enabled})
	c.JSON(http.StatusOK, gin.H{"reminder": reminder})
}

// Due lists the reminders the bot should send now (called by the bot service).
// @Summary     Reminders due for delivery
// @Description Unsent, enabled reminders whose time has come, for users with an active Telegram link that has the kind switched on (internal endpoint)
// @Tags        internal
// @Produce     json
// @Success     200 {object} object "Due reminders"
// @Router      /internal/telegram/reminders/due [get]
// @Security    InternalSecret
func (h *ReminderHandler) Due(c *gin.Context) {
	due, err := h.reminderService.Due(h.now())
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reminders": due})
}

// MarkSent confirms a reminder was delivered (called by the bot service).
// @Summary     Confirm a reminder was sent
// @Description Mark a reminder as sent. Idempotent: confirming twice is fine (internal endpoint)
// @Tags        internal
// @Produce     json
// @Param       id path string true "Reminder ID"
// @Success     200 {object} object "Reminder"
// @Failure     404 {object} ErrorResponse "Not found"
// @Router      /internal/telegram/reminders/{id}/sent [post]
// @Security    InternalSecret
func (h *ReminderHandler) MarkSent(c *gin.Context) {
	reminder, err := h.reminderService.MarkSent(c.Param("id"), h.now())
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reminder": reminder})
}
