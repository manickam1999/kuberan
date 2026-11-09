package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

func setupAppearanceRouter(h *AuthHandler) *gin.Engine {
	r := setupAuthRouter(h)
	r.GET("/appearance", h.GetAppearance) // public: no auth middleware
	return r
}

func TestAuthHandler_GetAppearance_IsPublicAndMinimal(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []AuthHandlerOption
		want string
	}{
		{"built-in default", nil, "catppuccin"},
		{"configured default", []AuthHandlerOption{WithDefaultPalette("nord")}, "nord"},
		{"unknown configured default is ignored", []AuthHandlerOption{WithDefaultPalette("neon")}, "catppuccin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No injectUserID and no token: the route must answer anyway.
			r := setupAppearanceRouter(NewAuthHandler(&mockUserService{}, &mockAuditService{}, tc.opts...))

			rec := doRequest(r, "GET", "/appearance", "")

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 without auth, got %d: %s", rec.Code, rec.Body.String())
			}
			body := parseJSON(t, rec)
			if len(body) != 1 || body["default_palette"] != tc.want {
				t.Errorf("expected only default_palette=%s, got %v", tc.want, body)
			}
		})
	}
}

func TestAuthHandler_GetProfile_ResolvedPalette(t *testing.T) {
	yes := true
	for _, tc := range []struct {
		name       string
		appearance models.Appearance
		opts       []AuthHandlerOption
		want       string
	}{
		{"user choice wins", models.Appearance{Palette: "gruvbox"}, []AuthHandlerOption{WithDefaultPalette("nord")}, "gruvbox"},
		{"user default follows the server", models.Appearance{Palette: "default"}, []AuthHandlerOption{WithDefaultPalette("nord")}, "nord"},
		{"missing key follows the server", models.Appearance{Mode: "dark", ShowCents: &yes}, []AuthHandlerOption{WithDefaultPalette("lagoon")}, "lagoon"},
		{"no server default falls back to catppuccin", models.Appearance{}, nil, "catppuccin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userSvc := &mockUserService{getUserByIDFn: func(id string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: id}, Appearance: tc.appearance}, nil
			}}
			r := setupAppearanceRouter(NewAuthHandler(userSvc, &mockAuditService{}, tc.opts...))

			rec := doRequest(r, "GET", "/profile", "")

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			user := parseJSON(t, rec)["user"].(map[string]interface{})
			if user["resolved_palette"] != tc.want {
				t.Errorf("resolved_palette = %v, want %s", user["resolved_palette"], tc.want)
			}
			if _, ok := user["appearance"].(map[string]interface{}); !ok {
				t.Errorf("appearance must be a JSON object, got %v", user["appearance"])
			}
		})
	}
}

func TestAuthHandler_UpdateProfileSettings_Appearance(t *testing.T) {
	var calls []services.ProfileSettingsUpdate
	newRouter := func() *gin.Engine {
		calls = nil
		userSvc := &mockUserService{updateProfileSettingsFn: func(_ string, u services.ProfileSettingsUpdate) error {
			calls = append(calls, u)
			return nil
		}}
		return setupAppearanceRouter(NewAuthHandler(userSvc, &mockAuditService{}))
	}

	t.Run("a valid partial update reaches the service", func(t *testing.T) {
		r := newRouter()
		rec := doRequest(r, "PATCH", "/profile", `{"appearance":{"palette":"nord","show_cents":false}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(calls) != 1 || calls[0].Appearance == nil {
			t.Fatalf("expected one update with appearance, got %+v", calls)
		}
		a := calls[0].Appearance
		if a.Palette == nil || *a.Palette != "nord" || a.ShowCents == nil || *a.ShowCents || a.Mode != nil {
			t.Errorf("unexpected patch %+v", a)
		}
	})

	t.Run("palette default is accepted", func(t *testing.T) {
		r := newRouter()
		rec := doRequest(r, "PATCH", "/profile", `{"appearance":{"palette":"default"}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	for name, body := range map[string]string{
		"unknown palette":     `{"appearance":{"palette":"neon"}}`,
		"unknown mode":        `{"appearance":{"mode":"sepia"}}`,
		"unknown chart_style": `{"appearance":{"chart_style":"fancy"}}`,
		"unknown density":     `{"appearance":{"density":"tiny"}}`,
		"unknown key":         `{"appearance":{"theme":"dark"}}`,
		"wrong type":          `{"appearance":{"show_cents":"yes"}}`,
		"empty appearance":    `{"appearance":{}}`,
		"null appearance":     `{"appearance":null}`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			r := newRouter()
			rec := doRequest(r, "PATCH", "/profile", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
			if len(calls) != 0 {
				t.Errorf("the service must not be called, got %+v", calls)
			}
		})
	}
}
