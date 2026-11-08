package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/provider"
)

const testKey = "super-secret-key"

// recordedReq is one request the fake API received.
type recordedReq struct {
	Method string
	Path   string
	Body   map[string]any
}

// fakeAPI is an httptest Kuberan API that records every request in order.
type fakeAPI struct {
	srv        *httptest.Server
	mu         sync.Mutex
	reqs       []recordedReq
	securities []client.Security
	fxPairs    client.FXPairs
	patchCode  map[string]int // security id -> forced PATCH status
	pricesDel  int
	// rejectCurrency makes the prices endpoint reject entries whose currency is not this.
	rejectCurrency string
}

func newFakeAPI(t *testing.T, secs []client.Security, pairs client.FXPairs) *fakeAPI {
	t.Helper()
	f := &fakeAPI{securities: secs, fxPairs: pairs, patchCode: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	raw, _ := io.ReadAll(r.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedReq{r.Method, r.URL.Path, body})
	f.mu.Unlock()

	if r.Header.Get("X-API-Key") != testKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/pipeline/securities":
		_ = json.NewEncoder(w).Encode(map[string]any{"securities": f.securities})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/pipeline/fx-pairs":
		_ = json.NewEncoder(w).Encode(f.fxPairs)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/pipeline/securities/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/pipeline/securities/")
		if code := f.patchCode[id]; code != 0 {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"code":"SECURITY_CURRENCY_LOCKED"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"security": map[string]any{"id": id}, "prices_deleted": f.pricesDel})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pipeline/securities/prices":
		prices, _ := body["prices"].([]any)
		recorded := 0
		rejected := []map[string]any{}
		for i, p := range prices {
			e := p.(map[string]any)
			if f.rejectCurrency != "" && e["currency"] != f.rejectCurrency {
				rejected = append(rejected, map[string]any{
					"index": i, "security_id": e["security_id"], "currency": e["currency"],
					"expected_currency": f.rejectCurrency, "reason": "currency_mismatch",
				})
				continue
			}
			recorded++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"recorded": recorded, "rejected": rejected})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pipeline/fx-rates":
		rates, _ := body["rates"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"recorded": len(rates)})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeAPI) requests() []recordedReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedReq(nil), f.reqs...)
}

// writes returns only the mutating requests (POST/PATCH).
func (f *fakeAPI) writes() []recordedReq {
	var out []recordedReq
	for _, r := range f.requests() {
		if r.Method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

type fakeYahoo struct {
	currency map[string]string // ticker -> currency
	hist     map[string]*provider.History
	errs     map[string]error
	mu       sync.Mutex
	histCall []string
	ranges   [][2]time.Time
}

func (y *fakeYahoo) Supports(t string) bool {
	switch t {
	case "stock", "etf", "bond", "reit", "commodity":
		return true
	}
	return false
}

func (y *fakeYahoo) FetchCurrency(_ context.Context, ticker string) (string, error) {
	if err := y.errs[ticker]; err != nil {
		return "", err
	}
	return y.currency[ticker], nil
}

func (y *fakeYahoo) FetchHistory(_ context.Context, ticker string, from, to time.Time) (*provider.History, error) {
	y.mu.Lock()
	y.histCall = append(y.histCall, ticker)
	y.ranges = append(y.ranges, [2]time.Time{from, to})
	y.mu.Unlock()
	if err := y.errs[ticker]; err != nil {
		return nil, err
	}
	return y.hist[ticker], nil
}

type fakeFX struct {
	hist map[string]*provider.History // "USDMYR" -> history
	errs map[string]error
	mu   sync.Mutex
	got  []string
}

func (f *fakeFX) FetchHistory(_ context.Context, base, quote string, _, _ time.Time) (*provider.History, error) {
	key := base + quote
	f.mu.Lock()
	f.got = append(f.got, key)
	f.mu.Unlock()
	if err := f.errs[key]; err != nil {
		return nil, err
	}
	if h, ok := f.hist[key]; ok {
		return h, nil
	}
	return &provider.History{}, nil
}

func bars(start time.Time, closes ...*float64) []provider.Bar {
	var out []provider.Bar
	for i, c := range closes {
		if c == nil {
			continue // a null close never reaches the CLI: the provider drops it
		}
		out = append(out, provider.Bar{Time: start.AddDate(0, 0, i), Close: *c})
	}
	return out
}

func f64(v float64) *float64 { return &v }

var t0 = time.Date(2025, 1, 2, 14, 30, 0, 0, time.UTC)

type harness struct {
	api    *fakeAPI
	yahoo  *fakeYahoo
	fx     *fakeFX
	normal int
	deps   *Deps
}

func newHarness(t *testing.T, secs []client.Security, pairs client.FXPairs) *harness {
	t.Helper()
	h := &harness{
		api:   newFakeAPI(t, secs, pairs),
		yahoo: &fakeYahoo{currency: map[string]string{}, hist: map[string]*provider.History{}, errs: map[string]error{}},
		fx:    &fakeFX{hist: map[string]*provider.History{}, errs: map[string]error{}},
	}
	api := client.NewKuberanClient(h.api.srv.URL, testKey, h.api.srv.Client())
	h.deps = &Deps{
		API:    api,
		Yahoo:  h.yahoo,
		FX:     h.fx,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC) },
		Normal: func(context.Context) int { h.normal++; return 0 },
	}
	return h
}

func (h *harness) run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Run(context.Background(), args, &out, &errb, func() (*Deps, error) { return h.deps, nil })
	return code, out.String(), errb.String()
}

func TestParseBackfill(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 3, 1, 23, 59, 0, 0, time.UTC) }
	d := func(s string) time.Time { v, _ := time.Parse(dateLayout, s); return v }

	tests := []struct {
		name    string
		args    []string
		wantErr string
		check   func(t *testing.T, o backfillOpts)
	}{
		{"defaults: both on, until is today UTC", []string{"--since", "2025-01-01"}, "", func(t *testing.T, o backfillOpts) {
			if !o.prices || !o.fx || o.dryRun || o.symbols != nil {
				t.Errorf("unexpected opts %+v", o)
			}
			if !o.since.Equal(d("2025-01-01")) || !o.until.Equal(d("2025-03-01")) {
				t.Errorf("since/until = %v/%v", o.since, o.until)
			}
			if !o.to().Equal(d("2025-03-02")) {
				t.Errorf("to = %v, want exclusive end of until day", o.to())
			}
		}},
		{"only prices", []string{"--since", "2025-01-01", "--prices"}, "", func(t *testing.T, o backfillOpts) {
			if !o.prices || o.fx {
				t.Errorf("opts %+v", o)
			}
		}},
		{"only fx", []string{"--since", "2025-01-01", "--fx"}, "", func(t *testing.T, o backfillOpts) {
			if o.prices || !o.fx {
				t.Errorf("opts %+v", o)
			}
		}},
		{"both flags explicit", []string{"--since", "2025-01-01", "--fx", "--prices"}, "", func(t *testing.T, o backfillOpts) {
			if !o.prices || !o.fx {
				t.Errorf("opts %+v", o)
			}
		}},
		{"until and symbols and dry-run", []string{"--since", "2025-01-01", "--until", "2025-01-31", "--symbols", "nvda, vwra,", "--dry-run"}, "", func(t *testing.T, o backfillOpts) {
			if !o.until.Equal(d("2025-01-31")) || !o.dryRun || len(o.symbols) != 2 {
				t.Errorf("opts %+v", o)
			}
			if _, ok := o.symbols["NVDA"]; !ok {
				t.Errorf("symbols not upper-cased: %v", o.symbols)
			}
		}},
		{"missing since", nil, "--since YYYY-MM-DD is required", nil},
		{"bad since", []string{"--since", "01/01/2025"}, "invalid --since", nil},
		{"impossible since date", []string{"--since", "2025-02-30"}, "invalid --since", nil},
		{"bad until", []string{"--since", "2025-01-01", "--until", "soon"}, "invalid --until", nil},
		{"until before since", []string{"--since", "2025-02-01", "--until", "2025-01-01"}, "before --since", nil},
		{"unknown flag", []string{"--since", "2025-01-01", "--bogus"}, "bogus", nil},
		{"stray argument", []string{"--since", "2025-01-01", "extra"}, "unexpected argument", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseBackfill(tt.args, io.Discard, now)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, o)
		})
	}
}

func TestRun_NoArgsRunsNormalPath(t *testing.T) {
	h := newHarness(t, nil, client.FXPairs{})
	h.deps.Normal = func(context.Context) int { h.normal++; return 2 }
	code, out, errOut := h.run()
	if h.normal != 1 {
		t.Fatalf("normal run called %d times, want 1", h.normal)
	}
	if code != 2 {
		t.Errorf("exit = %d, want the normal run's code 2", code)
	}
	if out != "" || errOut != "" {
		t.Errorf("no-arg run must print nothing itself: %q %q", out, errOut)
	}
	if len(h.api.requests()) != 0 {
		t.Errorf("subcommand API calls on the normal path: %v", h.api.requests())
	}
}

func TestRun_UsageErrorsExit2WithoutBuildingDeps(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown subcommand", []string{"frobnicate"}, `unknown subcommand "frobnicate"`},
		{"help", []string{"help"}, "Usage:"},
		{"backfill without since", []string{"backfill"}, "--since YYYY-MM-DD is required"},
		{"backfill bad date", []string{"backfill", "--since", "yesterday"}, "invalid --since"},
		{"reconcile stray arg", []string{"reconcile", "now"}, "unexpected argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := Run(context.Background(), tt.args, &out, &errb, func() (*Deps, error) {
				t.Fatal("deps must not be built for a usage error")
				return nil, nil
			})
			if code != ExitUsage {
				t.Errorf("exit = %d, want 2", code)
			}
			if !strings.Contains(errb.String(), tt.wantErr) || !strings.Contains(errb.String(), "oracle reconcile") {
				t.Errorf("stderr = %q", errb.String())
			}
		})
	}
}

func TestRun_ConfigErrorExits1(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"reconcile"}, &out, &errb, func() (*Deps, error) { return nil, errors.New("PIPELINE_API_KEY is required") })
	if code != ExitFail || !strings.Contains(errb.String(), "configuration error") {
		t.Errorf("code=%d stderr=%q", code, errb.String())
	}
}

func reconcileSecurities() []client.Security {
	return []client.Security{
		{ID: "s1", Symbol: "NVDA", AssetType: "stock", Exchange: "NASDAQ", Currency: "MYR"},
		{ID: "s2", Symbol: "VWRA", AssetType: "ETF", Exchange: "LSE", ProviderSymbol: "VWRA.L", Currency: "MYR"},
		{ID: "s3", Symbol: "MAYBANK", AssetType: "stock", Exchange: "BURSA", Currency: "myr"},
		{ID: "s4", Symbol: "BTC", AssetType: "cryptocurrency", Currency: "MYR"},
		{ID: "s5", Symbol: "BAD", AssetType: "stock", Exchange: "NYSE", Currency: "MYR"},
	}
}

func reconcileHarness(t *testing.T) *harness {
	h := newHarness(t, reconcileSecurities(), client.FXPairs{})
	h.yahoo.currency = map[string]string{"NVDA": "USD", "VWRA.L": "USD", "MAYBANK.KL": "MYR"}
	h.yahoo.errs["BAD"] = errors.New("unexpected status 404")
	h.api.pricesDel = 120
	return h
}

func TestReconcile_DryRunChangesNothing(t *testing.T) {
	h := reconcileHarness(t)
	code, out, _ := h.run("reconcile")

	if len(h.api.writes()) != 0 {
		t.Fatalf("dry run wrote to the API: %v", h.api.writes())
	}
	if code != ExitFail {
		t.Errorf("exit = %d, want 1 because one fetch failed", code)
	}
	for _, want := range []string{"NVDA", "VWRA.L", "mismatch (dry run", "mismatches=2 applied=0 failed=1", "fetch failed: unexpected status 404"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "BTC") {
		t.Errorf("crypto must not appear:\n%s", out)
	}
	// Case-insensitive match: the lower-case stored "myr" is not a mismatch.
	if !strings.Contains(out, "MAYBANK") || strings.Contains(out, "MAYBANK\tBURSA\tMAYBANK.KL\tmyr\tMYR\tmismatch") {
		t.Errorf("MAYBANK should be ok:\n%s", out)
	}
	if strings.Contains(out, testKey) {
		t.Error("API key leaked to output")
	}
}

func TestReconcile_ApplyPatchesMismatchesAndContinuesPastFailures(t *testing.T) {
	h := reconcileHarness(t)
	h.api.patchCode["s2"] = http.StatusConflict
	code, out, _ := h.run("reconcile", "--apply")

	writes := h.api.writes()
	if len(writes) != 2 {
		t.Fatalf("got %d writes, want 2 PATCHes: %v", len(writes), writes)
	}
	for i, id := range []string{"s1", "s2"} {
		w := writes[i]
		if w.Method != http.MethodPatch || w.Path != "/api/v1/pipeline/securities/"+id {
			t.Errorf("write %d = %s %s", i, w.Method, w.Path)
		}
		if w.Body["currency"] != "USD" || w.Body["reset_prices"] != true || len(w.Body) != 2 {
			t.Errorf("PATCH body = %v, want exactly currency=USD, reset_prices=true", w.Body)
		}
	}
	if code != ExitFail {
		t.Errorf("exit = %d, want 1 (a PATCH and a fetch failed)", code)
	}
	for _, want := range []string{"updated, prices_deleted=120", "update failed", "409", "applied=1 failed=2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReconcile_AllAppliedExits0(t *testing.T) {
	h := reconcileHarness(t)
	delete(h.yahoo.errs, "BAD")
	h.yahoo.currency["BAD"] = "MYR"
	code, _, _ := h.run("reconcile", "--apply")
	if code != ExitOK {
		t.Errorf("exit = %d, want 0", code)
	}
	if len(h.api.writes()) != 2 {
		t.Errorf("writes = %d, want 2", len(h.api.writes()))
	}
}

func priceBackfillHarness(t *testing.T) *harness {
	secs := []client.Security{
		{ID: "s1", Symbol: "NVDA", AssetType: "stock", Exchange: "NASDAQ", Currency: "USD"},
		{ID: "s2", Symbol: "VOD", AssetType: "stock", Exchange: "LSE", Currency: "GBP"},
		{ID: "s3", Symbol: "BTC", AssetType: "cryptocurrency", Currency: "MYR"},
	}
	h := newHarness(t, secs, client.FXPairs{Pairs: []client.CurrencyPair{{Base: "USD", Quote: "MYR"}}, BaseCurrencies: []string{"MYR"}})
	h.yahoo.hist["NVDA"] = &provider.History{Currency: "USD", Bars: bars(t0, f64(100.25), nil, f64(101.5))}
	h.yahoo.hist["VOD.L"] = &provider.History{Currency: "GBp", Bars: bars(t0, f64(525.495), f64(70.4))}
	h.fx.hist["USDMYR"] = &provider.History{Currency: "MYR", Bars: bars(t0, f64(4.4712), f64(4.5))}
	h.fx.hist["GBPMYR"] = &provider.History{Currency: "MYR", Bars: bars(t0, f64(5.7))}
	return h
}

func TestBackfill_FXBeforePricesAndPayloads(t *testing.T) {
	h := priceBackfillHarness(t)
	code, out, _ := h.run("backfill", "--since", "2025-01-01", "--until", "2025-01-31")
	if code != ExitOK {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}

	writes := h.api.writes()
	var paths []string
	for _, w := range writes {
		paths = append(paths, w.Path)
	}
	// Every FX post precedes every price post.
	lastFX, firstPrice := -1, len(paths)
	for i, p := range paths {
		if strings.HasSuffix(p, "/fx-rates") {
			lastFX = i
		} else if strings.HasSuffix(p, "/securities/prices") && i < firstPrice {
			firstPrice = i
		}
	}
	if lastFX < 0 || firstPrice == len(paths) || lastFX > firstPrice {
		t.Fatalf("FX must be posted before prices, got order %v", paths)
	}

	// FX pairs: API pair USD/MYR plus seen currencies (USD, GBP) x base MYR.
	if got := strings.Join(h.fx.got, ","); got != "GBPMYR,USDMYR" {
		t.Errorf("fx pairs fetched = %s", got)
	}

	// Price payloads: null close skipped, GBp normalised once, per-security posts.
	var nvda, vod []any
	for _, w := range writes {
		if !strings.HasSuffix(w.Path, "/securities/prices") {
			continue
		}
		ps := w.Body["prices"].([]any)
		switch ps[0].(map[string]any)["security_id"] {
		case "s1":
			nvda = ps
		case "s2":
			vod = ps
		}
	}
	if len(nvda) != 2 {
		t.Fatalf("NVDA posted %d entries, want 2 (null close skipped)", len(nvda))
	}
	e0 := nvda[0].(map[string]any)
	if e0["price"].(float64) != 10025 || e0["currency"] != "USD" || e0["recorded_at"] != "2025-01-02T14:30:00Z" {
		t.Errorf("NVDA[0] = %v", e0)
	}
	if got := nvda[1].(map[string]any)["recorded_at"]; got != "2025-01-04T14:30:00Z" {
		t.Errorf("NVDA[1] recorded_at = %v (null close on day 2 must be skipped, not shifted)", got)
	}
	if len(vod) != 2 {
		t.Fatalf("VOD posted %d entries, want 2", len(vod))
	}
	v0 := vod[0].(map[string]any)
	if v0["price"].(float64) != 525 || v0["currency"] != "GBP" {
		t.Errorf("VOD[0] = %v, want 525 GBP cents (525.495 GBp rounded once)", v0)
	}

	// FX payload: decimal string rate, bar timestamp, source yahoo.
	for _, w := range writes {
		if strings.HasSuffix(w.Path, "/fx-rates") {
			r := w.Body["rates"].([]any)[0].(map[string]any)
			if r["base"] == "USD" && (r["rate"] != "4.4712000000" || r["source"] != "yahoo" || r["recorded_at"] != "2025-01-02T14:30:00Z") {
				t.Errorf("USD/MYR rate entry = %v", r)
			}
		}
	}

	// History window: since 00:00 UTC to the end of the until day.
	if r := h.yahoo.ranges[0]; !r[0].Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) || !r[1].Equal(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("history range = %v", r)
	}
	// Crypto skipped, totals printed, key never printed.
	if strings.Contains(strings.Join(h.yahoo.histCall, ","), "BTC") {
		t.Error("crypto must not be fetched from Yahoo")
	}
	for _, want := range []string{"price totals: securities=2 fetched=4 recorded=4 rejected=0", "fx totals: pairs=2 fetched=3 recorded=3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, testKey) {
		t.Error("API key leaked to output")
	}
}

func TestBackfill_SymbolsFilterIsCaseInsensitive(t *testing.T) {
	h := priceBackfillHarness(t)
	code, _, _ := h.run("backfill", "--since", "2025-01-01", "--prices", "--symbols", "vod")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.Join(h.yahoo.histCall, ","); got != "VOD.L" {
		t.Errorf("fetched %q, want only VOD.L", got)
	}
	if len(h.fx.got) != 0 {
		t.Errorf("--prices alone must not touch FX: %v", h.fx.got)
	}
	writes := h.api.writes()
	if len(writes) != 1 || writes[0].Body["prices"].([]any)[0].(map[string]any)["security_id"] != "s2" {
		t.Errorf("writes = %v", writes)
	}
}

func TestBackfill_FXOnlyDoesNotFetchPrices(t *testing.T) {
	h := priceBackfillHarness(t)
	code, _, _ := h.run("backfill", "--since", "2025-01-01", "--fx")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if len(h.yahoo.histCall) != 0 {
		t.Errorf("--fx alone fetched price history: %v", h.yahoo.histCall)
	}
	if got := strings.Join(h.fx.got, ","); got != "USDMYR" {
		t.Errorf("fx pairs = %s, want only the API's USD/MYR", got)
	}
	for _, w := range h.api.writes() {
		if !strings.HasSuffix(w.Path, "/fx-rates") {
			t.Errorf("unexpected write %s", w.Path)
		}
	}
}

func TestBackfill_DryRunPostsNothing(t *testing.T) {
	h := priceBackfillHarness(t)
	code, out, _ := h.run("backfill", "--since", "2025-01-01", "--dry-run")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if w := h.api.writes(); len(w) != 0 {
		t.Fatalf("dry run posted: %v", w)
	}
	if len(h.yahoo.histCall) != 2 || len(h.fx.got) != 2 {
		t.Errorf("dry run must still fetch: yahoo=%v fx=%v", h.yahoo.histCall, h.fx.got)
	}
	if !strings.Contains(out, "dry run (would post 2)") || !strings.Contains(out, "fetched=4 recorded=0") {
		t.Errorf("output:\n%s", out)
	}
}

func TestBackfill_BatchesAtFiveThousand(t *testing.T) {
	secs := []client.Security{{ID: "s1", Symbol: "NVDA", AssetType: "stock", Currency: "USD"}}
	h := newHarness(t, secs, client.FXPairs{Pairs: []client.CurrencyPair{{Base: "USD", Quote: "MYR"}}, BaseCurrencies: []string{"MYR"}})
	var closes []*float64
	for i := 0; i < 5001; i++ {
		closes = append(closes, f64(10+float64(i)/100))
	}
	h.yahoo.hist["NVDA"] = &provider.History{Currency: "USD", Bars: bars(t0, closes...)}
	h.fx.hist["USDMYR"] = &provider.History{Bars: bars(t0, closes...)}

	if code, out, _ := h.run("backfill", "--since", "2025-01-01"); code != ExitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	var priceSizes, fxSizes []int
	for _, w := range h.api.writes() {
		switch {
		case strings.HasSuffix(w.Path, "/securities/prices"):
			priceSizes = append(priceSizes, len(w.Body["prices"].([]any)))
		case strings.HasSuffix(w.Path, "/fx-rates"):
			fxSizes = append(fxSizes, len(w.Body["rates"].([]any)))
		}
	}
	if fmt.Sprint(priceSizes) != "[5000 1]" || fmt.Sprint(fxSizes) != "[5000 1]" {
		t.Errorf("price batches %v, fx batches %v; want [5000 1] each", priceSizes, fxSizes)
	}
}

func TestBackfill_FailuresDoNotStopOthersAndExit1(t *testing.T) {
	h := priceBackfillHarness(t)
	h.yahoo.errs["NVDA"] = errors.New("unexpected status 500")
	h.fx.errs["USDMYR"] = errors.New("no data")
	code, out, _ := h.run("backfill", "--since", "2025-01-01")
	if code != ExitFail {
		t.Errorf("exit = %d, want 1", code)
	}
	// VOD prices and the GBP/MYR rates still went through.
	var sawVOD, sawGBP bool
	for _, w := range h.api.writes() {
		if strings.HasSuffix(w.Path, "/securities/prices") {
			sawVOD = w.Body["prices"].([]any)[0].(map[string]any)["security_id"] == "s2"
		}
		if strings.HasSuffix(w.Path, "/fx-rates") {
			sawGBP = w.Body["rates"].([]any)[0].(map[string]any)["base"] == "GBP"
		}
	}
	if !sawVOD || !sawGBP {
		t.Errorf("remaining work was skipped: vod=%v gbp=%v", sawVOD, sawGBP)
	}
	if !strings.Contains(out, "fetch failed: no data") || !strings.Contains(out, "finished with") {
		t.Errorf("output:\n%s", out)
	}
}

func TestBackfill_RejectedPricesAreCountedAndFail(t *testing.T) {
	h := priceBackfillHarness(t)
	h.api.rejectCurrency = "USD" // GBP prices for VOD get rejected
	code, out, _ := h.run("backfill", "--since", "2025-01-01", "--prices")
	if code != ExitFail {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(out, "rejected: currency_mismatch (posted GBP, security is USD)") || !strings.Contains(out, "rejected=2") {
		t.Errorf("output:\n%s", out)
	}
}
