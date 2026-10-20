// Package oracle orchestrates fetching prices from providers and pushing them to the Kuberan API.
package oracle

import (
	"context"
	"fmt"
	"log/slog"
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
	RecordPrices(ctx context.Context, prices []client.RecordPriceEntry) (int, error)
	ComputeSnapshots(ctx context.Context) (int, error)
	ProcessFixedDeposits(ctx context.Context) (client.ProcessFixedDepositsResult, error)
}

// CurrencyConverter converts prices from one currency to the target currency.
type CurrencyConverter interface {
	// NeedsConversion returns true if the given currency differs from the target.
	NeedsConversion(fromCurrency string) bool
	// Convert converts a price in cents from the given currency to the target currency cents.
	Convert(ctx context.Context, priceCents int64, fromCurrency string) (int64, error)
	// TargetCurrency returns the target currency code (e.g. "MYR").
	TargetCurrency() string
}

// RunResult contains the outcome of an oracle run.
type RunResult struct {
	SecuritiesFetched int
	PricesRecorded    int
	SnapshotsRecorded int
	FDPayoutsPosted   int
	FDsMatured        int
	Errors            []provider.FetchError
	Duration          time.Duration
}

// Oracle fetches security prices from external providers and records them via the Kuberan API.
type Oracle struct {
	client    SecurityClient
	providers []provider.Provider
	converter CurrencyConverter
	config    *config.Config
	logger    *slog.Logger
}

// NewOracle creates a new Oracle instance.
func NewOracle(client SecurityClient, providers []provider.Provider, converter CurrencyConverter, cfg *config.Config, logger *slog.Logger) *Oracle {
	return &Oracle{
		client:    client,
		providers: providers,
		converter: converter,
		config:    cfg,
		logger:    logger,
	}
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

	if len(securities) == 0 {
		o.logger.Info("no securities found, nothing to do")
		result.Duration = time.Since(start)
		return result, nil
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

	// 5. If no prices fetched, return early.
	if len(allResults) == 0 {
		o.logger.Info("no prices fetched")
		result.Duration = time.Since(start)
		return result, nil
	}

	// 5b. Convert all prices to the target currency using the currency
	// reported by each data source (e.g. Yahoo returns "USD" for NASDAQ stocks).
	var convertedResults []provider.PriceResult
	for _, r := range allResults {
		if o.converter != nil && o.converter.NeedsConversion(r.Currency) {
			converted, err := o.converter.Convert(ctx, r.Price, r.Currency)
			if err != nil {
				o.logger.Warn("currency conversion failed, skipping",
					"security_id", r.SecurityID,
					"currency", r.Currency,
					"target", o.converter.TargetCurrency(),
					"error", err,
				)
				result.Errors = append(result.Errors, provider.FetchError{
					SecurityID: r.SecurityID,
					Symbol:     fmt.Sprintf("id:%s", r.SecurityID),
					Err:        fmt.Errorf("currency conversion from %s to %s: %w", r.Currency, o.converter.TargetCurrency(), err),
				})
				continue
			}
			o.logger.Debug("converted price",
				"security_id", r.SecurityID,
				"from_currency", r.Currency,
				"original_cents", r.Price,
				"converted_cents", converted,
			)
			r.Price = converted
		}
		convertedResults = append(convertedResults, r)
	}

	if len(convertedResults) == 0 {
		o.logger.Info("no prices after conversion")
		result.Duration = time.Since(start)
		return result, nil
	}

	// 6. Convert to client price entries and record.
	entries := make([]client.RecordPriceEntry, len(convertedResults))
	for i, r := range convertedResults {
		entries[i] = client.RecordPriceEntry{
			SecurityID: r.SecurityID,
			Price:      r.Price,
			RecordedAt: r.RecordedAt.Format(time.RFC3339),
		}
	}

	recorded, err := o.client.RecordPrices(ctx, entries)
	if err != nil {
		return nil, err
	}
	result.PricesRecorded = recorded

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
