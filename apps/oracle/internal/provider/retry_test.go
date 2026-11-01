package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Keep retry backoff negligible so tests of failing providers stay fast.
	retryBaseDelay = time.Millisecond
	retryMaxDelay = 5 * time.Millisecond
	os.Exit(m.Run())
}

func newGetRequest(t *testing.T, url string) func() (*http.Request, error) {
	t.Helper()
	return func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	}
}

func TestDoWithRetry_RetriesTransientStatusThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	resp, err := doWithRetry(context.Background(), server.Client(), newGetRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestDoWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resp, err := doWithRetry(context.Background(), server.Client(), newGetRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	if got := int(calls.Load()); got != retryMaxAttempts {
		t.Errorf("calls = %d, want %d", got, retryMaxAttempts)
	}
}

func TestDoWithRetry_DoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	resp, err := doWithRetry(context.Background(), server.Client(), newGetRequest(t, server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestBackoffDelay_Bounded(t *testing.T) {
	for attempt := 1; attempt <= 10; attempt++ {
		d := backoffDelay(attempt)
		if d <= 0 || d > retryMaxDelay {
			t.Errorf("attempt %d: delay %v out of (0, %v]", attempt, d, retryMaxDelay)
		}
	}
}

func TestCoinGeckoFetchPrices_ChunksIDs(t *testing.T) {
	var calls atomic.Int32
	var maxIDs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		if n := int32(len(ids)); n > maxIDs.Load() {
			maxIDs.Store(n)
		}
		resp := map[string]map[string]float64{}
		for _, id := range ids {
			resp[id] = map[string]float64{"myr": 1.5}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	orig := coinGeckoChunkSize
	coinGeckoChunkSize = 1
	defer func() { coinGeckoChunkSize = orig }()

	secs := []Security{
		{ID: "a", Symbol: "BTC", AssetType: "crypto"},
		{ID: "b", Symbol: "ETH", AssetType: "crypto"},
	}
	p := &CoinGeckoProvider{httpClient: server.Client(), baseURL: server.URL, targetCurrency: "myr"}
	prices, errs := p.FetchPrices(context.Background(), secs)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(prices) != 2 {
		t.Fatalf("prices = %d, want 2", len(prices))
	}
	if calls.Load() != 2 || maxIDs.Load() != 1 {
		t.Errorf("calls = %d (want 2), max ids per call = %d (want 1)", calls.Load(), maxIDs.Load())
	}
}
