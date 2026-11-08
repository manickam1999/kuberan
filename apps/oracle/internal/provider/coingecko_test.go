package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoinGeckoProvider_Supports(t *testing.T) {
	p := NewCoinGeckoProvider(http.DefaultClient)

	if !p.Supports("crypto") {
		t.Error("expected Supports(crypto) = true")
	}

	unsupported := []string{"stock", "etf", "bond", "reit", ""}
	for _, at := range unsupported {
		if p.Supports(at) {
			t.Errorf("expected Supports(%q) = false", at)
		}
	}
}

func TestCoinGeckoProvider_FetchPrices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request uses myr as vs_currencies.
		if !strings.Contains(r.URL.RawQuery, "vs_currencies=myr") {
			t.Errorf("expected vs_currencies=myr in query, got %s", r.URL.RawQuery)
		}
		resp := map[string]map[string]float64{
			"bitcoin":  {"myr": 300539.12},
			"ethereum": {"myr": 15457.82},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "MYR"},
		{ID: "sec-2", Symbol: "ETH", AssetType: "crypto", Currency: "MYR"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	expected := map[string]int64{
		"sec-1": 30053912,
		"sec-2": 1545782,
	}
	for _, r := range results {
		want, ok := expected[r.SecurityID]
		if !ok {
			t.Errorf("unexpected security ID %s", r.SecurityID)
			continue
		}
		if r.Price != want {
			t.Errorf("security %s: got price %d, want %d", r.SecurityID, r.Price, want)
		}
		if r.Currency != "MYR" {
			t.Errorf("security %s: got currency %q, want %q", r.SecurityID, r.Currency, "MYR")
		}
	}
}

func TestCoinGeckoProvider_FetchPrices_UnknownSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not make HTTP request for unknown symbol only")
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "OBSCURECOIN", AssetType: "crypto", Currency: "USD"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(fetchErrors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(fetchErrors))
	}
	if fetchErrors[0].SecurityID != "sec-1" {
		t.Errorf("expected error for security ID sec-1, got %s", fetchErrors[0].SecurityID)
	}
	if !strings.Contains(fetchErrors[0].Err.Error(), "no CoinGecko mapping") {
		t.Errorf("expected mapping error, got: %v", fetchErrors[0].Err)
	}
}

func TestCoinGeckoProvider_FetchPrices_PartialResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return price for bitcoin but not ethereum.
		resp := map[string]map[string]float64{
			"bitcoin": {"myr": 300539.12},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "MYR"},
		{ID: "sec-2", Symbol: "ETH", AssetType: "crypto", Currency: "MYR"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(fetchErrors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(fetchErrors))
	}
	if results[0].SecurityID != "sec-1" {
		t.Errorf("expected result for security ID sec-1, got %s", results[0].SecurityID)
	}
	if results[0].Currency != "MYR" {
		t.Errorf("expected currency MYR, got %q", results[0].Currency)
	}
	if fetchErrors[0].SecurityID != "sec-2" {
		t.Errorf("expected error for security ID sec-2, got %s", fetchErrors[0].SecurityID)
	}
}

func TestCoinGeckoProvider_FetchPrices_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "MYR"},
		{ID: "sec-2", Symbol: "ETH", AssetType: "crypto", Currency: "MYR"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(fetchErrors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(fetchErrors))
	}
	for _, fe := range fetchErrors {
		if !strings.Contains(fe.Err.Error(), "429") {
			t.Errorf("expected error to mention 429, got: %v", fe.Err)
		}
	}
}

func TestLookupCoinGeckoID_Found(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{"BTC", "bitcoin"},
		{"ETH", "ethereum"},
		{"SOL", "solana"},
	}
	for _, tt := range tests {
		id, ok := LookupCoinGeckoID(tt.symbol)
		if !ok {
			t.Errorf("LookupCoinGeckoID(%q): expected found", tt.symbol)
			continue
		}
		if id != tt.want {
			t.Errorf("LookupCoinGeckoID(%q) = %q, want %q", tt.symbol, id, tt.want)
		}
	}
}

func TestLookupCoinGeckoID_CaseInsensitive(t *testing.T) {
	id, ok := LookupCoinGeckoID("btc")
	if !ok {
		t.Fatal("expected LookupCoinGeckoID(btc) to find match")
	}
	if id != "bitcoin" {
		t.Errorf("got %q, want bitcoin", id)
	}
}

func TestLookupCoinGeckoID_NotFound(t *testing.T) {
	id, ok := LookupCoinGeckoID("DOESNOTEXIST")
	if ok {
		t.Error("expected LookupCoinGeckoID(DOESNOTEXIST) to return false")
	}
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestCoinGeckoProvider_FetchPrices_GroupsByCurrency(t *testing.T) {
	// Prices are available in both MYR and USD; each request asks for one currency.
	table := map[string]map[string]float64{
		"bitcoin":  {"myr": 300539.12, "usd": 67234.56},
		"ethereum": {"myr": 15457.82, "usd": 3456.78},
		"solana":   {"myr": 800.10, "usd": 180.25},
	}
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vs := r.URL.Query().Get("vs_currencies")
		queries = append(queries, r.URL.RawQuery)
		resp := map[string]map[string]float64{}
		for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
			if p, ok := table[id][vs]; ok {
				resp[id] = map[string]float64{vs: p}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "MYR"},
		{ID: "sec-2", Symbol: "ETH", AssetType: "crypto", Currency: "usd"},
		{ID: "sec-3", Symbol: "SOL", AssetType: "crypto", Currency: "USD"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %v", fetchErrors)
	}
	if len(queries) != 2 {
		t.Fatalf("expected one request per currency group, got %d: %v", len(queries), queries)
	}

	want := map[string]struct {
		price    int64
		currency string
	}{
		"sec-1": {30053912, "MYR"},
		"sec-2": {345678, "USD"},
		"sec-3": {18025, "USD"},
	}
	if len(results) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(results))
	}
	for _, r := range results {
		w, ok := want[r.SecurityID]
		if !ok {
			t.Fatalf("unexpected result for %s", r.SecurityID)
		}
		if r.Price != w.price || r.Currency != w.currency {
			t.Errorf("%s: got %d %s, want %d %s", r.SecurityID, r.Price, r.Currency, w.price, w.currency)
		}
	}
}

func TestCoinGeckoProvider_FetchPrices_MissingCurrencyIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("should not make an HTTP request for a security without a currency")
	}))
	defer server.Close()

	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL}
	results, fetchErrors := p.FetchPrices(context.Background(), []Security{
		{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: ""},
	})
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(fetchErrors) != 1 || !strings.Contains(fetchErrors[0].Err.Error(), "no currency") {
		t.Errorf("expected one no-currency error, got %v", fetchErrors)
	}
}
