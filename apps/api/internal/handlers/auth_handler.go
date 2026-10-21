package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	userService  services.UserServicer
	auditService services.AuditServicer
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userService services.UserServicer, auditService services.AuditServicer) *AuthHandler {
	return &AuthHandler{userService: userService, auditService: auditService}
}

// RegisterRequest represents the registration request payload
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email,max=255"`
	Password  string `json:"password" binding:"required,min=8,max=128"`
	FirstName string `json:"first_name" binding:"max=100"`
	LastName  string `json:"last_name" binding:"max=100"`
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest represents the token refresh request payload.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UserResponse represents the user data in the response
type UserResponse struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// AuthResponse represents the authentication response with tokens.
type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         UserResponse `json:"user"`
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

	accessToken, refreshToken, err := h.generateTokenPair(user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(user.ID, "REGISTER", "user", user.ID, c.ClientIP(), nil)

	c.JSON(http.StatusCreated, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user":          userPayload(user),
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
// @Failure     401 {object} ErrorResponse "Invalid credentials"
// @Failure     423 {object} ErrorResponse "Account locked"
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
		h.auditService.Log("", "LOGIN_FAILED", "user", "", c.ClientIP(),
			map[string]interface{}{"email": req.Email})
		respondWithError(c, err)
		return
	}

	accessToken, refreshToken, err := h.generateTokenPair(user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(user.ID, "LOGIN", "user", user.ID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user":          userPayload(user),
	})
}

// RefreshToken exchanges a valid refresh token for a new token pair.
// @Summary     Refresh access token
// @Description Exchange a valid refresh token for new access and refresh tokens
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       request body RefreshRequest true "Refresh token"
// @Success     200 {object} AuthResponse "New tokens generated"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid or expired refresh token"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	// Validate the refresh token JWT
	claims, err := middleware.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		respondWithError(c, apperrors.ErrUnauthorized)
		return
	}

	// Verify the token hash matches what's stored in the DB
	storedHash, err := h.userService.GetRefreshTokenHash(claims.UserID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	incomingHash := middleware.HashToken(req.RefreshToken)
	if storedHash == "" || storedHash != incomingHash {
		respondWithError(c, apperrors.ErrUnauthorized)
		return
	}

	// Get the user to generate new tokens
	user, err := h.userService.GetUserByID(claims.UserID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	// Generate new token pair (rotation)
	accessToken, refreshToken, err := h.generateTokenPair(user)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user":          userPayload(user),
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
	// null or "" clears it.
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

	if req.DefaultAccountID.Set {
		accountID := req.DefaultAccountID.Value
		if accountID != nil && *accountID == "" {
			accountID = nil
		}
		if err := h.userService.UpdateDefaultAccount(userID, accountID); err != nil {
			respondWithError(c, err)
			return
		}
	}

	if req.HideBalances != nil {
		if err := h.userService.UpdateHideBalances(userID, *req.HideBalances); err != nil {
			respondWithError(c, err)
			return
		}
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

// generateTokenPair creates a new access/refresh token pair and stores
// the refresh token hash in the database.
func (h *AuthHandler) generateTokenPair(user *models.User) (accessToken, refreshToken string, err error) {
	accessToken, err = middleware.GenerateAccessToken(user)
	if err != nil {
		return "", "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	refreshToken, err = middleware.GenerateRefreshToken(user)
	if err != nil {
		return "", "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Store refresh token hash for validation on refresh
	if err := h.userService.StoreRefreshTokenHash(user.ID, middleware.HashToken(refreshToken)); err != nil {
		return "", "", err
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
