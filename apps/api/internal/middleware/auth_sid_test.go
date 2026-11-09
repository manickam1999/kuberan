package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"kuberan/internal/models"
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

func TestAuthMiddleware_SessionIDClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &models.User{Base: models.Base{ID: "user-1"}, Email: "a@example.com"}

	run := func(token string) string {
		r := gin.New()
		var sid interface{}
		r.GET("/x", AuthMiddleware(), func(c *gin.Context) {
			sid, _ = c.Get("sessionID")
			c.Status(http.StatusNoContent)
		})
		req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		s, _ := sid.(string)
		return s
	}

	withSID, err := GenerateAccessTokenForSession(user, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := run(withSID); got != "session-1" {
		t.Errorf("sid = %q, want session-1", got)
	}

	plain, err := GenerateAccessToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if got := run(plain); got != "" {
		t.Errorf("sid = %q, want empty for a token without the claim", got)
	}
}
