// Package oracle orchestrates fetching prices from providers and pushing them to the Kuberan API.
package oracle

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/config"
	"github.com/kuberan/oracle/internal/provider"
)

// SecurityClient defines the Kuberan API operations needed by the oracle.
type SecurityClient interface {
	GetSecurities(ctx context.Context) ([]client.Security, error)
	RecordPrices(ctx context.Context, prices []client.RecordPriceEntry) (client.RecordPricesResult, error)
	ComputeSnapshots(ctx context.Context) (int, error)
	ProcessFixedDeposits(ctx context.Context) (client.ProcessFixedDepositsResult, error)
	GetFXPairs(ctx context.Context) (client.FXPairs, error)
	RecordFXRates(ctx context.Context, rates []client.FXRateEntry) (int, error)
}

// RateFetcher fetches FX rates for publishing to the pipeline. Prices are never
// converted, so this is the only currency operation the oracle needs.
type RateFetcher interface {
	// FetchRate returns the rate of one unit of base in quote. Implementations
	// should cache rates so that a pair is requested at most once per run.
	FetchRate(ctx context.Context, base, quote string) (float64, error)
}

// RunResult contains the outcome of an oracle run.
type RunResult struct {
	SecuritiesFetched int
	PricesRecorded    int
	SnapshotsRecorded int
	FDPayoutsPosted   int
	FDsMatured        int
	FXRatesRecorded   int
	Errors            []provider.FetchError
	Duration          time.Duration
}

// Oracle fetches security prices from external providers and records them via the Kuberan API.
type Oracle struct {
	client    SecurityClient
	providers []provider.Provider
	rates     RateFetcher
	config    *config.Config
	logger    *slog.Logger
}

// NewOracle creates a new Oracle instance. rates may be nil, which disables FX
// rate publishing.
func NewOracle(client SecurityClient, providers []provider.Provider, rates RateFetcher, cfg *config.Config, logger *slog.Logger) *Oracle {
	return &Oracle{
		client:    client,
		providers: providers,
		rates:     rates,
		config:    cfg,
		logger:    logger,
	}
}

// symbolFor returns the symbol of a security for error reports, falling back to
// its ID when the security is not known to this run.
func symbolFor(symbols map[string]string, securityID string) string {
	if sym, ok := symbols[securityID]; ok {
		return sym
	}
	return "id:" + securityID
}

// normalizeAssetType converts an asset type string to the canonical lowercase form
// used by providers (e.g. "Stock" -> "stock", "Cryptocurrency" -> "crypto").
func normalizeAssetType(assetType string) string {
	t := strings.ToLower(assetType)
	if t == "cryptocurrency" {
		return "crypto"
	}
	return t
}

// fxSymbol builds the label used in FetchError for a currency pair.
func fxSymbol(base, quote string) string {
	return fmt.Sprintf("fx:%s/%s", base, quote)
}

// unionFXPairs returns the deduplicated, sorted union of the pipeline's configured
// pairs and, for every distinct provider-reported currency c: (c, b) for each base
// currency b. Currencies are upper-cased, and identity pairs (base == quote) and
// empty codes are dropped.
func unionFXPairs(configured []client.CurrencyPair, baseCurrencies []string, reported []string) []client.CurrencyPair {
	set := make(map[client.CurrencyPair]struct{})
	add := func(base, quote string) {
		b, q := strings.ToUpper(base), strings.ToUpper(quote)
		if b == "" || q == "" || b == q {
			return
		}
		set[client.CurrencyPair{Base: b, Quote: q}] = struct{}{}
	}

	for _, p := range configured {
		add(p.Base, p.Quote)
	}

	currencies := make(map[string]struct{})
	for _, c := range reported {
		if c = strings.ToUpper(c); c != "" {
			currencies[c] = struct{}{}
		}
	}
	for c := range currencies {
		for _, b := range baseCurrencies {
			add(c, b)
		}
	}

	pairs := make([]client.CurrencyPair, 0, len(set))
	for p := range set {
		pairs = append(pairs, p)
	}
	slices.SortFunc(pairs, func(a, b client.CurrencyPair) int {
		return cmp.Or(strings.Compare(a.Base, b.Base), strings.Compare(a.Quote, b.Quote))
	})
	return pairs
}

// UnionFXPairs is the exported form of unionFXPairs, used by the one-off
// subcommands so they select pairs exactly as the run path does.
func UnionFXPairs(configured []client.CurrencyPair, baseCurrencies []string, reported []string) []client.CurrencyPair {
	return unionFXPairs(configured, baseCurrencies, reported)
}

// publishFXRates fetches the rates for the union of needed pairs and posts them to
// the pipeline. Every failure is logged and recorded in result.Errors; none of them
// are returned, so the run continues and prices are still recorded.
func (o *Oracle) publishFXRates(ctx context.Context, start time.Time, results []provider.PriceResult, result *RunResult) {
	if o.rates == nil {
		o.logger.Warn("no rate fetcher configured, skipping fx rate publishing")
		return
	}

	fxPairs, err := o.client.GetFXPairs(ctx)
	if err != nil {
		o.logger.Warn("failed to fetch fx pairs, skipping fx rate publishing", "error", err)
		result.Errors = append(result.Errors, provider.FetchError{
			Symbol: "fx-pairs",
			Err:    fmt.Errorf("fetching fx pairs: %w", err),
		})
		return
	}

	reported := make([]string, 0, len(results))
	for _, r := range results {
		reported = append(reported, r.Currency)
	}
	pairs := unionFXPairs(fxPairs.Pairs, fxPairs.BaseCurrencies, reported)

	// One row per pair per UTC day: stamp the start of the day so every run
	// today upserts the same (base, quote, recorded_at) row with the latest
	// rate. Valuation takes the latest row <= the lookup time, and the
	// staleness window (FX_MAX_STALENESS, default 168h) is far above 24h.
	recordedAt := start.UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	entries := make([]client.FXRateEntry, 0, len(pairs))
	for _, p := range pairs {
		rate, err := o.rates.FetchRate(ctx, p.Base, p.Quote)
		if err != nil {
			o.logger.Warn("fx rate fetch failed, skipping pair", "base", p.Base, "quote", p.Quote, "error", err)
			result.Errors = append(result.Errors, provider.FetchError{
				Symbol: fxSymbol(p.Base, p.Quote),
				Err:    fmt.Errorf("fetching fx rate %s/%s: %w", p.Base, p.Quote, err),
			})
			continue
		}
		entries = append(entries, client.FXRateEntry{
			Base:       p.Base,
			Quote:      p.Quote,
			Rate:       strconv.FormatFloat(rate, 'f', 10, 64),
			RecordedAt: recordedAt,
			Source:     "yahoo",
		})
	}

	if len(entries) == 0 {
		o.logger.Info("no fx rates to publish", "pairs", len(pairs))
		return
	}

	recorded, err := o.client.RecordFXRates(ctx, entries)
	result.FXRatesRecorded = recorded
	if err != nil {
		o.logger.Warn("failed to record fx rates", "recorded", recorded, "attempted", len(entries), "error", err)
		result.Errors = append(result.Errors, provider.FetchError{
			Symbol: "fx-rates",
			Err:    fmt.Errorf("recording fx rates: %w", err),
		})
		return
	}
	o.logger.Info("fx rates published", "pairs", len(pairs), "recorded", recorded)
}

// Run executes a single oracle cycle: fetch securities, get prices, record results.
func (o *Oracle) Run(ctx context.Context) (*RunResult, error) {
	start := time.Now()
	result := &RunResult{}

	// 1. Fetch securities from Kuberan API.
	securities, err := o.client.GetSecurities(ctx)
	if err != nil {
		return nil, err
	}
	result.SecuritiesFetched = len(securities)

	// With no securities there are no prices to fetch, but FX rates (cash-only
	// multi-currency users), snapshots and fixed deposits still run below.
	if len(securities) == 0 {
		o.logger.Info("no securities found, skipping price fetch")
	}

	// 2. Convert to provider types.
	providerSecurities := make([]provider.Security, len(securities))
	for i, s := range securities {
		providerSecurities[i] = provider.Security{
			ID:             s.ID,
			Symbol:         s.Symbol,
			AssetType:      normalizeAssetType(s.AssetType),
			Exchange:       s.Exchange,
			ProviderSymbol: s.ProviderSymbol,
			Network:        s.Network,
			Currency:       s.Currency,
		}
	}

	// 3. Group by provider.
	groups := make(map[int][]provider.Security) // provider index -> securities
	for _, sec := range providerSecurities {
		matched := false
		for i, p := range o.providers {
			if p.Supports(sec.AssetType) {
				groups[i] = append(groups[i], sec)
				matched = true
				break
			}
		}
		if !matched {
			o.logger.Warn("no provider supports asset type", "symbol", sec.Symbol, "asset_type", sec.AssetType)
		}
	}

	// 4. Fetch prices from each provider concurrently.
	var mu sync.Mutex
	var allResults []provider.PriceResult
	var allErrors []provider.FetchError

	var wg sync.WaitGroup
	for i, secs := range groups {
		wg.Add(1)
		go func(p provider.Provider, securities []provider.Security) {
			defer wg.Done()
			o.logger.Info("fetching prices", "provider", p.Name(), "count", len(securities))
			prices, fetchErrors := p.FetchPrices(ctx, securities)
			mu.Lock()
			allResults = append(allResults, prices...)
			allErrors = append(allErrors, fetchErrors...)
			mu.Unlock()
		}(o.providers[i], secs)
	}
	wg.Wait()

	result.Errors = allErrors

	// Publish FX rates for the pairs the pipeline needs and for every currency
	// reported by the providers in this run. Failures are recorded in result.Errors
	// but never stop the run, so prices are still recorded.
	if o.config.PublishFX {
		o.publishFXRates(ctx, start, allResults, result)
	}

	// Prices are posted in the currency the provider reported; nothing is converted.
	// A price without a currency cannot be stored safely (the API would fall back to
	// the security's label), so it is reported and dropped.
	symbols := make(map[string]string, len(securities))
	for _, s := range securities {
		symbols[s.ID] = s.Symbol
	}
	priced := make([]provider.PriceResult, 0, len(allResults))
	for _, r := range allResults {
		if r.Currency == "" {
			result.Errors = append(result.Errors, provider.FetchError{
				SecurityID: r.SecurityID,
				Symbol:     symbolFor(symbols, r.SecurityID),
				Err:        errors.New("provider reported no currency for price"),
			})
			continue
		}
		priced = append(priced, r)
	}

	// 6. Record prices, if any. With no prices, snapshots and fixed deposits
	// still run.
	if len(priced) == 0 {
		o.logger.Info("no prices fetched")
	} else if err := o.recordPrices(ctx, priced, symbols, result); err != nil {
		return nil, err
	}

	// 7. Trigger snapshots if configured.
	if o.config.ComputeSnapshots {
		snapshots, err := o.client.ComputeSnapshots(ctx)
		if err != nil {
			o.logger.Warn("failed to compute snapshots", "error", err)
		} else {
			result.SnapshotsRecorded = snapshots
		}
	}

	// 8. Trigger fixed deposit processing if configured.
	if o.config.ProcessFixedDeposits {
		fdResult, err := o.client.ProcessFixedDeposits(ctx)
		if err != nil {
			o.logger.Warn("failed to process fixed deposits", "error", err)
		} else {
			result.FDPayoutsPosted = fdResult.PayoutsPosted
			result.FDsMatured = fdResult.FDsMatured
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// recordPrices posts the priced results to the pipeline. Rejected entries
// become fetch errors so the run summary counts them.
func (o *Oracle) recordPrices(ctx context.Context, priced []provider.PriceResult, symbols map[string]string, result *RunResult) error {
	entries := make([]client.RecordPriceEntry, len(priced))
	for i, r := range priced {
		entries[i] = client.RecordPriceEntry{
			SecurityID: r.SecurityID,
			Price:      r.Price,
			RecordedAt: r.RecordedAt.Format(time.RFC3339),
			Currency:   r.Currency,
		}
	}

	recorded, err := o.client.RecordPrices(ctx, entries)
	if err != nil {
		return err
	}
	result.PricesRecorded = recorded.Recorded
	for _, rej := range recorded.Rejected {
		o.logger.Warn("price rejected by pipeline",
			"security_id", rej.SecurityID,
			"currency", rej.Currency,
			"expected_currency", rej.ExpectedCurrency,
			"reason", rej.Reason,
		)
		result.Errors = append(result.Errors, provider.FetchError{
			SecurityID: rej.SecurityID,
			Symbol:     symbolFor(symbols, rej.SecurityID),
			Err: fmt.Errorf("price rejected: %s (posted %s, security is %s)",
				rej.Reason, rej.Currency, rej.ExpectedCurrency),
		})
	}
	return nil
}
