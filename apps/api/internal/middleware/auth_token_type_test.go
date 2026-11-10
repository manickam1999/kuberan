package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"kuberan/internal/models"
)

// TestAuthMiddleware_ExposesTokenType checks that handlers can tell bot tokens
// from access tokens, including bot tokens minted before this feature (they
// have no extra claims, only token_type "bot").
func TestAuthMiddleware_ExposesTokenType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &models.User{Base: models.Base{ID: "u1"}, Email: "a@b.c"}

	// A bot token exactly as GenerateBotToken minted it a year ago.
	legacyBot, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTClaims{
		UserID: user.ID, Email: user.Email, TokenType: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "kuberan-api", Subject: user.ID,
		},
	}).SignedString(getJWTKey())
	if err != nil {
		t.Fatal(err)
	}
	bot, _ := GenerateBotToken(user)
	access, _ := GenerateAccessToken(user)

	cases := map[string]struct{ token, want string }{
		"legacy bot token": {legacyBot, TokenTypeBot},
		"new bot token":    {bot, TokenTypeBot},
		"access token":     {access, "access"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got any
			r := gin.New()
			r.GET("/x", AuthMiddleware(), func(c *gin.Context) {
				got, _ = c.Get(ContextKeyTokenType)
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest("GET", "/x", http.NoBody)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || got != tc.want {
				t.Fatalf("code=%d token_type=%v, want %s", rec.Code, got, tc.want)
			}
		})
	}
}
