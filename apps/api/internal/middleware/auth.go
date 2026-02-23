package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"kuberan/internal/config"
	"kuberan/internal/models"
)

const (
	accessTokenExpiry  = 15 * time.Minute
	refreshTokenExpiry = 7 * 24 * time.Hour
	botTokenExpiry     = 365 * 24 * time.Hour // 1 year for bot tokens
)

// getJWTKey returns the JWT key from configuration
func getJWTKey() []byte {
	return []byte(config.Get().JWTSecret)
}

// JWTClaims represents the claims in the JWT
type JWTClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// GenerateAccessToken generates a short-lived JWT access token for a user.
func GenerateAccessToken(user *models.User) (string, error) {
	claims := &JWTClaims{
		UserID:    user.ID,
		Email:     user.Email,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "kuberan-api",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTKey())
}

// GenerateRefreshToken generates a long-lived JWT refresh token for a user.
func GenerateRefreshToken(user *models.User) (string, error) {
	claims := &JWTClaims{
		UserID:    user.ID,
		Email:     user.Email,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(refreshTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "kuberan-api",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTKey())
}

// GenerateBotToken generates a long-lived JWT token for bot service authentication.
func GenerateBotToken(user *models.User) (string, error) {
	claims := &JWTClaims{
		UserID:    user.ID,
		Email:     user.Email,
		TokenType: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(botTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "kuberan-api",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTKey())
}

// ValidateRefreshToken parses and validates a refresh token JWT.
// Returns the claims if valid, or an error if the token is invalid,
// expired, or not a refresh token.
func ValidateRefreshToken(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getJWTKey(), nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	if claims.TokenType != "refresh" {
		return nil, fmt.Errorf("token is not a refresh token")
	}

	return claims, nil
}

// HashToken returns the SHA-256 hex digest of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// AuthMiddleware verifies the JWT token and sets the user in the context
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		// Check if the header is in the correct format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		// Parse the token
		tokenString := parts[1]
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return getJWTKey(), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Reject refresh tokens used as access tokens
		if claims.TokenType == "refresh" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Set user ID and email in the context
		c.Set("userID", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}
