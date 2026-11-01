package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
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
}

// AuthHandlerOption configures an AuthHandler.
type AuthHandlerOption func(*AuthHandler)

// WithSecureCookie sets whether the refresh cookie carries the Secure flag
// (true by default; config COOKIE_SECURE turns it off for plain-HTTP dev).
func WithSecureCookie(secure bool) AuthHandlerOption {
	return func(h *AuthHandler) { h.secureCookie = secure }
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userService services.UserServicer, auditService services.AuditServicer, opts ...AuthHandlerOption) *AuthHandler {
	h := &AuthHandler{userService: userService, auditService: auditService, secureCookie: true}
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
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	user, err := h.userService.CreateUser(req.Email, req.Password, req.FirstName, req.LastName)
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
		"user":         userPayload(user),
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
		"user":         userPayload(user),
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
		"user":         userPayload(user),
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
		"user": userPayload(user),
	})
}

// userPayload is the user object embedded in auth and profile responses.
func userPayload(user *models.User) gin.H {
	return gin.H{
		"id":                 user.ID,
		"email":              user.Email,
		"first_name":         user.FirstName,
		"last_name":          user.LastName,
		"hide_balances":      user.HideBalances,
		"default_account_id": user.DefaultAccountID,
	}
}

// UpdateProfileSettingsRequest represents the payload for updating profile settings.
// Every field is optional, but at least one must be present.
type UpdateProfileSettingsRequest struct {
	HideBalances *bool `json:"hide_balances"`
	// DefaultAccountID sets the account preselected for new transactions;
	// null or "" clears it. Must be a UUID otherwise.
	DefaultAccountID optionalString `json:"default_account_id" swaggertype:"string"`
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
// @Description Update the authenticated user's profile settings (dashboard balance visibility, default transaction account)
// @Tags        user
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body UpdateProfileSettingsRequest true "Profile settings to update"
// @Success     200 {object} UserResponse "Updated user profile"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
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

	if req.HideBalances == nil && !req.DefaultAccountID.Set {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput,
			"at least one of hide_balances or default_account_id is required"))
		return
	}

	update := services.ProfileSettingsUpdate{
		HideBalances:      req.HideBalances,
		SetDefaultAccount: req.DefaultAccountID.Set,
	}
	if req.DefaultAccountID.Set {
		update.DefaultAccountID, err = optionalUUID(req.DefaultAccountID.Value, "default_account_id")
		if err != nil {
			respondWithError(c, err)
			return
		}
	}

	if err := h.userService.UpdateProfileSettings(userID, update); err != nil {
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
	h.auditService.Log(userID, "UPDATE_PROFILE", "user", userID, c.ClientIP(), changes)

	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": userPayload(user),
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
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
