package oracle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/config"
	"github.com/kuberan/oracle/internal/provider"
)

// mockClient implements SecurityClient for testing.
type mockClient struct {
	getSecuritiesFn        func(ctx context.Context) ([]client.Security, error)
	recordPricesFn         func(ctx context.Context, prices []client.RecordPriceEntry) (int, error)
	computeSnapshotsFn     func(ctx context.Context) (int, error)
	processFixedDepositsFn func(ctx context.Context) (client.ProcessFixedDepositsResult, error)
}

func (m *mockClient) GetSecurities(ctx context.Context) ([]client.Security, error) {
	return m.getSecuritiesFn(ctx)
}

func (m *mockClient) RecordPrices(ctx context.Context, prices []client.RecordPriceEntry) (int, error) {
	return m.recordPricesFn(ctx, prices)
}

func (m *mockClient) ComputeSnapshots(ctx context.Context) (int, error) {
	return m.computeSnapshotsFn(ctx)
}

func (m *mockClient) ProcessFixedDeposits(ctx context.Context) (client.ProcessFixedDepositsResult, error) {
	if m.processFixedDepositsFn != nil {
		return m.processFixedDepositsFn(ctx)
	}
	return client.ProcessFixedDepositsResult{}, nil
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
