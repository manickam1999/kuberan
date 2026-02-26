package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PipelineAuthMiddleware creates a Gin middleware that validates the X-API-Key
// header against the configured pipeline API key.
func PipelineAuthMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": gin.H{"code": "PIPELINE_NOT_CONFIGURED", "message": "Pipeline endpoints are not configured"}})
			return
		}
		key := c.GetHeader("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": gin.H{"code": "INVALID_API_KEY", "message": "Invalid or missing API key"}})
			return
		}
		c.Next()
	}
}

// InternalAuthMiddleware creates a Gin middleware that validates the X-Internal-Secret
// header against the configured internal secret (for bot service communication).
func InternalAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": gin.H{"code": "INTERNAL_AUTH_NOT_CONFIGURED", "message": "Internal authentication is not configured"}})
			return
		}
		providedSecret := c.GetHeader("X-Internal-Secret")
		if subtle.ConstantTimeCompare([]byte(providedSecret), []byte(secret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": gin.H{"code": "INVALID_INTERNAL_SECRET", "message": "Invalid or missing internal secret"}})
			return
		}
		c.Next()
	}
}
