package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/oracle"
	"github.com/kuberan/oracle/internal/provider"
)

// maxBatch is the most entries one pipeline request may carry.
const maxBatch = 5000

const dateLayout = "2006-01-02"

type backfillOpts struct {
	since, until time.Time // UTC midnights, both inclusive
	prices, fx   bool
	symbols      map[string]struct{} // upper-cased; nil means all
	dryRun       bool
}

// from and to return the half-open [from, to) instant range for the Yahoo
// period1/period2 parameters.
func (o backfillOpts) from() time.Time { return o.since }
func (o backfillOpts) to() time.Time   { return o.until.AddDate(0, 0, 1) }

func parseBackfill(args []string, stderr io.Writer, now func() time.Time) (backfillOpts, error) {
	var o backfillOpts
	var since, until, symbols string
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&since, "since", "", "first day to backfill, YYYY-MM-DD (required)")
	fs.StringVar(&until, "until", "", "last day to backfill, YYYY-MM-DD (default today, UTC)")
	fs.BoolVar(&o.prices, "prices", false, "backfill security prices")
	fs.BoolVar(&o.fx, "fx", false, "backfill FX rates")
	fs.StringVar(&symbols, "symbols", "", "comma-separated symbols to limit the price backfill to")
	fs.BoolVar(&o.dryRun, "dry-run", false, "fetch and count, post nothing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, errHelp
		}
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("backfill: unexpected argument %q", fs.Arg(0))
	}

	if since == "" {
		return o, errors.New("backfill: --since YYYY-MM-DD is required")
	}
	var err error
	if o.since, err = time.Parse(dateLayout, since); err != nil {
		return o, fmt.Errorf("backfill: invalid --since %q (want YYYY-MM-DD)", since)
	}
	today := now().UTC().Truncate(24 * time.Hour)
	if until == "" {
		o.until = today
	} else if o.until, err = time.Parse(dateLayout, until); err != nil {
		return o, fmt.Errorf("backfill: invalid --until %q (want YYYY-MM-DD)", until)
	}
	if o.until.Before(o.since) {
		return o, fmt.Errorf("backfill: --until %s is before --since %s", until, since)
	}

	if !o.prices && !o.fx {
		o.prices, o.fx = true, true
	}
	if symbols != "" {
		o.symbols = make(map[string]struct{})
		for _, s := range strings.Split(symbols, ",") {
			if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
				o.symbols[s] = struct{}{}
			}
		}
	}
	return o, nil
}

// priceHistory is one security's fetched, normalised history awaiting posting.
type priceHistory struct {
	sec     client.Security
	entries []client.RecordPriceEntry
	fetched int
}

// runBackfill fetches history, posts FX rates and then prices (FX always before
// prices), and returns 1 if anything failed.
func runBackfill(ctx context.Context, d *Deps, o backfillOpts, stdout io.Writer) int {
	failed := 0
	fail := func(format string, a ...any) {
		failed++
		d.Logger.Warn(fmt.Sprintf(format, a...))
	}

	// 1. Fetch price history (read-only) so FX knows which currencies are in play.
	var histories []priceHistory
	seen := map[string]struct{}{}
	if o.prices {
		secs, err := d.API.GetSecurities(ctx)
		if err != nil {
			d.Logger.Error("fetching securities failed", "error", err)
			return ExitFail
		}
		for _, s := range secs {
			ps, ok := yahooSecurity(d.Yahoo, s)
			if !ok {
				d.Logger.Info("skipping security not priced by Yahoo", "symbol", s.Symbol, "asset_type", s.AssetType)
				continue
			}
			if o.symbols != nil {
				if _, ok := o.symbols[strings.ToUpper(s.Symbol)]; !ok {
					continue
				}
			}
			ticker := provider.BuildYahooSymbol(ps)
			h, err := d.Yahoo.FetchHistory(ctx, ticker, o.from(), o.to())
			if err != nil {
				fail("price history fetch failed: symbol=%s ticker=%s error=%v", s.Symbol, ticker, err)
				continue
			}
			if len(h.Bars) > 0 && h.Currency == "" {
				fail("price history for %s reports no currency, skipping", s.Symbol)
				continue
			}
			ph := priceHistory{sec: s, fetched: len(h.Bars)}
			for _, b := range h.Bars {
				cents, cur := provider.NormalizeYahooPrice(b.Close, h.Currency)
				if cents <= 0 {
					continue
				}
				seen[cur] = struct{}{}
				ph.entries = append(ph.entries, client.RecordPriceEntry{
					SecurityID: s.ID,
					Price:      cents,
					RecordedAt: b.Time.Format(time.RFC3339),
					Currency:   cur,
				})
			}
			histories = append(histories, ph)
		}
	}

	// 2. FX first.
	if o.fx {
		failed += backfillFX(ctx, d, o, seen, stdout)
	}

	// 3. Then prices.
	if o.prices {
		failed += postPrices(ctx, d, o, histories, stdout)
	}

	if failed > 0 {
		_, _ = fmt.Fprintf(stdout, "\nbackfill finished with %d failure(s)\n", failed)
		return ExitFail
	}
	return ExitOK
}

func backfillFX(ctx context.Context, d *Deps, o backfillOpts, seen map[string]struct{}, stdout io.Writer) int {
	pairsResp, err := d.API.GetFXPairs(ctx)
	if err != nil {
		d.Logger.Error("fetching fx pairs failed", "error", err)
		return 1
	}
	reported := make([]string, 0, len(seen))
	for c := range seen {
		reported = append(reported, c)
	}
	sort.Strings(reported)
	pairs := oracle.UnionFXPairs(pairsResp.Pairs, pairsResp.BaseCurrencies, reported)

	failed := 0
	var totalFetched, totalRecorded int
	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FX PAIR\tFETCHED\tRECORDED\tREJECTED\tSTATUS")
	for _, p := range pairs {
		label := p.Base + "/" + p.Quote
		h, err := d.FX.FetchHistory(ctx, p.Base, p.Quote, o.from(), o.to())
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(tw, "%s\t0\t0\t0\tfetch failed: %v\n", label, err)
			continue
		}
		entries := make([]client.FXRateEntry, 0, len(h.Bars))
		for _, b := range h.Bars {
			if b.Close <= 0 {
				continue
			}
			entries = append(entries, client.FXRateEntry{
				Base:  p.Base,
				Quote: p.Quote,
				Rate:  strconv.FormatFloat(b.Close, 'f', 10, 64),
				// One row per pair per UTC day, merging with the live run.
				RecordedAt: b.Time.UTC().Truncate(24 * time.Hour).Format(time.RFC3339),
				Source:     "yahoo",
			})
		}
		totalFetched += len(h.Bars)
		if o.dryRun {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t0\t0\tdry run (would post %d)\n", label, len(h.Bars), len(entries))
			continue
		}
		recorded, err := d.API.RecordFXRates(ctx, entries)
		totalRecorded += recorded
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\trecord failed: %v\n", label, len(h.Bars), recorded, len(entries)-recorded, err)
			continue
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\tok\n", label, len(h.Bars), recorded, len(entries)-recorded)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(stdout, "fx totals: pairs=%d fetched=%d recorded=%d\n\n", len(pairs), totalFetched, totalRecorded)
	return failed
}

func postPrices(ctx context.Context, d *Deps, o backfillOpts, histories []priceHistory, stdout io.Writer) int {
	failed := 0
	var totalFetched, totalRecorded, totalRejected int
	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SYMBOL\tCURRENCY\tFETCHED\tRECORDED\tREJECTED\tSTATUS")
	for _, ph := range histories {
		cur := ""
		if len(ph.entries) > 0 {
			cur = ph.entries[0].Currency
		}
		totalFetched += ph.fetched
		if o.dryRun {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t0\t0\tdry run (would post %d)\n", ph.sec.Symbol, cur, ph.fetched, len(ph.entries))
			continue
		}
		var recorded, rejected int
		var postErr error
		var firstReason string
		for start := 0; start < len(ph.entries); start += maxBatch {
			end := min(start+maxBatch, len(ph.entries))
			res, err := d.API.RecordPrices(ctx, ph.entries[start:end])
			if err != nil {
				postErr = err
				break
			}
			recorded += res.Recorded
			rejected += len(res.Rejected)
			if firstReason == "" && len(res.Rejected) > 0 {
				r := res.Rejected[0]
				firstReason = fmt.Sprintf("%s (posted %s, security is %s)", r.Reason, r.Currency, r.ExpectedCurrency)
			}
		}
		totalRecorded += recorded
		totalRejected += rejected
		status := "ok"
		switch {
		case postErr != nil:
			failed++
			status = "record failed: " + postErr.Error()
		case rejected > 0:
			failed++
			status = "rejected: " + firstReason
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\n", ph.sec.Symbol, cur, ph.fetched, recorded, rejected, status)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(stdout, "price totals: securities=%d fetched=%d recorded=%d rejected=%d\n", len(histories), totalFetched, totalRecorded, totalRejected)
	return failed
}
