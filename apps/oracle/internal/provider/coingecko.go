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
	httpClient     *http.Client
	baseURL        string // overridable for tests
	targetCurrency string // currency code for vs_currencies (e.g. "myr")
}

// NewCoinGeckoProvider creates a new CoinGecko price provider.
// targetCurrency sets the vs_currencies parameter (e.g. "MYR") so all
// crypto prices are returned in the target currency directly.
func NewCoinGeckoProvider(httpClient *http.Client, targetCurrency string) *CoinGeckoProvider {
	return &CoinGeckoProvider{
		httpClient:     httpClient,
		baseURL:        "https://api.coingecko.com/api/v3/simple/price",
		targetCurrency: strings.ToLower(targetCurrency),
	}
}

// Name returns the provider's display name.
func (p *CoinGeckoProvider) Name() string { return "CoinGecko" }

// Supports returns true for crypto asset type only.
func (p *CoinGeckoProvider) Supports(assetType string) bool {
	return assetType == "crypto"
}

// FetchPrices fetches current prices from CoinGecko.
func (p *CoinGeckoProvider) FetchPrices(ctx context.Context, securities []Security) ([]PriceResult, []FetchError) {
	if len(securities) == 0 {
		return nil, nil
	}

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
		chunkPrices, chunkErrors := p.fetchChunk(ctx, chunk, idToSecs)
		prices = append(prices, chunkPrices...)
		fetchErrors = append(fetchErrors, chunkErrors...)
	}

	return prices, fetchErrors
}

// fetchChunk fetches one batch of CoinGecko ids via the simple price API.
func (p *CoinGeckoProvider) fetchChunk(ctx context.Context, ids []string, idToSecs map[string][]Security) ([]PriceResult, []FetchError) {
	chunkSecs := make(map[string][]Security, len(ids))
	for _, id := range ids {
		chunkSecs[id] = idToSecs[id]
	}

	// Use the configured target currency for all price requests.
	currency := p.targetCurrency

	// Call CoinGecko simple price API.
	url := p.baseURL + "?ids=" + strings.Join(ids, ",") + "&vs_currencies=" + currency

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
		price, hasCurrency := priceMap[currency]
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
				Currency:   strings.ToUpper(currency),
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
