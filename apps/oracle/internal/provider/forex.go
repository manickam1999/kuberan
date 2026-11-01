package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
)

// ForexConverter fetches exchange rates from Yahoo Finance and converts
// prices from their native currency to a target currency (e.g. MYR).
// Rates are cached in-memory for the lifetime of the converter instance,
// so a single instance should be used per oracle run.
type ForexConverter struct {
	httpClient     *http.Client
	baseURL        string // overridable for tests
	targetCurrency string
	mu             sync.RWMutex
	rates          map[string]float64 // e.g. "USD" -> 4.47 (1 USD = 4.47 MYR)
}

// NewForexConverter creates a new ForexConverter that converts to the given target currency.
func NewForexConverter(httpClient *http.Client, targetCurrency string) *ForexConverter {
	return &ForexConverter{
		httpClient:     httpClient,
		baseURL:        yahooBaseURL,
		targetCurrency: strings.ToUpper(targetCurrency),
		rates:          make(map[string]float64),
	}
}

// TargetCurrency returns the target currency code (e.g. "MYR").
func (f *ForexConverter) TargetCurrency() string {
	return f.targetCurrency
}

// NeedsConversion returns true if the given currency differs from the target.
func (f *ForexConverter) NeedsConversion(fromCurrency string) bool {
	return strings.ToUpper(fromCurrency) != f.targetCurrency
}

// GetRate fetches (or returns cached) the exchange rate from fromCurrency to the target currency.
// For example, if targetCurrency is MYR and fromCurrency is USD, it fetches USDMYR=X from Yahoo
// and returns the rate (e.g. 4.47).
func (f *ForexConverter) GetRate(ctx context.Context, fromCurrency string) (float64, error) {
	from := strings.ToUpper(fromCurrency)
	if from == f.targetCurrency {
		return 1.0, nil
	}

	// Check cache.
	f.mu.RLock()
	rate, ok := f.rates[from]
	f.mu.RUnlock()
	if ok {
		return rate, nil
	}

	// Fetch from Yahoo Finance.
	rate, err := f.fetchRate(ctx, from)
	if err != nil {
		return 0, err
	}

	// Cache the rate.
	f.mu.Lock()
	f.rates[from] = rate
	f.mu.Unlock()

	return rate, nil
}

// Convert converts a price in cents from the given currency to the target currency.
// Returns the converted price in target currency cents.
func (f *ForexConverter) Convert(ctx context.Context, priceCents int64, fromCurrency string) (int64, error) {
	if !f.NeedsConversion(fromCurrency) {
		return priceCents, nil
	}

	rate, err := f.GetRate(ctx, fromCurrency)
	if err != nil {
		return 0, err
	}

	converted := float64(priceCents) * rate
	return int64(math.Round(converted)), nil
}

// fetchRate fetches the exchange rate for a currency pair from Yahoo Finance.
// Yahoo Finance uses tickers like "USDMYR=X" for forex pairs.
func (f *ForexConverter) fetchRate(ctx context.Context, fromCurrency string) (float64, error) {
	ticker := fromCurrency + f.targetCurrency + "=X"
	url := f.baseURL + "/" + ticker + "?interval=1d&range=1d"

	resp, err := doWithRetry(ctx, f.httpClient, func() (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("building forex request: %w", reqErr)
		}
		req.Header.Set("User-Agent", yahooUA)
		return req, nil
	})
	if err != nil {
		return 0, fmt.Errorf("forex http request for %s: %w", ticker, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("forex request for %s: unexpected status %d", ticker, resp.StatusCode)
	}

	var chartResp yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&chartResp); err != nil {
		return 0, fmt.Errorf("decoding forex response for %s: %w", ticker, err)
	}

	if chartResp.Chart.Error != nil {
		return 0, fmt.Errorf("forex chart error for %s: %s: %s", ticker, chartResp.Chart.Error.Code, chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return 0, fmt.Errorf("no forex results for %s", ticker)
	}

	rate := chartResp.Chart.Result[0].Meta.RegularMarketPrice
	if rate <= 0 {
		return 0, fmt.Errorf("invalid forex rate for %s: %f", ticker, rate)
	}

	return rate, nil
}
