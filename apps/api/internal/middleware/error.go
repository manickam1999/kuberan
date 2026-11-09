package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
)

// ErrorHandler returns a Gin middleware that converts errors set on the Gin
// context into consistent JSON error responses. AppErrors are returned with
// their code and message; unexpected errors are logged and return a generic
// internal error to avoid leaking details.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Process the last error (most relevant in a middleware chain)
		err := c.Errors.Last().Err

		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			if appErr.Internal != nil {
				logger.Get().Errorw("app error",
					"code", appErr.Code,
					"message", appErr.Message,
					"internal", appErr.Internal.Error(),
					"path", c.Request.URL.Path,
				)
			}
			c.JSON(appErr.StatusCode, gin.H{"error": appErr.Body()})
			return
		}

		// Unexpected error: log full details, return generic message
		logger.Get().Errorw("unexpected error",
			"error", err.Error(),
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
		)
		c.JSON(apperrors.ErrInternalServer.StatusCode, gin.H{
			"error": gin.H{
				"code":    apperrors.ErrInternalServer.Code,
				"message": apperrors.ErrInternalServer.Message,
			},
		})
	}
}
