package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// v8ChartResponse builds a v8 chart JSON response for a single symbol.
func v8ChartResponse(symbol string, price float64) yahooChartResponse {
	return v8ChartResponseWithCurrency(symbol, price, "USD")
}

// v8ChartResponseWithCurrency builds a v8 chart JSON response with an explicit currency.
func v8ChartResponseWithCurrency(symbol string, price float64, currency string) yahooChartResponse {
	var resp yahooChartResponse
	resp.Chart.Result = []yahooChartResult{
		{Meta: yahooChartMeta{Symbol: symbol, Currency: currency, RegularMarketPrice: price}},
	}
	return resp
}

// v8ChartErrorResponse builds a v8 chart error JSON response.
func v8ChartErrorResponse(code, description string) yahooChartResponse {
	var resp yahooChartResponse
	resp.Chart.Error = &struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	}{Code: code, Description: description}
	return resp
}

// newV8MockServer creates a test server that serves v8 chart responses per symbol.
// priceMap maps ticker (from URL path) to price. Tickers not in the map get a chart error.
// All prices are returned with currency "USD".
func newV8MockServer(priceMap map[string]float64) *httptest.Server {
	return newV8MockServerWithCurrency(priceMap, "USD")
}

// newV8MockServerWithCurrency creates a test server with a specific currency for all responses.
func newV8MockServerWithCurrency(priceMap map[string]float64, currency string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")

		price, ok := priceMap[ticker]
		if !ok {
			_ = json.NewEncoder(w).Encode(v8ChartErrorResponse("Not Found", "No data found, symbol may be delisted"))
			return
		}
		_ = json.NewEncoder(w).Encode(v8ChartResponseWithCurrency(ticker, price, currency))
	}))
}

func TestYahooProvider_Supports(t *testing.T) {
	p := NewYahooProvider(http.DefaultClient)

	supported := []string{"stock", "etf", "bond", "reit", "commodity"}
	for _, at := range supported {
		if !p.Supports(at) {
			t.Errorf("expected Supports(%q) = true", at)
		}
	}

	unsupported := []string{"crypto", ""}
	for _, at := range unsupported {
		if p.Supports(at) {
			t.Errorf("expected Supports(%q) = false", at)
		}
	}
}

func TestYahooProvider_FetchPrices_Success(t *testing.T) {
	server := newV8MockServer(map[string]float64{
		"AAPL":  178.72,
		"MSFT":  420.55,
		"GOOGL": 175.03,
	})
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "AAPL", AssetType: "stock"},
		{ID: "sec-2", Symbol: "MSFT", AssetType: "stock"},
		{ID: "sec-3", Symbol: "GOOGL", AssetType: "stock"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := map[string]int64{
		"sec-1": 17872,
		"sec-2": 42055,
		"sec-3": 17503,
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
		if r.Currency != "USD" {
			t.Errorf("security %s: got currency %q, want %q", r.SecurityID, r.Currency, "USD")
		}
	}
}

func TestYahooProvider_FetchPrices_PartialFailure(t *testing.T) {
	// Only AAPL and MSFT have prices; FAKESYM is missing → chart error.
	server := newV8MockServer(map[string]float64{
		"AAPL": 178.72,
		"MSFT": 420.55,
	})
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "AAPL", AssetType: "stock"},
		{ID: "sec-2", Symbol: "MSFT", AssetType: "stock"},
		{ID: "sec-3", Symbol: "FAKESYM", AssetType: "stock"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if len(fetchErrors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(fetchErrors))
	}
	if fetchErrors[0].SecurityID != "sec-3" {
		t.Errorf("expected error for security ID sec-3, got %s", fetchErrors[0].SecurityID)
	}
}

func TestYahooProvider_FetchPrices_ExchangeSuffix(t *testing.T) {
	var capturedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPaths = append(capturedPaths, r.URL.Path)
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse(ticker, 100.00))
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "SHOP", AssetType: "stock", Exchange: "TSX"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	found := false
	for _, path := range capturedPaths {
		if strings.Contains(path, "SHOP.TO") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected request path to contain SHOP.TO, got paths=%v", capturedPaths)
	}
}

func TestYahooProvider_FetchPrices_ProviderSymbol(t *testing.T) {
	var capturedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPaths = append(capturedPaths, r.URL.Path)
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse(ticker, 6.50))
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "CIMB", AssetType: "stock", Exchange: "BURSA", ProviderSymbol: "1023.KL"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Price != 650 {
		t.Errorf("expected price 650 cents, got %d", results[0].Price)
	}

	// Verify the request used provider_symbol (1023.KL), not CIMB.KL.
	foundProvider := false
	foundWrong := false
	for _, path := range capturedPaths {
		if strings.Contains(path, "1023.KL") {
			foundProvider = true
		}
		if strings.Contains(path, "CIMB.KL") {
			foundWrong = true
		}
	}
	if !foundProvider {
		t.Errorf("expected request path to contain 1023.KL, got paths=%v", capturedPaths)
	}
	if foundWrong {
		t.Errorf("should NOT contain CIMB.KL, got paths=%v", capturedPaths)
	}
}

func TestYahooProvider_FetchPrices_Concurrent(t *testing.T) {
	var maxInFlight atomic.Int32
	var curInFlight atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := curInFlight.Add(1)
		// Track peak concurrency.
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		defer curInFlight.Add(-1)

		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartResponse(ticker, 100.00))
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}

	// Create 15 securities.
	securities := make([]Security, 15)
	for i := range securities {
		securities[i] = Security{
			ID:        fmt.Sprintf("sec-%d", i+1),
			Symbol:    "SYM" + string(rune('A'+i)),
			AssetType: "stock",
		}
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 15 {
		t.Errorf("expected 15 results, got %d", len(results))
	}
	if peak := maxInFlight.Load(); peak > int32(yahooMaxConcurrent) {
		t.Errorf("peak concurrency %d exceeded limit %d", peak, yahooMaxConcurrent)
	}
}

func TestYahooProvider_FetchPrices_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "AAPL", AssetType: "stock"},
		{ID: "sec-2", Symbol: "MSFT", AssetType: "stock"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(fetchErrors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(fetchErrors))
	}
	for _, fe := range fetchErrors {
		if !strings.Contains(fe.Err.Error(), "500") {
			t.Errorf("expected error to mention 500, got: %v", fe.Err)
		}
	}
}

func TestYahooProvider_FetchPrices_ZeroPrice(t *testing.T) {
	server := newV8MockServer(map[string]float64{
		"DEAD": 0,
	})
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "DEAD", AssetType: "stock"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 0 {
		t.Errorf("expected 0 results for zero price, got %d", len(results))
	}
	if len(fetchErrors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(fetchErrors))
	}
	if !strings.Contains(fetchErrors[0].Err.Error(), "zero price") {
		t.Errorf("expected error about zero price, got: %v", fetchErrors[0].Err)
	}
}

func TestYahooProvider_FetchPrices_ChartError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v8ChartErrorResponse("Not Found", "No data found, symbol may be delisted"))
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "DELISTED", AssetType: "stock"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(fetchErrors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(fetchErrors))
	}
	if !strings.Contains(fetchErrors[0].Err.Error(), "Not Found") {
		t.Errorf("expected error to mention 'Not Found', got: %v", fetchErrors[0].Err)
	}
}

func TestYahooProvider_FetchPrices_CurrencyFromResponse(t *testing.T) {
	// Each ticker responds with a different currency via per-ticker handlers.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		switch ticker {
		case "AAPL":
			_ = json.NewEncoder(w).Encode(v8ChartResponseWithCurrency("AAPL", 178.72, "USD"))
		case "1023.KL":
			_ = json.NewEncoder(w).Encode(v8ChartResponseWithCurrency("1023.KL", 8.55, "MYR"))
		case "VUAA.L":
			_ = json.NewEncoder(w).Encode(v8ChartResponseWithCurrency("VUAA.L", 525.12, "GBp"))
		default:
			_ = json.NewEncoder(w).Encode(v8ChartErrorResponse("Not Found", "unknown"))
		}
	}))
	defer server.Close()

	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	securities := []Security{
		{ID: "sec-1", Symbol: "AAPL", AssetType: "stock"},
		{ID: "sec-2", Symbol: "CIMB", AssetType: "stock", Exchange: "BURSA", ProviderSymbol: "1023.KL"},
		{ID: "sec-3", Symbol: "VUAA", AssetType: "etf", Exchange: "LSE", ProviderSymbol: "VUAA.L"},
	}

	results, fetchErrors := p.FetchPrices(context.Background(), securities)
	if len(fetchErrors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(fetchErrors), fetchErrors)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := map[string]struct {
		price    int64
		currency string
	}{
		"sec-1": {17872, "USD"},
		"sec-2": {855, "MYR"},
		"sec-3": {525, "GBP"}, // 525.12 GBp = 5.2512 GBP, i.e. 525 GBP cents
	}
	for _, r := range results {
		want, ok := expected[r.SecurityID]
		if !ok {
			t.Errorf("unexpected security ID %s", r.SecurityID)
			continue
		}
		if r.Price != want.price {
			t.Errorf("security %s: got price %d, want %d", r.SecurityID, r.Price, want.price)
		}
		if r.Currency != want.currency {
			t.Errorf("security %s: got currency %q, want %q", r.SecurityID, r.Currency, want.currency)
		}
	}
}

func TestNormalizeYahooPrice(t *testing.T) {
	tests := []struct {
		name     string
		price    float64
		reported string
		want     int64
		currency string
	}{
		{"USD is ordinary cents", 178.72, "USD", 17872, "USD"},
		{"lower-case reported currency is upper-cased", 178.72, "usd", 17872, "USD"},
		{"MYR is ordinary cents", 8.55, "MYR", 855, "MYR"},
		{"GBp is pence: 525.12 GBp is 525 GBP cents", 525.12, "GBp", 525, "GBP"},
		{"GBX is pence", 525.12, "GBX", 525, "GBP"},
		{"ZAc is South African cents", 1234.5, "ZAc", 1235, "ZAR"},
		{"ILA is agorot", 3210.4, "ILA", 3210, "ILS"},
		{"GBp half rounds away from zero", 100.5, "GBp", 101, "GBP"},
		{"GBp just below half rounds down", 100.49, "GBp", 100, "GBP"},
		{"GBp with three decimals rounds at the hundredth", 1234.567, "GBp", 1235, "GBP"},
		{"GBp rounds once: 525.495 is 525, not 526 via a rounded hundredth", 525.495, "GBp", 525, "GBP"},
		{"GBP is not a minor unit", 5.25, "GBP", 525, "GBP"},
		{"gbp in lower case is not a minor unit", 5.25, "gbp", 525, "GBP"},
		{"gbx in lower case is not a minor unit", 525, "gbx", 52500, "GBX"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cur := NormalizeYahooPrice(tt.price, tt.reported)
			if got != tt.want || cur != tt.currency {
				t.Errorf("NormalizeYahooPrice(%v, %q) = (%d, %q), want (%d, %q)",
					tt.price, tt.reported, got, cur, tt.want, tt.currency)
			}
		})
	}
}

func TestYahooProvider_FetchHistory(t *testing.T) {
	var gotQuery, gotUA, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotUA, gotPath = r.URL.RawQuery, r.Header.Get("User-Agent"), r.URL.Path
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"symbol":"VOD.L","currency":"GBp","regularMarketPrice":70},
			"timestamp":[1735828200,1735914600,1736001000],
			"indicators":{"quote":[{"close":[70.5,null,71.25]}]}}],"error":null}}`))
	}))
	defer server.Close()
	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	h, err := p.FetchHistory(context.Background(), "VOD.L", from, to)
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if gotPath != "/VOD.L" || gotQuery != "interval=1d&period1=1735689600&period2=1738368000" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if gotUA != yahooUA {
		t.Errorf("User-Agent = %q, want the shared one", gotUA)
	}
	if h.Currency != "GBp" {
		t.Errorf("currency = %q, want raw GBp", h.Currency)
	}
	if len(h.Bars) != 2 || h.Bars[0].Close != 70.5 || h.Bars[1].Close != 71.25 {
		t.Fatalf("bars = %+v, want the null close skipped", h.Bars)
	}
	if !h.Bars[0].Time.Equal(time.Unix(1735828200, 0)) || !h.Bars[1].Time.Equal(time.Unix(1736001000, 0)) {
		t.Errorf("bar times = %v, %v", h.Bars[0].Time, h.Bars[1].Time)
	}
}

func TestYahooProvider_FetchHistory_SharesChartErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":null,"error":{"code":"Not Found","description":"No data"}}}`))
	}))
	defer server.Close()
	p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
	if _, err := p.FetchHistory(context.Background(), "NOPE", time.Unix(0, 0), time.Unix(86400, 0)); err == nil || !strings.Contains(err.Error(), "chart error Not Found") {
		t.Errorf("err = %v", err)
	}
}

func TestForexConverter_FetchHistory_UsesPairTicker(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"currency":"MYR"},"timestamp":[1735828200],"indicators":{"quote":[{"close":[4.47]}]}}],"error":null}}`))
	}))
	defer server.Close()
	fc := &ForexConverter{httpClient: server.Client(), baseURL: server.URL, rates: map[string]float64{}}
	h, err := fc.FetchHistory(context.Background(), "usd", "myr", time.Unix(0, 0), time.Unix(86400, 0))
	if err != nil || gotPath != "/USDMYR=X" || len(h.Bars) != 1 || h.Bars[0].Close != 4.47 {
		t.Errorf("path=%s h=%+v err=%v", gotPath, h, err)
	}
}

func TestYahooProvider_FetchCurrency(t *testing.T) {
	for reported, want := range map[string]string{"GBp": "GBP", "usd": "USD", "ZAc": "ZAR"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"chart":{"result":[{"meta":{"currency":%q,"regularMarketPrice":1}}],"error":null}}`, reported)
		}))
		p := &YahooProvider{httpClient: server.Client(), baseURL: server.URL}
		got, err := p.FetchCurrency(context.Background(), "X")
		server.Close()
		if err != nil || got != want {
			t.Errorf("FetchCurrency(%s) = %q, %v; want %q", reported, got, err, want)
		}
	}
}
