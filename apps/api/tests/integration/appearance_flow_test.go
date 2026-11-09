package integration

import (
	"net/http"
	"testing"
)

func profileUser(t *testing.T, app *testApp, token string) map[string]interface{} {
	t.Helper()
	rec := app.request("GET", "/api/v1/profile", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
	}
	return parseJSON(t, rec)["user"].(map[string]interface{})
}

func TestAppearanceFlow_PublicDefaultNeedsNoAuth(t *testing.T) {
	app := setupApp(t)

	rec := app.request("GET", "/api/v1/appearance", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without a token, got %d: %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if len(body) != 1 || body["default_palette"] != "catppuccin" {
		t.Errorf("expected exactly {default_palette: catppuccin}, got %v", body)
	}
}

func TestAppearanceFlow_ProfileMergeAndResolution(t *testing.T) {
	app := setupApp(t)
	access, _, _ := app.registerUser(t, "look@test.com", "password123")

	user := profileUser(t, app, access)
	if a, ok := user["appearance"].(map[string]interface{}); !ok || len(a) != 0 {
		t.Errorf("a new user has an empty appearance object, got %v", user["appearance"])
	}
	if user["resolved_palette"] != "catppuccin" {
		t.Errorf("resolved_palette = %v, want catppuccin", user["resolved_palette"])
	}

	// A partial update merges; unsent keys survive.
	rec := app.request("PATCH", "/api/v1/profile", `{"appearance":{"palette":"nord","mode":"light"}}`, access)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("PATCH", "/api/v1/profile", `{"appearance":{"mode":"dark","show_cents":false}}`, access)
	if rec.Code != http.StatusOK {
		t.Fatalf("second patch: %d %s", rec.Code, rec.Body.String())
	}
	user = profileUser(t, app, access)
	a := user["appearance"].(map[string]interface{})
	if a["palette"] != "nord" || a["mode"] != "dark" || a["show_cents"] != false {
		t.Errorf("merged appearance wrong: %v", a)
	}
	if user["resolved_palette"] != "nord" {
		t.Errorf("resolved_palette = %v, want nord", user["resolved_palette"])
	}

	// "default" hands the palette back to the server.
	rec = app.request("PATCH", "/api/v1/profile", `{"appearance":{"palette":"default"}}`, access)
	if rec.Code != http.StatusOK {
		t.Fatalf("default patch: %d %s", rec.Code, rec.Body.String())
	}
	user = profileUser(t, app, access)
	if user["resolved_palette"] != "catppuccin" {
		t.Errorf("resolved_palette = %v, want catppuccin", user["resolved_palette"])
	}
	if user["appearance"].(map[string]interface{})["mode"] != "dark" {
		t.Errorf("other keys must survive: %v", user["appearance"])
	}
}

func TestAppearanceFlow_RejectsUnknownKeysAndValues(t *testing.T) {
	app := setupApp(t)
	access, _, _ := app.registerUser(t, "bad@test.com", "password123")

	for _, body := range []string{
		`{"appearance":{"palette":"neon"}}`,
		`{"appearance":{"mode":"sepia"}}`,
		`{"appearance":{"chart_style":"fancy"}}`,
		`{"appearance":{"density":"tiny"}}`,
		`{"appearance":{"theme":"dark"}}`,
		`{"appearance":{}}`,
	} {
		rec := app.request("PATCH", "/api/v1/profile", body, access)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", body, rec.Code)
		}
	}
	if a := profileUser(t, app, access)["appearance"].(map[string]interface{}); len(a) != 0 {
		t.Errorf("nothing may be stored after rejections, got %v", a)
	}
}

func TestAppearanceFlow_AppearanceIsPerUser(t *testing.T) {
	app := setupApp(t)
	one, _, _ := app.registerUser(t, "one@test.com", "password123")
	two, _, _ := app.registerUser(t, "two@test.com", "password123")

	rec := app.request("PATCH", "/api/v1/profile", `{"appearance":{"palette":"gruvbox"}}`, one)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if got := profileUser(t, app, two)["resolved_palette"]; got != "catppuccin" {
		t.Errorf("another user's choice leaked: %v", got)
	}
}
