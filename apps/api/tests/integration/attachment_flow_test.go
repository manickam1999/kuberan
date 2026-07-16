package integration

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

// makePNG builds a small valid PNG so the server-side sniff + re-encode
// pipeline accepts it as a genuine image rather than rejecting it as an
// unsupported type.
func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return buf.Bytes()
}

// createTxWithAccount registers no user; it assumes the caller registered and
// passes a token. It creates a cash account and a transaction, returning the
// transaction ID.
func (app *testApp) createTxWithAccount(t *testing.T, token string) string {
	t.Helper()
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Checking","currency":"USD","initial_balance":0}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account failed: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":4200,"description":"Lunch"}`, accountID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create transaction failed: %d %s", rec.Code, rec.Body.String())
	}
	return parseJSON(t, rec)["transaction"].(map[string]interface{})["id"].(string)
}

// TestAttachmentFlow_UploadListDownloadDelete exercises the full receipt
// attachment lifecycle over HTTP against the real router, service, and an
// in-memory blob store: upload → list → download (with hardened headers) →
// delete → verify gone.
func TestAttachmentFlow_UploadListDownloadDelete(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "receipts@test.com", "password123")
	txID := app.createTxWithAccount(t, token)

	// Upload a receipt image.
	rec := app.uploadFile(
		fmt.Sprintf("/api/v1/transactions/%s/attachments", txID),
		"receipt.png", "image/png", makePNG(t), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	att := parseJSON(t, rec)["attachment"].(map[string]interface{})
	attID := att["id"].(string)
	if att["content_type"] != "image/png" {
		t.Errorf("expected content_type image/png, got %v", att["content_type"])
	}
	// storage_key and checksum must never leak to the client (json:"-").
	if _, leaked := att["storage_key"]; leaked {
		t.Error("storage_key must not be exposed in the attachment JSON")
	}
	if _, leaked := att["checksum"]; leaked {
		t.Error("checksum must not be exposed in the attachment JSON")
	}

	// List returns exactly one attachment for the transaction.
	rec = app.request("GET", fmt.Sprintf("/api/v1/transactions/%s/attachments", txID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list failed: %d %s", rec.Code, rec.Body.String())
	}
	atts := parseJSON(t, rec)["attachments"].([]interface{})
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}

	// Download streams the bytes with the hardened serve headers.
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/transactions/%s/attachments/%s", txID, attID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("download failed: %d %s", rec.Code, rec.Body.String())
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("expected non-empty attachment body")
	}
	wantHeaders := map[string]string{
		"Content-Type":                 "image/png",
		"Content-Disposition":          "inline",
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      "default-src 'none'; sandbox",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cache-Control":                "private, no-store",
	}
	for k, want := range wantHeaders {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("download header %s = %q, want %q", k, got, want)
		}
	}

	// Delete removes the attachment.
	rec = app.request("DELETE",
		fmt.Sprintf("/api/v1/transactions/%s/attachments/%s", txID, attID), "", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete failed: %d %s", rec.Code, rec.Body.String())
	}

	// The list is now empty and the bytes are gone (404).
	rec = app.request("GET", fmt.Sprintf("/api/v1/transactions/%s/attachments", txID), "", token)
	if atts := parseJSON(t, rec)["attachments"].([]interface{}); len(atts) != 0 {
		t.Errorf("expected 0 attachments after delete, got %d", len(atts))
	}
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/transactions/%s/attachments/%s", txID, attID), "", token)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 downloading deleted attachment, got %d", rec.Code)
	}
}

// TestAttachmentFlow_RejectsUnsupportedType verifies the server sniffs the
// bytes and rejects a non-allowlisted payload (415) even when the multipart
// Content-Type claims an allowed image type.
func TestAttachmentFlow_RejectsUnsupportedType(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "badtype@test.com", "password123")
	txID := app.createTxWithAccount(t, token)

	// A plain-text payload lying about being a PNG.
	rec := app.uploadFile(
		fmt.Sprintf("/api/v1/transactions/%s/attachments", txID),
		"not-an-image.png", "image/png", []byte("this is definitely not an image"), token)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for unsupported bytes, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestAttachmentFlow_CrossUserIsolation verifies a second user cannot upload to
// or read another user's transaction attachments (ownership enforced).
func TestAttachmentFlow_CrossUserIsolation(t *testing.T) {
	app := setupApp(t)
	ownerToken, _, _ := app.registerUser(t, "owner@test.com", "password123")
	txID := app.createTxWithAccount(t, ownerToken)

	// Owner uploads a receipt.
	rec := app.uploadFile(
		fmt.Sprintf("/api/v1/transactions/%s/attachments", txID),
		"receipt.png", "image/png", makePNG(t), ownerToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner upload failed: %d %s", rec.Code, rec.Body.String())
	}
	attID := parseJSON(t, rec)["attachment"].(map[string]interface{})["id"].(string)

	attackerToken, _, _ := app.registerUser(t, "attacker@test.com", "password123")

	// Attacker cannot upload to the owner's transaction.
	rec = app.uploadFile(
		fmt.Sprintf("/api/v1/transactions/%s/attachments", txID),
		"evil.png", "image/png", makePNG(t), attackerToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for attacker upload to foreign tx, got %d %s", rec.Code, rec.Body.String())
	}

	// Attacker cannot download the owner's attachment.
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/transactions/%s/attachments/%s", txID, attID), "", attackerToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for attacker download of foreign attachment, got %d", rec.Code)
	}

	// Attacker cannot delete the owner's attachment.
	rec = app.request("DELETE",
		fmt.Sprintf("/api/v1/transactions/%s/attachments/%s", txID, attID), "", attackerToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for attacker delete of foreign attachment, got %d", rec.Code)
	}
}
