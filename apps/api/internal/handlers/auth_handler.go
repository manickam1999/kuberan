package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"kuberan/internal/appearance"
	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/uuid"
)

// RefreshCookieName is the HttpOnly cookie that carries the refresh token.
const RefreshCookieName = "kuberan_refresh"

// refreshCookiePath scopes the refresh cookie to the auth endpoints, so it is
// not sent with ordinary API requests.
const refreshCookiePath = "/api/v1/auth"

// maxAuditEmailLen bounds attacker-supplied emails stored in audit metadata.
const maxAuditEmailLen = 254

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	userService  services.UserServicer
	auditService services.AuditServicer
	secureCookie bool
	// defaultPalette is the validated DEFAULT_PALETTE (WithDefaultPalette).
	defaultPalette string
}

// AuthHandlerOption configures an AuthHandler.
type AuthHandlerOption func(*AuthHandler)

// WithSecureCookie sets whether the refresh cookie carries the Secure flag
// (true by default; config COOKIE_SECURE turns it off for plain-HTTP dev).
func WithSecureCookie(secure bool) AuthHandlerOption {
	return func(h *AuthHandler) { h.secureCookie = secure }
}

// WithDefaultPalette sets the server-wide default palette (DEFAULT_PALETTE).
// An unknown id is ignored and the built-in default stays.
func WithDefaultPalette(id string) AuthHandlerOption {
	return func(h *AuthHandler) {
		if appearance.IsPalette(id) {
			h.defaultPalette = id
		}
	}
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userService services.UserServicer, auditService services.AuditServicer, opts ...AuthHandlerOption) *AuthHandler {
	h := &AuthHandler{userService: userService, auditService: auditService, secureCookie: true,
		defaultPalette: appearance.DefaultPalette}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRequest represents the registration request payload. Password is
// capped at 72 runes here and 72 bytes in the service (bcrypt's limit).
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email,max=254"`
	Password  string `json:"password" binding:"required,min=8,max=72"`
	FirstName string `json:"first_name" binding:"max=100"`
	LastName  string `json:"last_name" binding:"max=100"`
	// BaseCurrency is the optional reporting currency (ISO 4217, 2-decimal,
	// same rule as PATCH /profile). Omitted keeps the default (MYR).
	BaseCurrency string `json:"base_currency" binding:"omitempty,iso4217" example:"EUR"`
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,max=1024"`
}

// RefreshRequest is the legacy refresh/logout body. The refresh token now
// travels in the kuberan_refresh cookie; the body is only read when the cookie
// is absent (migration window for clients that stored the token themselves).
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// UserResponse represents the user data in the response
type UserResponse struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// BaseCurrency is the ISO 4217 currency the user's aggregates are reported in.
	BaseCurrency string `json:"base_currency"`
	// Appearance holds the user's UI preferences; a missing key means default.
	Appearance models.Appearance `json:"appearance"`
	// ResolvedPalette is the palette to render: the user's choice, else the
	// server's DEFAULT_PALETTE, else catppuccin.
	ResolvedPalette string `json:"resolved_palette" example:"catppuccin"`
}

// AuthResponse represents the authentication response. The refresh token is
// set as the HttpOnly kuberan_refresh cookie, never returned in the body.
type AuthResponse struct {
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

// Register handles user registration
// @Summary     Register a new user
// @Description Register a new user with email and password
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       request body RegisterRequest true "User registration data"
// @Success     201 {object} AuthResponse "User registered and tokens generated"
// @Failure     400 {object} ErrorResponse "Invalid input (UNSUPPORTED_CURRENCY for a bad base_currency)"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	user, err := h.userService.CreateUser(req.Email, req.Password, req.FirstName, req.LastName,
		services.WithBaseCurrency(req.BaseCurrency))
	if err != nil {
		respondWithError(c, err)
		return
	}

	accessToken, refreshToken, err := h.startSession(c, user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(user.ID, "REGISTER", "user", user.ID, c.ClientIP(), nil)

	h.setRefreshCookie(c, refreshToken)
	c.JSON(http.StatusCreated, gin.H{
		"access_token": accessToken,
		"user":         h.userPayload(user),
	})
}

// Login handles user login
// @Summary     Login user
// @Description Authenticate a user and get access and refresh tokens
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       request body LoginRequest true "User login credentials"
// @Success     200 {object} AuthResponse "User authenticated and tokens generated"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid credentials (also returned while the account is locked)"
// @Failure     429 {object} ErrorResponse "Too many attempts"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	user, err := h.userService.AttemptLogin(req.Email, req.Password)
	if err != nil {
		h.auditLoginFailure(c, "LOGIN_FAILED", req.Email, err)
		respondWithError(c, err)
		return
	}

	accessToken, refreshToken, err := h.startSession(c, user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(user.ID, "LOGIN", "user", user.ID, c.ClientIP(), nil)

	h.setRefreshCookie(c, refreshToken)
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"user":         h.userPayload(user),
	})
}

// auditLoginFailure records a failed login under action, tagging why it
// failed (the response is the same 401 either way) and, when the attempt
// locked the account, a separate ACCOUNT_LOCKED event for the user.
func (h *AuthHandler) auditLoginFailure(c *gin.Context, action, email string, err error) {
	email = truncateRunes(email, maxAuditEmailLen)
	reason := "invalid_credentials"
	switch err {
	case services.ErrLoginLocked:
		reason = "account_locked"
	case services.ErrLoginLockoutTriggered:
		reason = "lockout_triggered"
	}
	if !errors.Is(err, apperrors.ErrInvalidCredentials) {
		reason = "error"
	}
	h.auditService.Log("", action, "user", "", c.ClientIP(),
		map[string]interface{}{"email": email, "reason": reason})

	if err == services.ErrLoginLockoutTriggered {
		userID := ""
		if user, lookupErr := h.userService.GetUserByEmail(email); lookupErr == nil {
			userID = user.ID
		}
		h.auditService.Log(userID, "ACCOUNT_LOCKED", "user", userID, c.ClientIP(),
			map[string]interface{}{"email": email})
	}
}

// RefreshToken exchanges the refresh token (kuberan_refresh cookie, or a
// legacy JSON body when no cookie is present) for a new access token, rotates
// the refresh cookie, and returns the user so the web app can bootstrap a
// session in one call.
// @Summary     Refresh access token
// @Description Exchange the refresh cookie for a new access token and rotated refresh cookie
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       request body RefreshRequest false "Legacy: refresh token, only read when the cookie is absent"
// @Success     200 {object} AuthResponse "New access token; refresh cookie rotated"
// @Failure     401 {object} ErrorResponse "Missing, invalid or expired refresh token"
// @Failure     429 {object} ErrorResponse "Too many requests"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	presented := refreshTokenFromRequest(c)
	if presented == "" {
		respondWithError(c, apperrors.ErrUnauthorized)
		return
	}

	// Validate the refresh token JWT
	claims, err := middleware.ValidateRefreshToken(presented)
	if err != nil {
		h.clearRefreshCookie(c)
		respondWithError(c, apperrors.ErrUnauthorized)
		return
	}

	user, err := h.userService.GetUserByID(claims.UserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserNotFound) {
			h.clearRefreshCookie(c)
			respondWithError(c, apperrors.ErrUnauthorized)
			return
		}
		respondWithError(c, err)
		return
	}
	if !user.IsActive {
		h.clearRefreshCookie(c)
		respondWithError(c, apperrors.ErrUnauthorized)
		return
	}

	accessToken, refreshToken, err := issueTokenPair(user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	// Rotate within this device's session only; other devices are unaffected.
	if err := h.userService.RotateRefreshSession(user.ID, middleware.HashToken(presented),
		middleware.HashToken(refreshToken), time.Now().Add(middleware.RefreshTokenExpiry)); err != nil {
		if errors.Is(err, apperrors.ErrUnauthorized) {
			// A validly signed token that matches no live session: revoked
			// (logout), rotated out past the grace window, or replayed.
			h.auditService.Log(user.ID, "REFRESH_REJECTED", "user", user.ID, c.ClientIP(),
				map[string]interface{}{"reason": "no_matching_session"})
			h.clearRefreshCookie(c)
		}
		respondWithError(c, err)
		return
	}

	h.setRefreshCookie(c, refreshToken)
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"user":         h.userPayload(user),
	})
}

// Logout ends the device session that owns the refresh token (cookie, or a
// legacy JSON body) and clears the cookie. Other devices stay signed in. It
// always succeeds so a stale token can't block logout.
// @Summary     Logout
// @Description Revoke the refresh session for this device and clear the refresh cookie
// @Tags        auth
// @Accept      json
// @Param       request body RefreshRequest false "Legacy: refresh token, only read when the cookie is absent"
// @Success     204 "Logged out"
// @Router      /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	presented := refreshTokenFromRequest(c)
	h.clearRefreshCookie(c)

	if claims, err := middleware.ValidateRefreshToken(presented); presented != "" && err == nil {
		if err := h.userService.RevokeRefreshSession(claims.UserID, middleware.HashToken(presented)); err != nil {
			respondWithError(c, err)
			return
		}
		h.auditService.Log(claims.UserID, "LOGOUT", "user", claims.UserID, c.ClientIP(), nil)
	}

	c.Status(http.StatusNoContent)
}

// refreshTokenFromRequest returns the refresh token from the kuberan_refresh
// cookie, falling back to a JSON body {"refresh_token": ...} when there is no
// cookie. Returns "" when neither is present.
func refreshTokenFromRequest(c *gin.Context) string {
	if v, err := c.Cookie(RefreshCookieName); err == nil && v != "" {
		return v
	}
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return ""
	}
	return req.RefreshToken
}

// setRefreshCookie stores the refresh token in a host-only, HttpOnly,
// SameSite=Strict cookie scoped to the auth endpoints.
func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   int(middleware.RefreshTokenExpiry / time.Second),
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

// truncateRunes cuts s to at most limit runes.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

// clearRefreshCookie expires the refresh cookie.
func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

// AppearanceDefaultResponse is the public, unauthenticated appearance payload.
type AppearanceDefaultResponse struct {
	// DefaultPalette is the server-wide palette id (DEFAULT_PALETTE).
	DefaultPalette string `json:"default_palette" example:"catppuccin"`
}

// GetAppearance returns the server's default palette. It is public because the
// sign-in, register and OAuth consent pages render before anyone is signed in,
// and exposes nothing else.
// @Summary     Get the server's default palette
// @Description Public. Returns only the palette used by users who have not picked one.
// @Tags        appearance
// @Produce     json
// @Success     200 {object} AppearanceDefaultResponse "Server default palette"
// @Router      /appearance [get]
func (h *AuthHandler) GetAppearance(c *gin.Context) {
	c.JSON(http.StatusOK, AppearanceDefaultResponse{DefaultPalette: h.defaultPalette})
}

// GetProfile returns the user's profile
// @Summary     Get user profile
// @Description Get the authenticated user's profile information
// @Tags        user
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} UserResponse "User profile"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /profile [get]
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": h.userPayload(user),
	})
}

// userPayload is the user object embedded in auth and profile responses.
func (h *AuthHandler) userPayload(user *models.User) gin.H {
	return gin.H{
		"id":                 user.ID,
		"email":              user.Email,
		"first_name":         user.FirstName,
		"last_name":          user.LastName,
		"hide_balances":      user.HideBalances,
		"default_account_id": user.DefaultAccountID,
		"base_currency":      user.BaseCurrency,
		"appearance":         user.Appearance,
		"resolved_palette":   appearance.Resolve(user.Appearance.Palette, h.defaultPalette),
		"last_login_at":      user.LastLoginAt,
	}
}

// UpdateProfileSettingsRequest represents the payload for updating profile settings.
// Every field is optional, but at least one must be present.
type UpdateProfileSettingsRequest struct {
	HideBalances *bool `json:"hide_balances"`
	// DefaultAccountID sets the account preselected for new transactions;
	// null or "" clears it. Must be a UUID otherwise.
	DefaultAccountID optionalString `json:"default_account_id" swaggertype:"string"`
	// BaseCurrency sets the currency aggregates are reported in (ISO 4217,
	// upper-case). It must be a supported currency with an exchange rate to USD.
	BaseCurrency *string `json:"base_currency" binding:"omitempty,iso4217"`
	// FirstName and LastName replace the user's names (empty clears).
	FirstName *string `json:"first_name" binding:"omitempty,max=100"`
	LastName  *string `json:"last_name" binding:"omitempty,max=100"`
	// Email replaces the login email (trimmed and lower-cased). Changing it
	// requires CurrentPassword; an email held by another user is 409
	// DUPLICATE_EMAIL.
	Email *string `json:"email" binding:"omitempty,email,max=254"`
	// CurrentPassword confirms an email change. A wrong value is 400
	// INVALID_CURRENT_PASSWORD and counts toward the login lockout.
	CurrentPassword string `json:"current_password" binding:"max=1024"`
	// Appearance is a partial update of the UI preferences (palette, mode,
	// chart_style, density, show_cents). Only the keys sent change; unknown
	// keys and values are rejected.
	Appearance *services.AppearancePatch `json:"appearance"`
}

// optionalString distinguishes an absent JSON field (Set=false) from an
// explicit null (Set=true, Value=nil).
type optionalString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON records that the field was present and decodes its value.
func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	return json.Unmarshal(data, &o.Value)
}

// UpdateProfileSettings updates the authenticated user's profile settings.
// @Summary     Update profile settings
// @Description Update the authenticated user's profile: name, email (needs current_password), dashboard balance visibility, default transaction account, base currency, appearance preferences (partial merge)
// @Tags        user
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body UpdateProfileSettingsRequest true "Profile settings to update"
// @Success     200 {object} UserResponse "Updated user profile"
// @Failure     400 {object} ErrorResponse "Invalid input, or INVALID_CURRENT_PASSWORD"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     409 {object} ErrorResponse "DUPLICATE_EMAIL or BASE_CURRENCY_LOCKED"
// @Failure     423 {object} ErrorResponse "ACCOUNT_LOCKED after too many wrong current passwords"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /profile [patch]
func (h *AuthHandler) UpdateProfileSettings(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateProfileSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	if req.HideBalances == nil && !req.DefaultAccountID.Set && req.BaseCurrency == nil &&
		req.FirstName == nil && req.LastName == nil && req.Email == nil && req.Appearance.IsEmpty() {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput,
			"at least one of hide_balances, default_account_id, base_currency, first_name, last_name, email or appearance is required"))
		return
	}

	update := services.ProfileSettingsUpdate{
		HideBalances:      req.HideBalances,
		SetDefaultAccount: req.DefaultAccountID.Set,
		BaseCurrency:      req.BaseCurrency,
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		Email:             req.Email,
		CurrentPassword:   req.CurrentPassword,
		Appearance:        req.Appearance,
	}
	if req.DefaultAccountID.Set {
		update.DefaultAccountID, err = optionalUUID(req.DefaultAccountID.Value, "default_account_id")
		if err != nil {
			respondWithError(c, err)
			return
		}
	}

	// The previous values go into the audit entries.
	before, err := h.userService.GetUserByID(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.userService.UpdateProfileSettings(userID, update); err != nil {
		if update.Email != nil && errors.Is(err, apperrors.ErrAccountLocked) {
			h.auditService.Log(userID, "ACCOUNT_LOCKED", "user", userID, c.ClientIP(),
				map[string]interface{}{"reason": "email_change_password"})
		}
		respondWithError(c, err)
		return
	}

	changes := map[string]interface{}{}
	if update.HideBalances != nil {
		changes["hide_balances"] = *update.HideBalances
	}
	if update.SetDefaultAccount {
		changes["default_account_id"] = update.DefaultAccountID
	}
	if update.FirstName != nil {
		changes["first_name"] = strings.TrimSpace(*update.FirstName)
	}
	if update.LastName != nil {
		changes["last_name"] = strings.TrimSpace(*update.LastName)
	}
	if !req.Appearance.IsEmpty() {
		changes["appearance"] = req.Appearance
	}
	if len(changes) > 0 {
		h.auditService.Log(userID, "UPDATE_PROFILE", "user", userID, c.ClientIP(), changes)
	}
	if update.BaseCurrency != nil {
		h.auditService.Log(userID, "UPDATE_BASE_CURRENCY", "user", userID, c.ClientIP(),
			map[string]interface{}{"old": before.BaseCurrency, "new": strings.ToUpper(strings.TrimSpace(*update.BaseCurrency))})
	}
	if update.Email != nil {
		if newEmail := services.NormalizeEmail(*update.Email); newEmail != before.Email {
			h.auditService.Log(userID, "UPDATE_EMAIL", "user", userID, c.ClientIP(),
				map[string]interface{}{"old": before.Email, "new": newEmail})
		}
	}

	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": h.userPayload(user),
	})
}

// startSession issues a token pair for a fresh login and records a new
// per-device refresh session for it.
func (h *AuthHandler) startSession(c *gin.Context, user *models.User) (accessToken, refreshToken string, err error) {
	accessToken, refreshToken, err = issueTokenPair(user)
	if err != nil {
		return "", "", err
	}

	if err := h.userService.CreateRefreshSession(user.ID, middleware.HashToken(refreshToken),
		c.Request.UserAgent(), c.ClientIP(), time.Now().Add(middleware.RefreshTokenExpiry)); err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// issueTokenPair signs a new access/refresh token pair without persisting it.
func issueTokenPair(user *models.User) (accessToken, refreshToken string, err error) {
	accessToken, err = middleware.GenerateAccessToken(user)
	if err != nil {
		return "", "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	refreshToken, err = middleware.GenerateRefreshToken(user)
	if err != nil {
		return "", "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return accessToken, refreshToken, nil
}

// ErrorDetail represents the inner error object in an error response.
type ErrorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"` // machine-readable context, e.g. FX_AMOUNT_REQUIRED's suggested_amount
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ChangePasswordRequest is the POST /auth/password payload. NewPassword
// follows the registration rules (8 to 72 bytes).
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required,max=1024"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

// RevokedSessionsResponse reports how many other device sessions were ended.
type RevokedSessionsResponse struct {
	RevokedSessions int64 `json:"revoked_sessions"`
}

// SessionResponse is one signed-in device in GET /auth/sessions.
type SessionResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	// LastSeenAt is the last token rotation, or CreatedAt before the first.
	LastSeenAt time.Time `json:"last_seen_at"`
	UserAgent  string    `json:"user_agent"`
	IPAddress  string    `json:"ip_address"`
	// Current is true for the session the request's access token belongs to.
	Current bool `json:"current"`
}

// ChangePassword changes the authenticated user's password.
// @Summary     Change password
// @Description Verify the current password, set a new one and end every other device session (the current one stays signed in). Wrong current passwords count toward the login lockout.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body ChangePasswordRequest true "Current and new password"
// @Success     200 {object} RevokedSessionsResponse "Password changed"
// @Failure     400 {object} ErrorResponse "Invalid input, or INVALID_CURRENT_PASSWORD"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     423 {object} ErrorResponse "ACCOUNT_LOCKED"
// @Failure     429 {object} ErrorResponse "Too many attempts"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/password [post]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	revoked, err := h.userService.ChangePassword(userID, req.CurrentPassword, req.NewPassword, h.currentSessionID(c, userID))
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidCurrentPassword) || errors.Is(err, apperrors.ErrAccountLocked) {
			reason := "invalid_current_password"
			if errors.Is(err, apperrors.ErrAccountLocked) {
				reason = "account_locked"
			}
			h.auditService.Log(userID, "CHANGE_PASSWORD_FAILED", "user", userID, c.ClientIP(),
				map[string]interface{}{"reason": reason})
		}
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CHANGE_PASSWORD", "user", userID, c.ClientIP(),
		map[string]interface{}{"revoked_sessions": revoked})
	c.JSON(http.StatusOK, RevokedSessionsResponse{RevokedSessions: revoked})
}

// ListSessions lists the user's signed-in devices.
// @Summary     List device sessions
// @Description List the authenticated user's live refresh sessions; current marks the one whose refresh cookie the request carries
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string][]SessionResponse "sessions"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/sessions [get]
func (h *AuthHandler) ListSessions(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	sessions, err := h.userService.ListRefreshSessions(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	currentID := h.currentSessionID(c, userID)
	out := make([]SessionResponse, 0, len(sessions))
	for i := range sessions {
		session := &sessions[i]
		lastSeen := session.CreatedAt
		if session.RotatedAt != nil {
			lastSeen = *session.RotatedAt
		}
		out = append(out, SessionResponse{
			ID:         session.ID,
			CreatedAt:  session.CreatedAt,
			LastSeenAt: lastSeen,
			UserAgent:  session.UserAgent,
			IPAddress:  session.IPAddress,
			Current:    currentID != "" && session.ID == currentID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

// RevokeSession ends one other device's session.
// @Summary     Revoke a device session
// @Description End another device's refresh session. The current session cannot be revoked here; use logout.
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Session ID"
// @Success     204 "Session revoked"
// @Failure     400 {object} ErrorResponse "CURRENT_SESSION or invalid ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "SESSION_NOT_FOUND"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/sessions/{id} [delete]
func (h *AuthHandler) RevokeSession(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	sessionID := c.Param("id")
	if !uuid.IsValid(sessionID) {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid session ID format"))
		return
	}

	if err := h.userService.RevokeRefreshSessionByID(userID, sessionID, h.currentSessionID(c, userID)); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "REVOKE_SESSION", "refresh_session", sessionID, c.ClientIP(), nil)
	c.Status(http.StatusNoContent)
}

// RevokeOtherSessions ends every device session except the current one.
// @Summary     Revoke all other device sessions
// @Description End every refresh session of the user except the one making the request
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} RevokedSessionsResponse "Other sessions revoked"
// @Failure     400 {object} ErrorResponse "Current session unknown (no refresh cookie sent)"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/sessions/revoke-others [post]
func (h *AuthHandler) RevokeOtherSessions(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	revoked, err := h.userService.RevokeOtherRefreshSessions(userID, h.currentSessionID(c, userID))
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "REVOKE_OTHER_SESSIONS", "user", userID, c.ClientIP(),
		map[string]interface{}{"revoked_sessions": revoked})
	c.JSON(http.StatusOK, RevokedSessionsResponse{RevokedSessions: revoked})
}

// currentSessionID resolves the caller's device session from the
// kuberan_refresh cookie (the browser sends it to every /auth route): the
// session whose current or just-rotated-out token hash matches. It returns ""
// when there is no cookie or no live session matches, e.g. for bot tokens and
// non-browser clients.
func (h *AuthHandler) currentSessionID(c *gin.Context, userID string) string {
	token, err := c.Cookie(RefreshCookieName)
	if err != nil || token == "" {
		return ""
	}
	id, err := h.userService.FindRefreshSessionID(userID, middleware.HashToken(token))
	if err != nil {
		return ""
	}
	return id
}
