package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSecurities_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/securities" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"securities": []map[string]any{
				{"id": "sec-1", "symbol": "AAPL", "name": "Apple Inc.", "asset_type": "stock", "currency": "USD", "exchange": "NASDAQ", "network": "", "provider_symbol": ""},
				{"id": "sec-2", "symbol": "BTC", "name": "Bitcoin", "asset_type": "crypto", "currency": "USD", "exchange": "", "network": "bitcoin", "provider_symbol": ""},
				{"id": "sec-3", "symbol": "CIMB", "name": "CIMB Group", "asset_type": "stock", "currency": "MYR", "exchange": "BURSA", "network": "", "provider_symbol": "1023.KL"},
			},
		})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	securities, err := c.GetSecurities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(securities) != 3 {
		t.Fatalf("expected 3 securities, got %d", len(securities))
	}

	if securities[0].ID != "sec-1" || securities[0].Symbol != "AAPL" || securities[0].AssetType != "stock" {
		t.Errorf("first security mismatch: %+v", securities[0])
	}
	if securities[0].ProviderSymbol != "" {
		t.Errorf("first security: expected empty provider_symbol, got %q", securities[0].ProviderSymbol)
	}
	if securities[1].ID != "sec-2" || securities[1].Symbol != "BTC" || securities[1].Network != "bitcoin" {
		t.Errorf("second security mismatch: %+v", securities[1])
	}
	if securities[2].ID != "sec-3" || securities[2].Symbol != "CIMB" || securities[2].Exchange != "BURSA" {
		t.Errorf("third security mismatch: %+v", securities[2])
	}
	if securities[2].ProviderSymbol != "1023.KL" {
		t.Errorf("third security: expected provider_symbol '1023.KL', got %q", securities[2].ProviderSymbol)
	}
}

func TestGetSecurities_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "bad-key", server.Client())
	_, err := c.GetSecurities(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "unexpected status 401"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestGetSecurities_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	_, err := c.GetSecurities(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "unexpected status 500"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestRecordPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/securities/prices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"prices_recorded": 5})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	prices := []RecordPriceEntry{
		{SecurityID: "sec-1", Price: 17872, RecordedAt: "2025-01-15T10:00:00Z"},
		{SecurityID: "sec-2", Price: 6723456, RecordedAt: "2025-01-15T10:00:00Z"},
		{SecurityID: "sec-3", Price: 10050, RecordedAt: "2025-01-15T10:00:00Z"},
		{SecurityID: "sec-4", Price: 500, RecordedAt: "2025-01-15T10:00:00Z"},
		{SecurityID: "sec-5", Price: 25000, RecordedAt: "2025-01-15T10:00:00Z"},
	}

	n, err := c.RecordPrices(context.Background(), prices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 prices recorded, got %d", n)
	}
}

func TestRecordPrices_ValidatesRequestBody(t *testing.T) {
	var capturedBody []byte
	var capturedAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("X-API-Key")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"prices_recorded": 2})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "my-secret-key", server.Client())
	prices := []RecordPriceEntry{
		{SecurityID: "sec-1", Price: 17872, RecordedAt: "2025-01-15T10:00:00Z"},
		{SecurityID: "sec-2", Price: 6723456, RecordedAt: "2025-01-15T10:00:00Z"},
	}

	_, err := c.RecordPrices(context.Background(), prices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify API key was sent.
	if capturedAPIKey != "my-secret-key" {
		t.Errorf("expected API key 'my-secret-key', got %q", capturedAPIKey)
	}

	// Verify JSON structure.
	var parsed struct {
		Prices []struct {
			SecurityID string `json:"security_id"`
			Price      int64  `json:"price"`
			RecordedAt string `json:"recorded_at"`
		} `json:"prices"`
	}
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("parsing captured body: %v", err)
	}
	if len(parsed.Prices) != 2 {
		t.Fatalf("expected 2 prices in body, got %d", len(parsed.Prices))
	}
	if parsed.Prices[0].SecurityID != "sec-1" || parsed.Prices[0].Price != 17872 {
		t.Errorf("first price mismatch: %+v", parsed.Prices[0])
	}
	if parsed.Prices[1].SecurityID != "sec-2" || parsed.Prices[1].Price != 6723456 {
		t.Errorf("second price mismatch: %+v", parsed.Prices[1])
	}
}

func TestComputeSnapshots_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/snapshots" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify body has recorded_at field.
		var body struct {
			RecordedAt string `json:"recorded_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body.RecordedAt == "" {
			t.Error("expected recorded_at in body")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"snapshots_recorded": 3})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	n, err := c.ComputeSnapshots(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 snapshots recorded, got %d", n)
	}
}

func TestProcessFixedDeposits_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/fixed-deposits/process" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify body has as_of field.
		var body struct {
			AsOf string `json:"as_of"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body.AsOf == "" {
			t.Error("expected as_of in body")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{
			"payouts_posted": 5,
			"fds_matured":    2,
		})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	result, err := c.ProcessFixedDeposits(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PayoutsPosted != 5 {
		t.Errorf("expected 5 payouts posted, got %d", result.PayoutsPosted)
	}
	if result.FDsMatured != 2 {
		t.Errorf("expected 2 fds matured, got %d", result.FDsMatured)
	}
}

func TestProcessFixedDeposits_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "bad-key", server.Client())
	_, err := c.ProcessFixedDeposits(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "unexpected status 401"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
