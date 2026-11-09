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

// minorUnitCurrencies maps the sub-unit currency codes Yahoo reports (case
// sensitive) to the major ISO 4217 currency they are a hundredth of.
var minorUnitCurrencies = map[string]string{
	"GBp": "GBP",
	"GBX": "GBP",
	"ZAc": "ZAR",
	"ILA": "ILS",
}

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

// yahooChartResult is one entry of a chart response's result list.
type yahooChartResult struct {
	Meta       yahooChartMeta `json:"meta"`
	Timestamp  []int64        `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Close []*float64 `json:"close"`
		} `json:"quote"`
	} `json:"indicators"`
}

// yahooChartResponse is the top-level Yahoo Finance v8 chart API response.
type yahooChartResponse struct {
	Chart struct {
		Result []yahooChartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// yahooChartMeta is the meta block of a Yahoo chart result.
type yahooChartMeta struct {
	Symbol             string  `json:"symbol"`
	Currency           string  `json:"currency"`
	RegularMarketPrice float64 `json:"regularMarketPrice"`
}

// fetchChart requests a Yahoo v8 chart URL (with retries and the shared
// User-Agent) and returns the first result. It is the single place that turns a
// chart response into data, shared by the live price, FX rate and history fetches.
// label names the ticker in error messages.
func fetchChart(ctx context.Context, httpClient *http.Client, url, label string) (*yahooChartResult, error) {
	resp, err := doWithRetry(ctx, httpClient, func() (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("building request: %w", reqErr)
		}
		req.Header.Set("User-Agent", yahooUA)
		return req, nil
	})
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
		return nil, fmt.Errorf("no results for %s", label)
	}
	return &chartResp.Chart.Result[0], nil
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
	res, err := fetchChart(ctx, p.httpClient, p.baseURL+"/"+ticker+"?interval=1d&range=1d", ticker)
	if err != nil {
		return nil, err
	}

	meta := res.Meta
	if meta.RegularMarketPrice == 0 {
		return nil, fmt.Errorf("zero price for %s", ticker)
	}

	cents, currency := NormalizeYahooPrice(meta.RegularMarketPrice, meta.Currency)
	return &PriceResult{
		SecurityID: secID,
		Price:      cents,
		Currency:   currency,
		RecordedAt: now,
	}, nil
}

// FetchCurrency returns the normalised ISO 4217 currency Yahoo reports for ticker
// (minor units such as GBp map to their major currency).
func (p *YahooProvider) FetchCurrency(ctx context.Context, ticker string) (string, error) {
	res, err := fetchChart(ctx, p.httpClient, p.baseURL+"/"+ticker+"?interval=1d&range=1d", ticker)
	if err != nil {
		return "", err
	}
	if res.Meta.Currency == "" {
		return "", fmt.Errorf("no currency reported for %s", ticker)
	}
	_, currency := NormalizeYahooPrice(0, res.Meta.Currency)
	return currency, nil
}

// Bar is one daily history point: the bar's timestamp and its close.
type Bar struct {
	Time  time.Time
	Close float64
}

// History is a ticker's daily history as reported by Yahoo. Currency is the raw
// meta.currency (not normalised); Bars exclude entries with a null close.
type History struct {
	Currency string
	Bars     []Bar
}

// BuildYahooSymbol returns the Yahoo ticker the provider would request for sec.
func BuildYahooSymbol(sec Security) string { return buildYahooSymbol(sec) }

// FetchHistory fetches daily bars for ticker with from <= bar < to (period1 and
// period2 in unix seconds, interval=1d) through the same chart request, retry and
// parsing as the live fetch. Bars whose close is null are skipped.
func (p *YahooProvider) FetchHistory(ctx context.Context, ticker string, from, to time.Time) (*History, error) {
	return fetchHistory(ctx, p.httpClient, p.baseURL, ticker, from, to)
}

// FetchHistory fetches daily history for a currency pair (ticker BASEQUOTE=X).
func (f *ForexConverter) FetchHistory(ctx context.Context, base, quote string, from, to time.Time) (*History, error) {
	ticker := strings.ToUpper(base) + strings.ToUpper(quote) + "=X"
	return fetchHistory(ctx, f.httpClient, f.baseURL, ticker, from, to)
}

func fetchHistory(ctx context.Context, httpClient *http.Client, baseURL, ticker string, from, to time.Time) (*History, error) {
	url := fmt.Sprintf("%s/%s?interval=1d&period1=%d&period2=%d", baseURL, ticker, from.Unix(), to.Unix())
	res, err := fetchChart(ctx, httpClient, url, ticker)
	if err != nil {
		return nil, err
	}
	h := &History{Currency: res.Meta.Currency}
	if len(res.Indicators.Quote) == 0 {
		return h, nil
	}
	closes := res.Indicators.Quote[0].Close
	for i, ts := range res.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue
		}
		h.Bars = append(h.Bars, Bar{Time: time.Unix(ts, 0).UTC(), Close: *closes[i]})
	}
	return h, nil
}

// NormalizeYahooPrice converts a price as reported by Yahoo, in the unit named by
// reportedCurrency, into integer cents of the normalised ISO 4217 currency.
//
// Sub-unit codes (GBp, GBX, ZAc, ILA) are mapped to their major currency. Their
// price is already in hundredths of the major unit, i.e. the major currency's
// cents, so the cents are the price rounded once (half away from zero): 525.12
// GBp becomes 525 GBP cents (5.25 GBP). Other currencies are price*100 rounded.
// The codes are matched case-sensitively, so "GBP" is never treated as a sub-unit.
func NormalizeYahooPrice(price float64, reportedCurrency string) (cents int64, currency string) {
	if major, ok := minorUnitCurrencies[reportedCurrency]; ok {
		return int64(math.Round(price)), major
	}
	return int64(math.Round(price * 100)), strings.ToUpper(reportedCurrency)
}
