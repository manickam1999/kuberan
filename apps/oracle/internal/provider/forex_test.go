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

func TestForexConverter_FetchRate_SameCurrency(t *testing.T) {
	fc := NewForexConverter(http.DefaultClient)

	rate, err := fc.FetchRate(context.Background(), "MYR", "MYR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 1.0 {
		t.Errorf("rate = %f, want 1.0", rate)
	}
}

func TestForexConverter_FetchRate_Success(t *testing.T) {
	server := newForexMockServer(map[string]float64{
		"USDMYR=X": 4.47,
		"SGDMYR=X": 3.32,
	})
	defer server.Close()

	fc := &ForexConverter{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rates:      make(map[string]float64),
	}

	rate, err := fc.FetchRate(context.Background(), "USD", "MYR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 4.47 {
		t.Errorf("USD rate = %f, want 4.47", rate)
	}

	rate, err = fc.FetchRate(context.Background(), "SGD", "MYR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 3.32 {
		t.Errorf("SGD rate = %f, want 3.32", rate)
	}
}

func TestForexConverter_FetchRate_Cached(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse("USDMYR=X", 4.47))
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rates:      make(map[string]float64),
	}

	// First call should fetch.
	rate, err := fc.FetchRate(context.Background(), "USD", "MYR")
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
	rate, err = fc.FetchRate(context.Background(), "USD", "MYR")
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

func TestForexConverter_FetchRate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rates:      make(map[string]float64),
	}

	_, err := fc.FetchRate(context.Background(), "USD", "MYR")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("expected error about status 500, got: %v", err)
	}
}

func TestForexConverter_FetchRate_ChartError(t *testing.T) {
	server := newForexMockServer(map[string]float64{}) // Empty map → chart error
	defer server.Close()

	fc := &ForexConverter{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rates:      make(map[string]float64),
	}

	_, err := fc.FetchRate(context.Background(), "XYZ", "MYR")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "chart error Not Found") {
		t.Errorf("expected chart error, got: %v", err)
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
		{"usd to myr", "USD", "MYR", 4.47, "USDMYR=X"},
		{"sgd to usd", "SGD", "USD", 0.74, "SGDUSD=X"},
		{"lowercase input normalised", "eur", "usd", 1.08, "EURUSD=X"},
		{"identical currencies need no request", "MYR", "myr", 1.0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &ForexConverter{
				httpClient: server.Client(),
				baseURL:    server.URL,
				rates:      make(map[string]float64),
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

func TestForexConverter_FetchRate_CacheSharedAcrossCalls(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse(strings.TrimPrefix(r.URL.Path, "/"), 4.47))
	}))
	defer server.Close()

	fc := &ForexConverter{
		httpClient: server.Client(),
		baseURL:    server.URL,
		rates:      make(map[string]float64),
	}

	// Publishing the same pair twice must reuse the cached rate.
	if _, err := fc.FetchRate(context.Background(), "USD", "MYR"); err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if _, err := fc.FetchRate(context.Background(), "usd", "myr"); err != nil {
		t.Fatalf("FetchRate lower-case: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want 1 (case-variant requests share one cache entry)", requestCount)
	}

	// A different pair is a different cache entry.
	if _, err := fc.FetchRate(context.Background(), "USD", "SGD"); err != nil {
		t.Fatalf("FetchRate USD/SGD: %v", err)
	}
	if requestCount != 2 {
		t.Errorf("requestCount = %d, want 2 after a new pair", requestCount)
	}
}
