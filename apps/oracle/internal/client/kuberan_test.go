package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		_ = json.NewEncoder(w).Encode(map[string]any{"recorded": 5, "rejected": []any{}})
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

	res, err := c.RecordPrices(context.Background(), prices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Recorded != 5 {
		t.Errorf("expected 5 prices recorded, got %d", res.Recorded)
	}
	if len(res.Rejected) != 0 {
		t.Errorf("expected no rejected entries, got %+v", res.Rejected)
	}
}

func TestRecordPrices_LegacyCountKey(t *testing.T) {
	// An API that still answers with prices_recorded must not read as zero recorded.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"prices_recorded": 3})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	res, err := c.RecordPrices(context.Background(), []RecordPriceEntry{{SecurityID: "sec-1", Price: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Recorded != 3 {
		t.Errorf("Recorded = %d, want 3", res.Recorded)
	}
}

func TestRecordPrices_DecodesRejectedEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"recorded":1,"rejected":[{"index":1,"security_id":"sec-2","currency":"USD","expected_currency":"MYR","reason":"currency_mismatch"}]}`))
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	res, err := c.RecordPrices(context.Background(), []RecordPriceEntry{
		{SecurityID: "sec-1", Price: 1, Currency: "MYR"},
		{SecurityID: "sec-2", Price: 2, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Recorded != 1 {
		t.Errorf("Recorded = %d, want 1", res.Recorded)
	}
	want := RejectedPrice{Index: 1, SecurityID: "sec-2", Currency: "USD", ExpectedCurrency: "MYR", Reason: "currency_mismatch"}
	if len(res.Rejected) != 1 || res.Rejected[0] != want {
		t.Errorf("Rejected = %+v, want [%+v]", res.Rejected, want)
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
		_ = json.NewEncoder(w).Encode(map[string]any{"recorded": 2, "rejected": []any{}})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "my-secret-key", server.Client())
	prices := []RecordPriceEntry{
		{SecurityID: "sec-1", Price: 17872, RecordedAt: "2025-01-15T10:00:00Z", Currency: "USD"},
		{SecurityID: "sec-2", Price: 6723456, RecordedAt: "2025-01-15T10:00:00Z", Currency: "MYR"},
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
			Currency   string `json:"currency"`
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
	if parsed.Prices[0].Currency != "USD" || parsed.Prices[1].Currency != "MYR" {
		t.Errorf("currency not sent per entry: %+v", parsed.Prices)
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

// countingTransport records how many requests went through it.
type countingTransport struct {
	calls int
	next  http.RoundTripper
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls++
	return c.next.RoundTrip(r)
}

func TestLongRunningClient_UsedForSnapshotsAndFixedDeposits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/pipeline/snapshots":
			_ = json.NewEncoder(w).Encode(map[string]int{"snapshots_recorded": 1})
		case "/api/v1/pipeline/fixed-deposits/process":
			_ = json.NewEncoder(w).Encode(map[string]int{"payouts_posted": 0, "fds_matured": 0})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"securities": []any{}})
		}
	}))
	defer server.Close()

	short := &countingTransport{next: http.DefaultTransport}
	long := &countingTransport{next: http.DefaultTransport}
	c := NewKuberanClient(server.URL, "k", &http.Client{Transport: short}).
		WithLongRunningClient(&http.Client{Transport: long})

	if _, err := c.ComputeSnapshots(context.Background()); err != nil {
		t.Fatalf("ComputeSnapshots: %v", err)
	}
	if _, err := c.ProcessFixedDeposits(context.Background()); err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}
	if _, err := c.GetSecurities(context.Background()); err != nil {
		t.Fatalf("GetSecurities: %v", err)
	}

	if long.calls != 2 {
		t.Errorf("long client calls = %d, want 2", long.calls)
	}
	if short.calls != 1 {
		t.Errorf("short client calls = %d, want 1", short.calls)
	}
}

func TestGetFXPairs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/fx-pairs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing or wrong API key header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pairs":           []map[string]string{{"base": "USD", "quote": "MYR"}, {"base": "MYR", "quote": "USD"}},
			"base_currencies": []string{"MYR"},
		})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	fx, err := c.GetFXPairs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fx.Pairs) != 2 || fx.Pairs[0] != (CurrencyPair{Base: "USD", Quote: "MYR"}) {
		t.Errorf("pairs mismatch: %+v", fx.Pairs)
	}
	if len(fx.BaseCurrencies) != 1 || fx.BaseCurrencies[0] != "MYR" {
		t.Errorf("base currencies mismatch: %+v", fx.BaseCurrencies)
	}
}

func TestGetFXPairs_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	_, err := c.GetFXPairs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "unexpected status 500"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestRecordFXRates_ValidatesRequestBody(t *testing.T) {
	var capturedBody []byte
	var capturedAPIKey, capturedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/pipeline/fx-rates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedAPIKey = r.Header.Get("X-API-Key")
		capturedContentType = r.Header.Get("Content-Type")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"recorded": 1})
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "my-secret-key", server.Client())
	n, err := c.RecordFXRates(context.Background(), []FXRateEntry{
		{Base: "USD", Quote: "MYR", Rate: "4.4712000000", RecordedAt: "2026-11-03T08:00:00Z", Source: "yahoo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("recorded = %d, want 1", n)
	}
	if capturedAPIKey != "my-secret-key" {
		t.Errorf("API key = %q, want %q", capturedAPIKey, "my-secret-key")
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}

	// The rate must travel as a JSON string, not a number.
	var parsed struct {
		Rates []map[string]any `json:"rates"`
	}
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("parsing captured body: %v", err)
	}
	if len(parsed.Rates) != 1 {
		t.Fatalf("expected 1 rate in body, got %d", len(parsed.Rates))
	}
	want := map[string]any{
		"base":        "USD",
		"quote":       "MYR",
		"rate":        "4.4712000000",
		"recorded_at": "2026-11-03T08:00:00Z",
		"source":      "yahoo",
	}
	for k, v := range want {
		if parsed.Rates[0][k] != v {
			t.Errorf("rate field %q = %#v, want %#v", k, parsed.Rates[0][k], v)
		}
	}
}

func TestRecordFXRates_BatchesOf5000(t *testing.T) {
	var batchSizes []int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Rates []FXRateEntry `json:"rates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		batchSizes = append(batchSizes, len(body.Rates))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"recorded": len(body.Rates)})
	}))
	defer server.Close()

	entries := make([]FXRateEntry, 5001)
	for i := range entries {
		entries[i] = FXRateEntry{Base: "USD", Quote: "MYR", Rate: "4.4712000000", RecordedAt: "2026-11-03T08:00:00Z", Source: "yahoo"}
	}

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	n, err := c.RecordFXRates(context.Background(), entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5001 {
		t.Errorf("recorded = %d, want 5001", n)
	}
	if len(batchSizes) != 2 || batchSizes[0] != 5000 || batchSizes[1] != 1 {
		t.Errorf("batch sizes = %v, want [5000 1]", batchSizes)
	}
}

func TestRecordFXRates_LaterBatchFailureKeepsEarlierCount(t *testing.T) {
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body struct {
			Rates []FXRateEntry `json:"rates"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"recorded": len(body.Rates)})
	}))
	defer server.Close()

	entries := make([]FXRateEntry, 10001)
	c := NewKuberanClient(server.URL, "test-key", server.Client())
	n, err := c.RecordFXRates(context.Background(), entries)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "unexpected status 400"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
	if n != 5000 {
		t.Errorf("recorded = %d, want 5000 (first batch only)", n)
	}
	if calls != 2 {
		t.Errorf("requests = %d, want 2 (third batch must not be sent)", calls)
	}
}

func TestUpdateSecurity_SendsBodyAndDecodes(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/pipeline/securities/sec-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("missing API key")
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"security":{"id":"sec-1","currency":"USD"},"prices_deleted":42}`))
	}))
	defer server.Close()

	c := NewKuberanClient(server.URL, "test-key", server.Client())
	cur := "USD"
	res, err := c.UpdateSecurity(context.Background(), "sec-1", UpdateSecurityRequest{Currency: &cur, ResetPrices: true})
	if err != nil {
		t.Fatalf("UpdateSecurity: %v", err)
	}
	if res.PricesDeleted != 42 || res.Security.Currency != "USD" {
		t.Errorf("result = %+v", res)
	}
	if len(body) != 2 || body["currency"] != "USD" || body["reset_prices"] != true {
		t.Errorf("body = %v, want only currency and reset_prices", body)
	}
}

func TestUpdateSecurity_ConflictCarriesStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"SECURITY_CURRENCY_LOCKED"}`))
	}))
	defer server.Close()
	c := NewKuberanClient(server.URL, "test-key", server.Client())
	cur := "USD"
	_, err := c.UpdateSecurity(context.Background(), "sec-1", UpdateSecurityRequest{Currency: &cur})
	if err == nil || !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "SECURITY_CURRENCY_LOCKED") {
		t.Errorf("err = %v", err)
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Error("error leaks the API key")
	}
}
