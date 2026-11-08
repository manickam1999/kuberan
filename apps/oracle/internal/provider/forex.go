package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ForexConverter fetches exchange rates from Yahoo Finance. It is used only to
// publish FX rates to the pipeline; prices are never converted in the oracle.
// Rates are cached in-memory for the lifetime of the converter instance, keyed
// by currency pair, so a single instance should be used per oracle run and any
// pair is fetched at most once per run.
type ForexConverter struct {
	httpClient *http.Client
	baseURL    string // overridable for tests
	mu         sync.RWMutex
	rates      map[string]float64 // e.g. "USDMYR" -> 4.47 (1 USD = 4.47 MYR)
}

// NewForexConverter creates a new ForexConverter.
func NewForexConverter(httpClient *http.Client) *ForexConverter {
	return &ForexConverter{
		httpClient: httpClient,
		baseURL:    yahooBaseURL,
		rates:      make(map[string]float64),
	}
}

// FetchRate returns the exchange rate of one unit of base expressed in quote
// (e.g. FetchRate(ctx, "USD", "MYR") = 4.47). Identical currencies yield 1.0.
// Successful rates are cached per pair, so a pair already fetched in this
// converter's lifetime is not requested again.
func (f *ForexConverter) FetchRate(ctx context.Context, base, quote string) (float64, error) {
	b := strings.ToUpper(base)
	q := strings.ToUpper(quote)
	if b == q {
		return 1.0, nil
	}
	key := b + q

	// Check cache.
	f.mu.RLock()
	rate, ok := f.rates[key]
	f.mu.RUnlock()
	if ok {
		return rate, nil
	}

	// Fetch from Yahoo Finance.
	rate, err := f.fetchRate(ctx, b, q)
	if err != nil {
		return 0, err
	}

	// Cache the rate.
	f.mu.Lock()
	f.rates[key] = rate
	f.mu.Unlock()

	return rate, nil
}

// fetchRate fetches the exchange rate for a currency pair from Yahoo Finance.
// Yahoo Finance uses tickers like "USDMYR=X" for forex pairs.
func (f *ForexConverter) fetchRate(ctx context.Context, base, quote string) (float64, error) {
	ticker := base + quote + "=X"
	res, err := fetchChart(ctx, f.httpClient, f.baseURL+"/"+ticker+"?interval=1d&range=1d", ticker)
	if err != nil {
		return 0, fmt.Errorf("forex request for %s: %w", ticker, err)
	}

	rate := res.Meta.RegularMarketPrice
	if rate <= 0 {
		return 0, fmt.Errorf("invalid forex rate for %s: %f", ticker, rate)
	}

	return rate, nil
}
