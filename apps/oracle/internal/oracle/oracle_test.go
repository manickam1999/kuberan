package oracle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/config"
	"github.com/kuberan/oracle/internal/provider"
)

// mockClient implements SecurityClient for testing. Every call is appended to
// calls so tests can assert the order of operations.
type mockClient struct {
	getSecuritiesFn        func(ctx context.Context) ([]client.Security, error)
	recordPricesFn         func(ctx context.Context, prices []client.RecordPriceEntry) (int, error)
	computeSnapshotsFn     func(ctx context.Context) (int, error)
	processFixedDepositsFn func(ctx context.Context) (client.ProcessFixedDepositsResult, error)
	getFXPairsFn           func(ctx context.Context) (client.FXPairs, error)
	recordFXRatesFn        func(ctx context.Context, rates []client.FXRateEntry) (int, error)
	calls                  []string
}

func (m *mockClient) GetSecurities(ctx context.Context) ([]client.Security, error) {
	m.calls = append(m.calls, "GetSecurities")
	return m.getSecuritiesFn(ctx)
}

func (m *mockClient) RecordPrices(ctx context.Context, prices []client.RecordPriceEntry) (int, error) {
	m.calls = append(m.calls, "RecordPrices")
	return m.recordPricesFn(ctx, prices)
}

func (m *mockClient) ComputeSnapshots(ctx context.Context) (int, error) {
	m.calls = append(m.calls, "ComputeSnapshots")
	return m.computeSnapshotsFn(ctx)
}

func (m *mockClient) ProcessFixedDeposits(ctx context.Context) (client.ProcessFixedDepositsResult, error) {
	m.calls = append(m.calls, "ProcessFixedDeposits")
	if m.processFixedDepositsFn != nil {
		return m.processFixedDepositsFn(ctx)
	}
	return client.ProcessFixedDepositsResult{}, nil
}

func (m *mockClient) GetFXPairs(ctx context.Context) (client.FXPairs, error) {
	m.calls = append(m.calls, "GetFXPairs")
	if m.getFXPairsFn != nil {
		return m.getFXPairsFn(ctx)
	}
	return client.FXPairs{Pairs: []client.CurrencyPair{}}, nil
}

func (m *mockClient) RecordFXRates(ctx context.Context, rates []client.FXRateEntry) (int, error) {
	m.calls = append(m.calls, "RecordFXRates")
	if m.recordFXRatesFn != nil {
		return m.recordFXRatesFn(ctx, rates)
	}
	return len(rates), nil
}

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name        string
	supports    func(assetType string) bool
	fetchPrices func(ctx context.Context, securities []provider.Security) ([]provider.PriceResult, []provider.FetchError)
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Supports(assetType string) bool { return m.supports(assetType) }

func (m *mockProvider) FetchPrices(ctx context.Context, securities []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
	return m.fetchPrices(ctx, securities)
}

// mockConverter implements CurrencyConverter for testing.
type mockConverter struct {
	target          string
	needsConversion func(fromCurrency string) bool
	convertFn       func(ctx context.Context, priceCents int64, fromCurrency string) (int64, error)
	fetchRateFn     func(ctx context.Context, base, quote string) (float64, error)
}

func (m *mockConverter) FetchRate(ctx context.Context, base, quote string) (float64, error) {
	if m.fetchRateFn == nil {
		return 0, errors.New("fetchRateFn not set")
	}
	return m.fetchRateFn(ctx, base, quote)
}

func (m *mockConverter) NeedsConversion(fromCurrency string) bool {
	return m.needsConversion(fromCurrency)
}

func (m *mockConverter) Convert(ctx context.Context, priceCents int64, fromCurrency string) (int64, error) {
	return m.convertFn(ctx, priceCents, fromCurrency)
}

func (m *mockConverter) TargetCurrency() string {
	return m.target
}

// newMYRConverter returns a mock converter that multiplies USD prices by 4.47
// and leaves MYR prices as-is.
func newMYRConverter() *mockConverter {
	return &mockConverter{
		target: "MYR",
		needsConversion: func(fromCurrency string) bool {
			return strings.ToUpper(fromCurrency) != "MYR"
		},
		convertFn: func(_ context.Context, priceCents int64, fromCurrency string) (int64, error) {
			if strings.ToUpper(fromCurrency) == "USD" {
				return int64(float64(priceCents) * 4.47), nil
			}
			return priceCents, nil
		},
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func defaultConfig(snapshots bool) *config.Config {
	return &config.Config{
		KuberanAPIURL:        "http://localhost:8080",
		PipelineAPIKey:       "test-key",
		RequestTimeout:       30 * time.Second,
		ComputeSnapshots:     snapshots,
		ProcessFixedDeposits: true,
		TargetCurrency:       "MYR",
	}
}

func defaultConfigNoFD(snapshots bool) *config.Config {
	cfg := defaultConfig(snapshots)
	cfg.ProcessFixedDeposits = false
	return cfg
}

func TestOracle_Run_FullFlow(t *testing.T) {
	now := time.Now().UTC()

	var recordedPrices []client.RecordPriceEntry
	snapshotsCalled := false

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD", Exchange: "NASDAQ"},
				{ID: "sec-2", Symbol: "MSFT", AssetType: "stock", Currency: "USD", Exchange: "NASDAQ"},
				{ID: "sec-3", Symbol: "CIMB", AssetType: "stock", Currency: "MYR", Exchange: "BURSA", ProviderSymbol: "1023.KL"},
				{ID: "sec-4", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
				{ID: "sec-5", Symbol: "ETH", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			snapshotsCalled = true
			return 3, nil
		},
	}

	// Yahoo returns native exchange currency: USD for NASDAQ, MYR for BURSA.
	var providerSymbolSeen bool
	yahooProvider := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" || at == "etf" || at == "reit" || at == "bond" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, len(secs))
			for i, s := range secs {
				if s.Symbol == "CIMB" && s.ProviderSymbol == "1023.KL" {
					providerSymbolSeen = true
				}
				// Simulate Yahoo returning native currency per exchange.
				currency := "USD"
				if s.Exchange == "BURSA" {
					currency = "MYR"
				}
				results[i] = provider.PriceResult{SecurityID: s.ID, Price: 10000 + int64(i+1)*100, Currency: currency, RecordedAt: now}
			}
			return results, nil
		},
	}

	// CoinGecko returns prices directly in MYR (target currency).
	geckoProvider := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, len(secs))
			for i, s := range secs {
				// Extract numeric ID from "sec-N" format for consistent test prices
				var idNum int64
				_, _ = fmt.Sscanf(s.ID, "sec-%d", &idNum)
				results[i] = provider.PriceResult{SecurityID: s.ID, Price: 500000 + idNum*1000, Currency: "MYR", RecordedAt: now}
			}
			return results, nil
		},
	}

	conv := newMYRConverter()
	orc := NewOracle(mc, []provider.Provider{yahooProvider, geckoProvider}, conv, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SecuritiesFetched != 5 {
		t.Errorf("SecuritiesFetched = %d, want 5", result.SecuritiesFetched)
	}
	if result.PricesRecorded != 5 {
		t.Errorf("PricesRecorded = %d, want 5", result.PricesRecorded)
	}
	if result.SnapshotsRecorded != 3 {
		t.Errorf("SnapshotsRecorded = %d, want 3", result.SnapshotsRecorded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %d, want 0", len(result.Errors))
	}
	if len(recordedPrices) != 5 {
		t.Errorf("recorded %d prices, want 5", len(recordedPrices))
	}
	if !snapshotsCalled {
		t.Error("ComputeSnapshots was not called")
	}
	if !providerSymbolSeen {
		t.Error("ProviderSymbol was not propagated to provider for CIMB security")
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}

	// Verify that USD prices were converted (multiplied by 4.47) and MYR prices were not.
	for _, p := range recordedPrices {
		switch p.SecurityID {
		case "sec-1": // AAPL (USD stock): original = 10000 + 1*100 = 10100 → 10100 * 4.47 = 45147
			if p.Price != 45147 {
				t.Errorf("AAPL price = %d, want 45147 (USD converted to MYR)", p.Price)
			}
		case "sec-2": // MSFT (USD stock): original = 10000 + 2*100 = 10200 → 10200 * 4.47 = 45594
			if p.Price != 45594 {
				t.Errorf("MSFT price = %d, want 45594 (USD converted to MYR)", p.Price)
			}
		case "sec-3": // CIMB (MYR stock): original = 10000 + 3*100 = 10300 → no conversion
			if p.Price != 10300 {
				t.Errorf("CIMB price = %d, want 10300 (MYR, no conversion)", p.Price)
			}
		case "sec-4": // BTC (MYR from CoinGecko): original = 500000 + 4*1000 = 504000 → no conversion
			if p.Price != 504000 {
				t.Errorf("BTC price = %d, want 504000 (MYR from CoinGecko, no conversion)", p.Price)
			}
		case "sec-5": // ETH (MYR from CoinGecko): original = 500000 + 5*1000 = 505000 → no conversion
			if p.Price != 505000 {
				t.Errorf("ETH price = %d, want 505000 (MYR from CoinGecko, no conversion)", p.Price)
			}
		}
	}
}

func TestOracle_Run_PartialProviderFailure(t *testing.T) {
	now := time.Now().UTC()

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
				{ID: "sec-2", Symbol: "MSFT", AssetType: "stock", Currency: "USD"},
				{ID: "sec-3", Symbol: "FAIL", AssetType: "stock", Currency: "USD"},
				{ID: "sec-4", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
				{ID: "sec-5", Symbol: "ETH", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 2, nil
		},
	}

	yahooProvider := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			var results []provider.PriceResult
			var errs []provider.FetchError
			for _, s := range secs {
				if s.Symbol == "FAIL" {
					errs = append(errs, provider.FetchError{SecurityID: s.ID, Symbol: s.Symbol, Err: errors.New("not found")})
				} else {
					results = append(results, provider.PriceResult{SecurityID: s.ID, Price: 10000, RecordedAt: now})
				}
			}
			return results, errs
		},
	}

	geckoProvider := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, len(secs))
			for i, s := range secs {
				results[i] = provider.PriceResult{SecurityID: s.ID, Price: 500000, RecordedAt: now}
			}
			return results, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{yahooProvider, geckoProvider}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PricesRecorded != 4 {
		t.Errorf("PricesRecorded = %d, want 4", result.PricesRecorded)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
	if len(result.Errors) > 0 && result.Errors[0].Symbol != "FAIL" {
		t.Errorf("error symbol = %q, want %q", result.Errors[0].Symbol, "FAIL")
	}
}

func TestOracle_Run_NoSecurities(t *testing.T) {
	providerCalled := false

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{}, nil
		},
		recordPricesFn: func(_ context.Context, _ []client.RecordPriceEntry) (int, error) {
			t.Error("RecordPrices should not be called")
			return 0, nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			t.Error("ComputeSnapshots should not be called")
			return 0, nil
		},
	}

	mp := &mockProvider{
		name:     "Test",
		supports: func(_ string) bool { return true },
		fetchPrices: func(_ context.Context, _ []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			providerCalled = true
			return nil, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SecuritiesFetched != 0 {
		t.Errorf("SecuritiesFetched = %d, want 0", result.SecuritiesFetched)
	}
	if result.PricesRecorded != 0 {
		t.Errorf("PricesRecorded = %d, want 0", result.PricesRecorded)
	}
	if providerCalled {
		t.Error("provider should not be called when no securities")
	}
}

func TestNormalizeAssetType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"stock", "stock"},
		{"Stock", "stock"},
		{"STOCK", "stock"},
		{"etf", "etf"},
		{"ETF", "etf"},
		{"crypto", "crypto"},
		{"Cryptocurrency", "crypto"},
		{"CRYPTOCURRENCY", "crypto"},
		{"bond", "bond"},
		{"Bond", "bond"},
		{"reit", "reit"},
		{"REIT", "reit"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAssetType(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAssetType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOracle_Run_MixedCaseAssetTypes(t *testing.T) {
	now := time.Now().UTC()

	var recordedPrices []client.RecordPriceEntry

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "Stock", Currency: "USD"},
				{ID: "sec-2", Symbol: "VWRA", AssetType: "ETF", Currency: "USD"},
				{ID: "sec-3", Symbol: "BTC", AssetType: "Cryptocurrency", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 1, nil
		},
	}

	yahooProvider := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" || at == "etf" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, len(secs))
			for i, s := range secs {
				results[i] = provider.PriceResult{SecurityID: s.ID, Price: 10000, RecordedAt: now}
			}
			return results, nil
		},
	}

	geckoProvider := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, len(secs))
			for i, s := range secs {
				results[i] = provider.PriceResult{SecurityID: s.ID, Price: 500000, RecordedAt: now}
			}
			return results, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{yahooProvider, geckoProvider}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SecuritiesFetched != 3 {
		t.Errorf("SecuritiesFetched = %d, want 3", result.SecuritiesFetched)
	}
	if result.PricesRecorded != 3 {
		t.Errorf("PricesRecorded = %d, want 3", result.PricesRecorded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %d, want 0", len(result.Errors))
	}
	if len(recordedPrices) != 3 {
		t.Errorf("recorded %d prices, want 3", len(recordedPrices))
	}
}

func TestOracle_Run_UnsupportedAssetType(t *testing.T) {
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "BOND1", AssetType: "bond", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, _ []client.RecordPriceEntry) (int, error) {
			t.Error("RecordPrices should not be called when no prices fetched")
			return 0, nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			t.Error("ComputeSnapshots should not be called when no prices fetched")
			return 0, nil
		},
	}

	// Provider that only supports crypto — bond won't match.
	cryptoOnly := &mockProvider{
		name:     "CryptoOnly",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, _ []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			t.Error("FetchPrices should not be called for unsupported asset type")
			return nil, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{cryptoOnly}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SecuritiesFetched != 1 {
		t.Errorf("SecuritiesFetched = %d, want 1", result.SecuritiesFetched)
	}
	if result.PricesRecorded != 0 {
		t.Errorf("PricesRecorded = %d, want 0", result.PricesRecorded)
	}
}

func TestOracle_Run_GetSecuritiesFails(t *testing.T) {
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return nil, errors.New("connection refused")
		},
		recordPricesFn: func(_ context.Context, _ []client.RecordPriceEntry) (int, error) {
			return 0, nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 0, nil
		},
	}

	orc := NewOracle(mc, nil, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if err.Error() != "connection refused" {
		t.Errorf("error = %q, want %q", err.Error(), "connection refused")
	}
}

func TestOracle_Run_RecordPricesFails(t *testing.T) {
	now := time.Now().UTC()

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, _ []client.RecordPriceEntry) (int, error) {
			return 0, errors.New("server error")
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			t.Error("ComputeSnapshots should not be called when RecordPrices fails")
			return 0, nil
		},
	}

	mp := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 17800, RecordedAt: now},
			}, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if err.Error() != "server error" {
		t.Errorf("error = %q, want %q", err.Error(), "server error")
	}
}

func TestOracle_Run_SnapshotFailureNonFatal(t *testing.T) {
	now := time.Now().UTC()

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 0, errors.New("snapshot service unavailable")
		},
	}

	mp := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 17800, RecordedAt: now},
			}, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PricesRecorded != 1 {
		t.Errorf("PricesRecorded = %d, want 1", result.PricesRecorded)
	}
	if result.SnapshotsRecorded != 0 {
		t.Errorf("SnapshotsRecorded = %d, want 0", result.SnapshotsRecorded)
	}
}

func TestOracle_Run_SnapshotsDisabled(t *testing.T) {
	now := time.Now().UTC()
	snapshotsCalled := false

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			snapshotsCalled = true
			return 1, nil
		},
	}

	mp := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 6700000, RecordedAt: now},
			}, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfigNoFD(false), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshotsCalled {
		t.Error("ComputeSnapshots should not be called when disabled")
	}
	if result.PricesRecorded != 1 {
		t.Errorf("PricesRecorded = %d, want 1", result.PricesRecorded)
	}
	if result.SnapshotsRecorded != 0 {
		t.Errorf("SnapshotsRecorded = %d, want 0", result.SnapshotsRecorded)
	}
}

func TestOracle_Run_ProcessFixedDeposits(t *testing.T) {
	now := time.Now().UTC()
	fdCalled := false

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 1, nil
		},
		processFixedDepositsFn: func(_ context.Context) (client.ProcessFixedDepositsResult, error) {
			fdCalled = true
			return client.ProcessFixedDepositsResult{PayoutsPosted: 3, FDsMatured: 1}, nil
		},
	}

	mp := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 6700000, RecordedAt: now},
			}, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fdCalled {
		t.Error("ProcessFixedDeposits was not called")
	}
	if result.FDPayoutsPosted != 3 {
		t.Errorf("FDPayoutsPosted = %d, want 3", result.FDPayoutsPosted)
	}
	if result.FDsMatured != 1 {
		t.Errorf("FDsMatured = %d, want 1", result.FDsMatured)
	}
}

func TestOracle_Run_ProcessFixedDeposits_Disabled(t *testing.T) {
	now := time.Now().UTC()
	fdCalled := false

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 1, nil
		},
		processFixedDepositsFn: func(_ context.Context) (client.ProcessFixedDepositsResult, error) {
			fdCalled = true
			return client.ProcessFixedDepositsResult{}, nil
		},
	}

	mp := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 6700000, RecordedAt: now},
			}, nil
		},
	}

	cfg := defaultConfig(true)
	cfg.ProcessFixedDeposits = false
	orc := NewOracle(mc, []provider.Provider{mp}, nil, cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fdCalled {
		t.Error("ProcessFixedDeposits should not be called when disabled")
	}
	if result.FDPayoutsPosted != 0 {
		t.Errorf("FDPayoutsPosted = %d, want 0", result.FDPayoutsPosted)
	}
}

func TestOracle_Run_ProcessFixedDeposits_FailureNonFatal(t *testing.T) {
	now := time.Now().UTC()

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "BTC", AssetType: "crypto", Currency: "USD"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			return 1, nil
		},
		processFixedDepositsFn: func(_ context.Context) (client.ProcessFixedDepositsResult, error) {
			return client.ProcessFixedDepositsResult{}, errors.New("fd service unavailable")
		},
	}

	mp := &mockProvider{
		name:     "CoinGecko",
		supports: func(at string) bool { return at == "crypto" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return []provider.PriceResult{
				{SecurityID: secs[0].ID, Price: 6700000, RecordedAt: now},
			}, nil
		},
	}

	orc := NewOracle(mc, []provider.Provider{mp}, nil, defaultConfig(true), newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("FD failure should be non-fatal, got error: %v", err)
	}

	if result.PricesRecorded != 1 {
		t.Errorf("PricesRecorded = %d, want 1", result.PricesRecorded)
	}
	if result.FDPayoutsPosted != 0 {
		t.Errorf("FDPayoutsPosted = %d, want 0 on error", result.FDPayoutsPosted)
	}
}

// callIndex returns the position of name in calls, or -1 when absent.
func callIndex(calls []string, name string) int {
	return slices.Index(calls, name)
}

// fakeYahooTransport answers Yahoo chart requests from a fixed rate table and
// counts the requests made for each ticker. It lets the real ForexConverter run
// without network access.
type fakeYahooTransport struct {
	rates    map[string]float64
	requests map[string]int
}

func newFakeYahooTransport(rates map[string]float64) *fakeYahooTransport {
	return &fakeYahooTransport{rates: rates, requests: make(map[string]int)}
}

func (f *fakeYahooTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ticker := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	f.requests[ticker]++

	body := fmt.Sprintf(`{"chart":{"result":null,"error":{"code":"Not Found","description":"No data found for %s"}}}`, ticker)
	if rate, ok := f.rates[ticker]; ok {
		body = fmt.Sprintf(`{"chart":{"result":[{"meta":{"regularMarketPrice":%v}}],"error":null}}`, rate)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}, nil
}

// newFXTestConverter returns a real ForexConverter (target MYR) backed by the fake transport.
func newFXTestConverter(transport http.RoundTripper) *provider.ForexConverter {
	return provider.NewForexConverter(&http.Client{Transport: transport}, "MYR")
}

// currencyStockProvider prices every stock at 10000 cents in the currency given
// for its security ID.
func currencyStockProvider(currencyByID map[string]string, now time.Time) *mockProvider {
	return &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" },
		fetchPrices: func(_ context.Context, secs []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			results := make([]provider.PriceResult, 0, len(secs))
			for _, s := range secs {
				results = append(results, provider.PriceResult{
					SecurityID: s.ID,
					Price:      10000,
					Currency:   currencyByID[s.ID],
					RecordedAt: now,
				})
			}
			return results, nil
		},
	}
}

func TestUnionFXPairs(t *testing.T) {
	tests := []struct {
		name       string
		configured []client.CurrencyPair
		bases      []string
		reported   []string
		target     string
		want       []client.CurrencyPair
	}{
		{
			name:       "configured pairs only",
			configured: []client.CurrencyPair{{Base: "USD", Quote: "MYR"}},
			bases:      []string{"MYR"},
			target:     "MYR",
			want:       []client.CurrencyPair{{Base: "USD", Quote: "MYR"}},
		},
		{
			name:     "reported currency pairs with every base and the target",
			bases:    []string{"MYR", "SGD"},
			reported: []string{"USD"},
			target:   "MYR",
			want:     []client.CurrencyPair{{Base: "USD", Quote: "MYR"}, {Base: "USD", Quote: "SGD"}},
		},
		{
			name:       "duplicates, case and empty codes are normalised and deduplicated",
			configured: []client.CurrencyPair{{Base: "usd", Quote: "myr"}},
			bases:      []string{"myr"},
			reported:   []string{"USD", "usd", ""},
			target:     "myr",
			want:       []client.CurrencyPair{{Base: "USD", Quote: "MYR"}},
		},
		{
			name:     "identity pairs are dropped",
			bases:    []string{"MYR"},
			reported: []string{"MYR"},
			target:   "MYR",
			want:     nil,
		},
		{
			name:     "reported target currency still pairs with other bases",
			bases:    []string{"SGD"},
			reported: []string{"MYR"},
			target:   "MYR",
			want:     []client.CurrencyPair{{Base: "MYR", Quote: "SGD"}},
		},
		{
			name:       "output is sorted by base then quote",
			configured: []client.CurrencyPair{{Base: "USD", Quote: "SGD"}, {Base: "GBP", Quote: "MYR"}},
			bases:      []string{"MYR"},
			reported:   []string{"EUR"},
			target:     "MYR",
			want: []client.CurrencyPair{
				{Base: "EUR", Quote: "MYR"},
				{Base: "GBP", Quote: "MYR"},
				{Base: "USD", Quote: "SGD"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unionFXPairs(tt.configured, tt.bases, tt.reported, tt.target)
			if !slices.Equal(got, tt.want) {
				t.Errorf("unionFXPairs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOracle_Run_PublishesFXBeforePricesAndSnapshots(t *testing.T) {
	now := time.Now().UTC()
	currencies := map[string]string{"sec-1": "USD", "sec-2": "SGD"}

	var fxEntries []client.FXRateEntry
	var recordedPrices []client.RecordPriceEntry
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
				{ID: "sec-2", Symbol: "D05", AssetType: "stock", Currency: "SGD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{BaseCurrencies: []string{"MYR"}}, nil
		},
		recordFXRatesFn: func(_ context.Context, rates []client.FXRateEntry) (int, error) {
			fxEntries = rates
			return len(rates), nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) { return 0, nil },
	}

	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47, "SGDMYR=X": 3.32})
	cfg := defaultConfig(true)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(currencies, now)}, newFXTestConverter(transport), cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Rates must be posted after the provider fetch and before prices and snapshots.
	fxIdx := callIndex(mc.calls, "RecordFXRates")
	fxPairsIdx := callIndex(mc.calls, "GetFXPairs")
	priceIdx := callIndex(mc.calls, "RecordPrices")
	snapIdx := callIndex(mc.calls, "ComputeSnapshots")
	if fxPairsIdx < 0 || fxIdx < 0 || priceIdx < 0 || snapIdx < 0 {
		t.Fatalf("missing calls, got order %v", mc.calls)
	}
	if fxPairsIdx >= fxIdx || fxIdx >= priceIdx || priceIdx >= snapIdx {
		t.Errorf("call order = %v, want GetFXPairs < RecordFXRates < RecordPrices < ComputeSnapshots", mc.calls)
	}

	if len(fxEntries) != 2 {
		t.Fatalf("posted %d fx rates, want 2: %+v", len(fxEntries), fxEntries)
	}
	if fxEntries[0].Base != "SGD" || fxEntries[0].Quote != "MYR" || fxEntries[0].Rate != "3.3200000000" {
		t.Errorf("SGD/MYR entry = %+v", fxEntries[0])
	}
	if fxEntries[1].Base != "USD" || fxEntries[1].Quote != "MYR" || fxEntries[1].Rate != "4.4700000000" {
		t.Errorf("USD/MYR entry = %+v", fxEntries[1])
	}
	for _, e := range fxEntries {
		if e.Source != "yahoo" {
			t.Errorf("source = %q, want yahoo", e.Source)
		}
		recordedAt, err := time.Parse(time.RFC3339, e.RecordedAt)
		if err != nil {
			t.Fatalf("recorded_at %q is not RFC3339: %v", e.RecordedAt, err)
		}
		if recordedAt.Second() != 0 || recordedAt.Nanosecond() != 0 || recordedAt.Location() != time.UTC {
			t.Errorf("recorded_at = %v, want a UTC time truncated to the minute", recordedAt)
		}
		if time.Since(recordedAt) > 2*time.Minute {
			t.Errorf("recorded_at = %v is not this run's start time", recordedAt)
		}
	}

	if result.FXRatesRecorded != 2 {
		t.Errorf("FXRatesRecorded = %d, want 2", result.FXRatesRecorded)
	}
	if result.PricesRecorded != 2 || len(recordedPrices) != 2 {
		t.Errorf("PricesRecorded = %d (%d entries), want 2", result.PricesRecorded, len(recordedPrices))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %+v, want none", result.Errors)
	}
}

func TestOracle_Run_FXRatesFetchedOncePerPair(t *testing.T) {
	now := time.Now().UTC()
	currencies := map[string]string{"sec-1": "USD", "sec-2": "SGD"}

	var recordedPrices []client.RecordPriceEntry
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
				{ID: "sec-2", Symbol: "D05", AssetType: "stock", Currency: "SGD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{
				Pairs:          []client.CurrencyPair{{Base: "USD", Quote: "MYR"}},
				BaseCurrencies: []string{"MYR"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) { return 0, nil },
	}

	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47, "SGDMYR=X": 3.32})
	cfg := defaultConfig(true)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(currencies, now)}, newFXTestConverter(transport), cfg, newTestLogger())
	if _, err := orc.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Publishing and conversion share the converter cache: one Yahoo request per pair.
	for _, ticker := range []string{"USDMYR=X", "SGDMYR=X"} {
		if transport.requests[ticker] != 1 {
			t.Errorf("%s requested %d times, want 1", ticker, transport.requests[ticker])
		}
	}
	if len(transport.requests) != 2 {
		t.Errorf("unexpected tickers requested: %v", transport.requests)
	}

	// Step 5b still converts with the same rates as before.
	prices := map[string]int64{}
	for _, p := range recordedPrices {
		prices[p.SecurityID] = p.Price
	}
	if prices["sec-1"] != 44700 {
		t.Errorf("USD price = %d, want 44700", prices["sec-1"])
	}
	if prices["sec-2"] != 33200 {
		t.Errorf("SGD price = %d, want 33200", prices["sec-2"])
	}
}

func TestOracle_Run_FXRecordFailureDoesNotStopPrices(t *testing.T) {
	now := time.Now().UTC()

	var recordedPrices []client.RecordPriceEntry
	snapshotsCalled := false
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{
				Pairs:          []client.CurrencyPair{{Base: "USD", Quote: "MYR"}, {Base: "USD", Quote: "SGD"}},
				BaseCurrencies: []string{"MYR"},
			}, nil
		},
		recordFXRatesFn: func(_ context.Context, _ []client.FXRateEntry) (int, error) {
			return 1, errors.New("pipeline unavailable")
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) {
			snapshotsCalled = true
			return 1, nil
		},
	}

	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47, "USDSGD=X": 1.33})
	cfg := defaultConfig(true)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(map[string]string{"sec-1": "USD"}, now)}, newFXTestConverter(transport), cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("FX failure must not fail the run, got: %v", err)
	}
	if len(recordedPrices) != 1 || result.PricesRecorded != 1 {
		t.Errorf("prices recorded = %d (result %d), want 1", len(recordedPrices), result.PricesRecorded)
	}
	if !snapshotsCalled {
		t.Error("ComputeSnapshots should still run after an FX failure")
	}
	if result.FXRatesRecorded != 1 {
		t.Errorf("FXRatesRecorded = %d, want the partial count 1", result.FXRatesRecorded)
	}

	var found bool
	for _, e := range result.Errors {
		if e.Symbol == "fx-rates" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an fx-rates entry in Errors, got %+v", result.Errors)
	}
}

func TestOracle_Run_GetFXPairsFailureDoesNotStopPrices(t *testing.T) {
	now := time.Now().UTC()

	var recordedPrices []client.RecordPriceEntry
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{}, errors.New("fx-pairs endpoint down")
		},
		recordFXRatesFn: func(_ context.Context, _ []client.FXRateEntry) (int, error) {
			t.Error("RecordFXRates must not be called when the pairs cannot be fetched")
			return 0, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) { return 0, nil },
	}

	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47})
	cfg := defaultConfig(false)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(map[string]string{"sec-1": "USD"}, now)}, newFXTestConverter(transport), cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recordedPrices) != 1 {
		t.Errorf("prices recorded = %d, want 1", len(recordedPrices))
	}
	var found bool
	for _, e := range result.Errors {
		if e.Symbol == "fx-pairs" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an fx-pairs entry in Errors, got %+v", result.Errors)
	}
}

func TestOracle_Run_FXPairFetchFailureSkipsOnlyThatPair(t *testing.T) {
	now := time.Now().UTC()
	currencies := map[string]string{"sec-1": "USD", "sec-2": "SGD"}

	var fxEntries []client.FXRateEntry
	var recordedPrices []client.RecordPriceEntry
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
				{ID: "sec-2", Symbol: "D05", AssetType: "stock", Currency: "SGD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{BaseCurrencies: []string{"MYR"}}, nil
		},
		recordFXRatesFn: func(_ context.Context, rates []client.FXRateEntry) (int, error) {
			fxEntries = rates
			return len(rates), nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			recordedPrices = prices
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) { return 0, nil },
	}

	// SGDMYR=X is unknown to the fake Yahoo, so only that pair fails to fetch.
	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47})
	cfg := defaultConfig(false)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(currencies, now)}, newFXTestConverter(transport), cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fxEntries) != 1 || fxEntries[0].Base != "USD" {
		t.Errorf("posted fx entries = %+v, want only USD/MYR", fxEntries)
	}

	var fxErr bool
	for _, e := range result.Errors {
		if e.Symbol == "fx:SGD/MYR" {
			fxErr = true
		}
	}
	if !fxErr {
		t.Errorf("expected fx:SGD/MYR in Errors, got %+v", result.Errors)
	}

	// The SGD price is skipped by the unchanged conversion step; the USD price is recorded.
	if len(recordedPrices) != 1 || recordedPrices[0].SecurityID != "sec-1" {
		t.Errorf("recorded prices = %+v, want only sec-1", recordedPrices)
	}
}

func TestOracle_Run_PublishFXDisabled(t *testing.T) {
	now := time.Now().UTC()

	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			t.Error("GetFXPairs must not be called when PublishFX is false")
			return client.FXPairs{}, nil
		},
		recordPricesFn: func(_ context.Context, prices []client.RecordPriceEntry) (int, error) {
			return len(prices), nil
		},
		computeSnapshotsFn: func(_ context.Context) (int, error) { return 0, nil },
	}

	conv := newMYRConverter()
	conv.fetchRateFn = func(_ context.Context, _, _ string) (float64, error) {
		t.Error("FetchRate must not be called when PublishFX is false")
		return 0, nil
	}

	cfg := defaultConfig(false)
	cfg.PublishFX = false

	orc := NewOracle(mc, []provider.Provider{currencyStockProvider(map[string]string{"sec-1": "USD"}, now)}, conv, cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FXRatesRecorded != 0 {
		t.Errorf("FXRatesRecorded = %d, want 0", result.FXRatesRecorded)
	}
	if result.PricesRecorded != 1 {
		t.Errorf("PricesRecorded = %d, want 1", result.PricesRecorded)
	}
}

func TestOracle_Run_FXPublishedWhenNoProvidersReturnPrices(t *testing.T) {
	mc := &mockClient{
		getSecuritiesFn: func(_ context.Context) ([]client.Security, error) {
			return []client.Security{
				{ID: "sec-1", Symbol: "AAPL", AssetType: "stock", Currency: "USD"},
			}, nil
		},
		getFXPairsFn: func(_ context.Context) (client.FXPairs, error) {
			return client.FXPairs{
				Pairs:          []client.CurrencyPair{{Base: "USD", Quote: "MYR"}},
				BaseCurrencies: []string{"MYR"},
			}, nil
		},
		recordPricesFn: func(_ context.Context, _ []client.RecordPriceEntry) (int, error) {
			t.Error("RecordPrices must not be called when no prices were fetched")
			return 0, nil
		},
	}

	// The provider returns nothing, so the pairs come only from the pipeline's own list.
	empty := &mockProvider{
		name:     "Yahoo Finance",
		supports: func(at string) bool { return at == "stock" },
		fetchPrices: func(_ context.Context, _ []provider.Security) ([]provider.PriceResult, []provider.FetchError) {
			return nil, nil
		},
	}

	transport := newFakeYahooTransport(map[string]float64{"USDMYR=X": 4.47})
	cfg := defaultConfig(false)
	cfg.PublishFX = true

	orc := NewOracle(mc, []provider.Provider{empty}, newFXTestConverter(transport), cfg, newTestLogger())
	result, err := orc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FXRatesRecorded != 1 {
		t.Errorf("FXRatesRecorded = %d, want 1", result.FXRatesRecorded)
	}
	if callIndex(mc.calls, "RecordFXRates") < 0 {
		t.Errorf("RecordFXRates not called, calls = %v", mc.calls)
	}
}
