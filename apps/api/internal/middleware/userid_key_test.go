package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserIDKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := UserIDKey(c); got != "" {
		t.Errorf("no user: key = %q, want empty", got)
	}
	c.Set("userID", "u1")
	if got := UserIDKey(c); got != "user:u1" {
		t.Errorf("key = %q, want user:u1", got)
	}
}
