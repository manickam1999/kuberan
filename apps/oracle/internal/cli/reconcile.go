package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/provider"
)

var errHelp = errors.New("help requested")

type reconcileOpts struct {
	apply bool
}

func parseReconcile(args []string, stderr io.Writer) (reconcileOpts, error) {
	var o reconcileOpts
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.apply, "apply", false, "apply the currency corrections (default is a dry run)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, errHelp
		}
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("reconcile: unexpected argument %q", fs.Arg(0))
	}
	return o, nil
}

// yahooSecurity reports whether a security is priced by Yahoo and builds the
// provider-side view of it, with the same asset-type logic as the run.
func yahooSecurity(y YahooSource, s client.Security) (provider.Security, bool) {
	assetType := strings.ToLower(s.AssetType)
	if assetType == "cryptocurrency" {
		assetType = "crypto"
	}
	if !y.Supports(assetType) {
		return provider.Security{}, false
	}
	return provider.Security{
		ID:             s.ID,
		Symbol:         s.Symbol,
		AssetType:      assetType,
		Exchange:       s.Exchange,
		ProviderSymbol: s.ProviderSymbol,
		Network:        s.Network,
		Currency:       s.Currency,
	}, true
}

// runReconcile compares stored currencies with Yahoo's and, with --apply, fixes
// the mismatches. It returns 1 if any fetch or update failed.
func runReconcile(ctx context.Context, d *Deps, o reconcileOpts, stdout io.Writer) int {
	secs, err := d.API.GetSecurities(ctx)
	if err != nil {
		d.Logger.Error("fetching securities failed", "error", err)
		return ExitFail
	}

	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SYMBOL\tEXCHANGE\tPROVIDER_SYMBOL\tSTORED\tPROVIDER\tACTION")

	var mismatches, applied, failed int
	for _, s := range secs {
		ps, ok := yahooSecurity(d.Yahoo, s)
		if !ok {
			continue
		}
		ticker := provider.BuildYahooSymbol(ps)
		row := func(provCur, action string) {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Symbol, s.Exchange, ticker, s.Currency, provCur, action)
		}

		cur, err := d.Yahoo.FetchCurrency(ctx, ticker)
		if err != nil {
			failed++
			row("-", "fetch failed: "+err.Error())
			continue
		}
		if strings.EqualFold(cur, s.Currency) {
			row(cur, "ok")
			continue
		}

		mismatches++
		if !o.apply {
			row(cur, "mismatch (dry run, use --apply)")
			continue
		}
		res, err := d.API.UpdateSecurity(ctx, s.ID, client.UpdateSecurityRequest{Currency: &cur, ResetPrices: true})
		if err != nil {
			failed++
			row(cur, "update failed: "+err.Error())
			continue
		}
		applied++
		row(cur, fmt.Sprintf("updated, prices_deleted=%d", res.PricesDeleted))
	}
	_ = tw.Flush()

	mode := "dry run"
	if o.apply {
		mode = "apply"
	}
	_, _ = fmt.Fprintf(stdout, "\nreconcile (%s): mismatches=%d applied=%d failed=%d\n", mode, mismatches, applied, failed)
	if failed > 0 {
		return ExitFail
	}
	return ExitOK
}
