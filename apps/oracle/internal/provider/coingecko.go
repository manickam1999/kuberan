package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// coinGeckoChunkSize caps the ids sent in one simple/price request (a
// variable so tests can shrink it).
var coinGeckoChunkSize = 250

// CoinGeckoProvider fetches prices from CoinGecko for cryptocurrencies.
type CoinGeckoProvider struct {
	httpClient *http.Client
	baseURL    string // overridable for tests
}

// NewCoinGeckoProvider creates a new CoinGecko price provider. Each security is
// priced in its own currency (securities are grouped by currency), so no target
// currency is configured here.
func NewCoinGeckoProvider(httpClient *http.Client) *CoinGeckoProvider {
	return &CoinGeckoProvider{
		httpClient: httpClient,
		baseURL:    "https://api.coingecko.com/api/v3/simple/price",
	}
}

// Name returns the provider's display name.
func (p *CoinGeckoProvider) Name() string { return "CoinGecko" }

// Supports returns true for crypto asset type only.
func (p *CoinGeckoProvider) Supports(assetType string) bool {
	return assetType == "crypto"
}

// FetchPrices fetches current prices from CoinGecko. Securities are grouped by
// their upper-cased currency; each group is priced in that currency with one
// request per chunk of ids, and the reported currency of its prices is the group's.
func (p *CoinGeckoProvider) FetchPrices(ctx context.Context, securities []Security) ([]PriceResult, []FetchError) {
	if len(securities) == 0 {
		return nil, nil
	}

	var fetchErrors []FetchError
	byCurrency := make(map[string][]Security)
	var currencies []string

	for _, sec := range securities {
		currency := strings.ToUpper(sec.Currency)
		if currency == "" {
			fetchErrors = append(fetchErrors, FetchError{
				SecurityID: sec.ID,
				Symbol:     sec.Symbol,
				Err:        fmt.Errorf("security %s has no currency", sec.Symbol),
			})
			continue
		}
		if _, exists := byCurrency[currency]; !exists {
			currencies = append(currencies, currency)
		}
		byCurrency[currency] = append(byCurrency[currency], sec)
	}

	var prices []PriceResult
	for _, currency := range currencies {
		curPrices, curErrors := p.fetchCurrency(ctx, currency, byCurrency[currency])
		prices = append(prices, curPrices...)
		fetchErrors = append(fetchErrors, curErrors...)
	}

	return prices, fetchErrors
}

// fetchCurrency prices the securities of one currency group.
func (p *CoinGeckoProvider) fetchCurrency(ctx context.Context, currency string, securities []Security) ([]PriceResult, []FetchError) {
	// Map securities to CoinGecko IDs.
	var fetchErrors []FetchError
	idToSecs := make(map[string][]Security) // CoinGecko ID -> securities (multiple tickers can map to same ID)
	var ids []string

	for _, sec := range securities {
		cgID, ok := LookupCoinGeckoID(sec.Symbol)
		if !ok {
			fetchErrors = append(fetchErrors, FetchError{
				SecurityID: sec.ID,
				Symbol:     sec.Symbol,
				Err:        fmt.Errorf("no CoinGecko mapping for symbol %s", sec.Symbol),
			})
			continue
		}
		if _, exists := idToSecs[cgID]; !exists {
			ids = append(ids, cgID)
		}
		idToSecs[cgID] = append(idToSecs[cgID], sec)
	}

	if len(ids) == 0 {
		return nil, fetchErrors
	}

	// Query CoinGecko in chunks so the URL and per-request id count stay
	// within the API's limits; a failed chunk only fails its own securities.
	var prices []PriceResult
	for start := 0; start < len(ids); start += coinGeckoChunkSize {
		chunk := ids[start:min(start+coinGeckoChunkSize, len(ids))]
		chunkPrices, chunkErrors := p.fetchChunk(ctx, currency, chunk, idToSecs)
		prices = append(prices, chunkPrices...)
		fetchErrors = append(fetchErrors, chunkErrors...)
	}

	return prices, fetchErrors
}

// fetchChunk fetches one batch of CoinGecko ids, priced in currency, via the
// simple price API.
func (p *CoinGeckoProvider) fetchChunk(ctx context.Context, currency string, ids []string, idToSecs map[string][]Security) ([]PriceResult, []FetchError) {
	chunkSecs := make(map[string][]Security, len(ids))
	for _, id := range ids {
		chunkSecs[id] = idToSecs[id]
	}

	// CoinGecko keys its price maps by lower-case vs_currency.
	vsCurrency := strings.ToLower(currency)

	// Call CoinGecko simple price API.
	url := p.baseURL + "?ids=" + strings.Join(ids, ",") + "&vs_currencies=" + vsCurrency

	resp, err := doWithRetry(ctx, p.httpClient, func() (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("building request: %w", reqErr)
		}
		return req, nil
	})
	if err != nil {
		return nil, appendAllErrors(nil, chunkSecs, fmt.Errorf("http request: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, appendAllErrors(nil, chunkSecs, fmt.Errorf("unexpected status %d", resp.StatusCode))
	}

	// Response shape: {"bitcoin": {"usd": 67234.56}, "ethereum": {"usd": 3456.78}}
	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, appendAllErrors(nil, chunkSecs, fmt.Errorf("decoding response: %w", err))
	}

	now := time.Now().UTC()
	var prices []PriceResult
	var fetchErrors []FetchError

	for _, cgID := range ids {
		secs := idToSecs[cgID]
		priceMap, found := result[cgID]
		if !found {
			for _, sec := range secs {
				fetchErrors = append(fetchErrors, FetchError{
					SecurityID: sec.ID,
					Symbol:     sec.Symbol,
					Err:        fmt.Errorf("CoinGecko ID %s not found in response", cgID),
				})
			}
			continue
		}
		price, hasCurrency := priceMap[vsCurrency]
		if !hasCurrency || price == 0 {
			for _, sec := range secs {
				fetchErrors = append(fetchErrors, FetchError{
					SecurityID: sec.ID,
					Symbol:     sec.Symbol,
					Err:        fmt.Errorf("no %s price for CoinGecko ID %s", currency, cgID),
				})
			}
			continue
		}
		cents := int64(math.Round(price * 100))
		for _, sec := range secs {
			prices = append(prices, PriceResult{
				SecurityID: sec.ID,
				Price:      cents,
				Currency:   currency,
				RecordedAt: now,
			})
		}
	}

	return prices, fetchErrors
}

// appendAllErrors creates FetchErrors for all mapped securities and appends to existing errors.
func appendAllErrors(existing []FetchError, idToSecs map[string][]Security, err error) []FetchError {
	for _, secs := range idToSecs {
		for _, sec := range secs {
			existing = append(existing, FetchError{
				SecurityID: sec.ID,
				Symbol:     sec.Symbol,
				Err:        err,
			})
		}
	}
	return existing
}
