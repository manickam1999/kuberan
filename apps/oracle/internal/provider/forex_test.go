package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newForexMockServer creates a test server that responds with exchange rates.
// rateMap maps forex ticker (e.g. "USDMYR=X") to the rate value.
func newForexMockServer(rateMap map[string]float64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path is /{ticker}, query has ?interval=1d&range=1d
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")

		rate, ok := rateMap[ticker]
		if !ok {
			_ = json.NewEncoder(w).Encode(v8ChartErrorResponse("Not Found", "No data found for "+ticker))
			return
		}
		_ = json.NewEncoder(w).Encode(v8ChartResponse(ticker, rate))
	}))
}

func TestForexConverter_NeedsConversion(t *testing.T) {
	fc := NewForexConverter(http.DefaultClient, "MYR")

	tests := []struct {
		currency string
		want     bool
	}{
		{"USD", true},
		{"usd", true},
		{"SGD", true},
		{"MYR", false},
		{"myr", false},
		{"Myr", false},
	}
	for _, tt := range tests {
		got := fc.NeedsConversion(tt.currency)
		if got != tt.want {
			t.Errorf("NeedsConversion(%q) = %v, want %v", tt.currency, got, tt.want)
		}
	}
}

func TestForexConverter_GetRate_SameCurrency(t *testing.T) {
	fc := NewForexConverter(http.DefaultClient, "MYR")

	rate, err := fc.GetRate(context.Background(), "MYR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 1.0 {
		t.Errorf("rate = %f, want 1.0", rate)
	}
}

func TestForexConverter_GetRate_Success(t *testing.T) {
	server := newForexMockServer(map[string]float64{
		"USDMYR=X": 4.47,
		"SGDMYR=X": 3.32,
	})
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	rate, err := fc.GetRate(context.Background(), "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 4.47 {
		t.Errorf("USD rate = %f, want 4.47", rate)
	}

	rate, err = fc.GetRate(context.Background(), "SGD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 3.32 {
		t.Errorf("SGD rate = %f, want 3.32", rate)
	}
}

func TestForexConverter_GetRate_Cached(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse("USDMYR=X", 4.47))
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	// First call should fetch.
	rate, err := fc.GetRate(context.Background(), "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 4.47 {
		t.Errorf("rate = %f, want 4.47", rate)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want 1", requestCount)
	}

	// Second call should use cache.
	rate, err = fc.GetRate(context.Background(), "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 4.47 {
		t.Errorf("rate = %f, want 4.47", rate)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d after second call, want 1 (should be cached)", requestCount)
	}
}

func TestForexConverter_GetRate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	_, err := fc.GetRate(context.Background(), "USD")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("expected error about status 500, got: %v", err)
	}
}

func TestForexConverter_GetRate_ChartError(t *testing.T) {
	server := newForexMockServer(map[string]float64{}) // Empty map → chart error
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	_, err := fc.GetRate(context.Background(), "XYZ")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "forex chart error") {
		t.Errorf("expected chart error, got: %v", err)
	}
}

func TestForexConverter_Convert_SameCurrency(t *testing.T) {
	fc := NewForexConverter(http.DefaultClient, "MYR")

	result, err := fc.Convert(context.Background(), 1000, "MYR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 1000 {
		t.Errorf("result = %d, want 1000", result)
	}
}

func TestForexConverter_Convert_Success(t *testing.T) {
	server := newForexMockServer(map[string]float64{
		"USDMYR=X": 4.47,
	})
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	// $100.00 (10000 cents) * 4.47 = RM447.00 (44700 cents)
	result, err := fc.Convert(context.Background(), 10000, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 44700 {
		t.Errorf("result = %d, want 44700", result)
	}
}

func TestForexConverter_Convert_Rounding(t *testing.T) {
	server := newForexMockServer(map[string]float64{
		"USDMYR=X": 4.4735,
	})
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	// $178.72 (17872 cents) * 4.4735 = RM799.50392 → 79950 cents (rounded)
	result, err := fc.Convert(context.Background(), 17872, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 17872 * 4.4735 = 79950.392 → rounds to 79950
	expected := int64(79950)
	if result != expected {
		t.Errorf("result = %d, want %d", result, expected)
	}
}

func TestForexConverter_Convert_CaseInsensitive(t *testing.T) {
	server := newForexMockServer(map[string]float64{
		"USDMYR=X": 4.47,
	})
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	result, err := fc.Convert(context.Background(), 10000, "usd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 44700 {
		t.Errorf("result = %d, want 44700", result)
	}
}

func TestForexConverter_TargetCurrency(t *testing.T) {
	fc := NewForexConverter(http.DefaultClient, "MYR")
	if fc.TargetCurrency() != "MYR" {
		t.Errorf("TargetCurrency() = %q, want %q", fc.TargetCurrency(), "MYR")
	}
}

func TestForexConverter_FetchRate_Pairs(t *testing.T) {
	requested := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		requested[ticker]++
		w.Header().Set("Content-Type", "application/json")
		rates := map[string]float64{"USDMYR=X": 4.47, "SGDUSD=X": 0.74, "EURUSD=X": 1.08}
		rate, ok := rates[ticker]
		if !ok {
			_ = json.NewEncoder(w).Encode(v8ChartErrorResponse("Not Found", "No data found for "+ticker))
			return
		}
		_ = json.NewEncoder(w).Encode(v8ChartResponse(ticker, rate))
	}))
	defer server.Close()

	tests := []struct {
		name        string
		base, quote string
		want        float64
		wantTicker  string
	}{
		{"target pair", "USD", "MYR", 4.47, "USDMYR=X"},
		{"non-target pair", "SGD", "USD", 0.74, "SGDUSD=X"},
		{"lowercase input normalised", "eur", "usd", 1.08, "EURUSD=X"},
		{"identical currencies need no request", "MYR", "myr", 1.0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &ForexConverter{
				httpClient:     server.Client(),
				baseURL:        server.URL,
				targetCurrency: "MYR",
				rates:          make(map[string]float64),
			}
			before := len(requested)
			got, err := fc.FetchRate(context.Background(), tt.base, tt.quote)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("FetchRate(%s, %s) = %v, want %v", tt.base, tt.quote, got, tt.want)
			}
			if tt.wantTicker == "" {
				if len(requested) != before {
					t.Errorf("identical pair triggered a request")
				}
				return
			}
			if requested[tt.wantTicker] == 0 {
				t.Errorf("ticker %s was not requested; requests: %v", tt.wantTicker, requested)
			}
		})
	}
}

func TestForexConverter_FetchRate_CacheSharedWithGetRateAndConvert(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse(strings.TrimPrefix(r.URL.Path, "/"), 4.47))
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient:     server.Client(),
		baseURL:        server.URL,
		targetCurrency: "MYR",
		rates:          make(map[string]float64),
	}

	// Publishing fetches the pair first; the later conversion must reuse it.
	if _, err := fc.FetchRate(context.Background(), "USD", "MYR"); err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if _, err := fc.GetRate(context.Background(), "USD"); err != nil {
		t.Fatalf("GetRate: %v", err)
	}
	if _, err := fc.Convert(context.Background(), 10000, "usd"); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want 1 (FetchRate, GetRate and Convert share one cache entry)", requestCount)
	}

	// A different pair is a different cache entry.
	if _, err := fc.FetchRate(context.Background(), "USD", "SGD"); err != nil {
		t.Fatalf("FetchRate USD/SGD: %v", err)
	}
	if requestCount != 2 {
		t.Errorf("requestCount = %d, want 2 after a new pair", requestCount)
	}
}
