package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	yahooBaseURL       = "https://query1.finance.yahoo.com/v8/finance/chart"
	yahooMaxConcurrent = 10
	yahooUA            = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
)

// exchangeSuffixes maps exchange codes to Yahoo Finance ticker suffixes.
var exchangeSuffixes = map[string]string{
	"TSX":      ".TO",
	"TSXV":     ".V",
	"LSE":      ".L",
	"HKEX":     ".HK",
	"ASX":      ".AX",
	"NSE":      ".NS",
	"BSE":      ".BO",
	"SGX":      ".SI",
	"KRX":      ".KS",
	"KOSDAQ":   ".KQ",
	"BURSA":    ".KL",
	"JPX":      ".T",
	"FRA":      ".F",
	"XETRA":    ".DE",
	"SIX":      ".SW",
	"EURONEXT": ".PA",
}

// yahooChartResponse is the top-level Yahoo Finance v8 chart API response.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				Currency           string  `json:"currency"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// YahooProvider fetches prices from Yahoo Finance for stocks, ETFs, bonds, and REITs.
type YahooProvider struct {
	httpClient *http.Client
	baseURL    string // overridable for tests
}

// NewYahooProvider creates a new Yahoo Finance price provider.
func NewYahooProvider(httpClient *http.Client) *YahooProvider {
	return &YahooProvider{httpClient: httpClient, baseURL: yahooBaseURL}
}

// Name returns the provider's display name.
func (p *YahooProvider) Name() string { return "Yahoo Finance" }

// Supports returns true for stock, etf, bond, reit, and commodity asset types.
func (p *YahooProvider) Supports(assetType string) bool {
	switch assetType {
	case "stock", "etf", "bond", "reit", "commodity":
		return true
	default:
		return false
	}
}

// buildYahooSymbol converts a security to a Yahoo-compatible ticker.
// If ProviderSymbol is set, it is used directly. Otherwise, the symbol
// is combined with the exchange suffix from the exchangeSuffixes map.
func buildYahooSymbol(sec Security) string {
	if sec.ProviderSymbol != "" {
		return sec.ProviderSymbol
	}
	if suffix, ok := exchangeSuffixes[sec.Exchange]; ok {
		return sec.Symbol + suffix
	}
	return sec.Symbol
}

// FetchPrices fetches current prices from Yahoo Finance using the v8 chart endpoint.
// Each security is fetched individually with concurrent requests limited by a semaphore.
func (p *YahooProvider) FetchPrices(ctx context.Context, securities []Security) ([]PriceResult, []FetchError) {
	if len(securities) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	sem := make(chan struct{}, yahooMaxConcurrent)

	var mu sync.Mutex
	var results []PriceResult
	var fetchErrors []FetchError

	var wg sync.WaitGroup
	for _, sec := range securities {
		wg.Add(1)
		go func(sec Security) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			ticker := buildYahooSymbol(sec)
			result, err := p.fetchOne(ctx, ticker, sec.ID, now)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fetchErrors = append(fetchErrors, FetchError{
					SecurityID: sec.ID,
					Symbol:     sec.Symbol,
					Err:        err,
				})
				return
			}
			results = append(results, *result)
		}(sec)
	}
	wg.Wait()

	return results, fetchErrors
}

// fetchOne fetches the price for a single ticker from the Yahoo v8 chart endpoint.
func (p *YahooProvider) fetchOne(ctx context.Context, ticker string, secID string, now time.Time) (*PriceResult, error) {
	url := p.baseURL + "/" + ticker + "?interval=1d&range=1d"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", yahooUA)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var chartResp yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&chartResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if chartResp.Chart.Error != nil {
		return nil, fmt.Errorf("chart error %s: %s", chartResp.Chart.Error.Code, chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return nil, fmt.Errorf("no results for %s", ticker)
	}

	meta := chartResp.Chart.Result[0].Meta
	if meta.RegularMarketPrice == 0 {
		return nil, fmt.Errorf("zero price for %s", ticker)
	}

	return &PriceResult{
		SecurityID: secID,
		Price:      int64(math.Round(meta.RegularMarketPrice * 100)),
		Currency:   strings.ToUpper(meta.Currency),
		RecordedAt: now,
	}, nil
}
