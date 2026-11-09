package integration

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
)

// exportRequest GETs /api/v1/export. It advertises gzip like a browser does, to
// prove the zip is not compressed a second time.
func (app *testApp) exportRequest(token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/v1/export", http.NoBody)
	req.Header.Set("Accept-Encoding", "gzip")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

func unzipCSVs(t *testing.T, body []byte) map[string][][]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a zip: %v", err)
	}
	out := map[string][][]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		recs, err := csv.NewReader(rc).ReadAll()
		_ = rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		out[f.Name] = recs
	}
	return out
}

func TestExportFlow_ZipScopingRateLimitAndAudit(t *testing.T) {
	app := setupApp(t)
	alice, _, aliceID := app.registerUser(t, "alice-export@test.com", "password123")
	bob, _, _ := app.registerUser(t, "bob-export@test.com", "password123")

	if rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Alice Cash","initial_balance":250000,"currency":"MYR"}`, alice); rec.Code != http.StatusCreated {
		t.Fatalf("alice account: %d %s", rec.Code, rec.Body.String())
	}
	if rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Bob Cash","initial_balance":100,"currency":"USD"}`, bob); rec.Code != http.StatusCreated {
		t.Fatalf("bob account: %d %s", rec.Code, rec.Body.String())
	}

	// Unauthenticated callers are refused.
	if rec := app.exportRequest(""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}

	rec := app.exportRequest(alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("zip must not be gzip-encoded, got Content-Encoding %q", ce)
	}
	wantName := "kuberan-export-" + time.Now().UTC().Format("2006-01-02") + ".zip"
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, wantName) {
		t.Errorf("Content-Disposition = %q, want file %s", cd, wantName)
	}
	files := unzipCSVs(t, rec.Body.Bytes())
	accounts := files["accounts.csv"]
	if len(accounts) < 2 {
		t.Fatalf("accounts.csv has no data rows: %v", accounts)
	}
	var sawAlice bool
	for _, row := range accounts[1:] {
		for _, cell := range row {
			if cell == "Bob Cash" {
				t.Errorf("alice's export contains bob's account: %v", row)
			}
			if cell == "Alice Cash" {
				sawAlice = true
			}
		}
	}
	if !sawAlice {
		t.Error("alice's export is missing her account")
	}
	if got := len(files); got != 10 {
		t.Errorf("expected 10 CSV files, got %d", got)
	}

	// Rate limit: a second export in the same minute is refused with Retry-After.
	rec = app.exportRequest(alice)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on the second export, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 should carry Retry-After")
	}
	// ... but the budget is per user.
	if rec := app.exportRequest(bob); rec.Code != http.StatusOK {
		t.Fatalf("bob's first export should succeed, got %d", rec.Code)
	}

	// Audited, once per successful export of alice.
	var n int64
	app.DB.Model(&models.AuditLog{}).Where("action = ? AND user_id = ?", "EXPORT_DATA", aliceID).Count(&n)
	if n != 1 {
		t.Errorf("expected 1 EXPORT_DATA audit entry for alice, got %d", n)
	}
}

func TestSystemStatusFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "status-flow@test.com", "password123")

	if rec := app.request("GET", "/api/v1/system/status", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	rec := app.request("GET", "/api/v1/system/status", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	st := parseJSON(t, rec)
	for _, k := range []string{"latest_price_at", "latest_fx_rate_at", "latest_snapshot_at", "last_backup_at"} {
		v, ok := st[k]
		if !ok || v != nil {
			t.Errorf("%s should be present and null on a fresh install, got %v (present=%v)", k, v, ok)
		}
	}

	finished := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	runs := []models.BackupRun{
		{StartedAt: finished.Add(-time.Minute), FinishedAt: finished, Status: "ok"},
		{StartedAt: finished, FinishedAt: finished.Add(30 * time.Minute), Status: "failed", Error: "pg_dump failed"},
	}
	for i := range runs {
		if err := app.DB.Create(&runs[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	st = parseJSON(t, app.request("GET", "/api/v1/system/status", "", token))
	got, ok := st["last_backup_at"].(string)
	if !ok {
		t.Fatalf("last_backup_at = %v, want RFC3339 string", st["last_backup_at"])
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil || !parsed.Equal(finished) {
		t.Errorf("last_backup_at = %q, want %s (latest ok run)", got, finished.Format(time.RFC3339))
	}
}
