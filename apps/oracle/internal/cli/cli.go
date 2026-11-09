// Package cli implements the oracle's command line: the normal scheduled run
// (no arguments) and the one-off migration subcommands reconcile and backfill.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/provider"
)

// Exit codes returned by Run.
const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2
)

const usageText = `Usage:
  oracle                         run the normal price/FX cycle (the scheduled mode)
  oracle reconcile [--apply]     compare each Yahoo security's stored currency with
                                 Yahoo's; dry run unless --apply is given
  oracle backfill --since YYYY-MM-DD [--until YYYY-MM-DD] [--prices] [--fx]
                  [--symbols A,B] [--dry-run]
                                 backfill daily history; with neither --prices nor
                                 --fx both are done (FX first); --until defaults to
                                 today (UTC); --dry-run posts nothing
  oracle help                    show this message
`

// API is the subset of the Kuberan pipeline client the subcommands use.
type API interface {
	GetSecurities(ctx context.Context) ([]client.Security, error)
	UpdateSecurity(ctx context.Context, id string, upd client.UpdateSecurityRequest) (client.UpdateSecurityResult, error)
	RecordPrices(ctx context.Context, prices []client.RecordPriceEntry) (client.RecordPricesResult, error)
	GetFXPairs(ctx context.Context) (client.FXPairs, error)
	RecordFXRates(ctx context.Context, rates []client.FXRateEntry) (int, error)
}

// YahooSource is the Yahoo access the subcommands need.
type YahooSource interface {
	Supports(assetType string) bool
	FetchCurrency(ctx context.Context, ticker string) (string, error)
	FetchHistory(ctx context.Context, ticker string, from, to time.Time) (*provider.History, error)
}

// FXSource fetches daily history for currency pairs.
type FXSource interface {
	FetchHistory(ctx context.Context, base, quote string, from, to time.Time) (*provider.History, error)
}

// Deps are the collaborators of the commands. They are built lazily (see Run) so
// that usage errors never need configuration.
type Deps struct {
	API    API
	Yahoo  YahooSource
	FX     FXSource
	Logger *slog.Logger
	Now    func() time.Time
	// Normal runs the scheduled cycle and returns its exit code.
	Normal func(ctx context.Context) int
}

// Run dispatches args (os.Args[1:]) and returns the process exit code. No
// arguments runs the normal cycle. newDeps is called only once the arguments are
// known to be valid.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, newDeps func() (*Deps, error)) int {
	var run func(*Deps) int

	if len(args) == 0 {
		run = func(d *Deps) int { return d.Normal(ctx) }
	} else {
		switch args[0] {
		case "reconcile":
			opts, err := parseReconcile(args[1:], stderr)
			if err != nil {
				return usageError(stderr, err)
			}
			run = func(d *Deps) int { return runReconcile(ctx, d, opts, stdout) }
		case "backfill":
			opts, err := parseBackfill(args[1:], stderr, time.Now)
			if err != nil {
				return usageError(stderr, err)
			}
			run = func(d *Deps) int { return runBackfill(ctx, d, opts, stdout) }
		default:
			if args[0] != "help" && args[0] != "-h" && args[0] != "--help" {
				_, _ = fmt.Fprintf(stderr, "oracle: unknown subcommand %q\n\n", args[0])
			}
			_, _ = fmt.Fprint(stderr, usageText)
			return ExitUsage
		}
	}

	d, err := newDeps()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return ExitFail
	}
	return run(d)
}

// usageError prints a parse error (flag errors were already reported by the flag
// set) and the usage text, and returns the usage exit code.
func usageError(stderr io.Writer, err error) int {
	if err != errHelp {
		_, _ = fmt.Fprintf(stderr, "oracle: %v\n\n", err)
	}
	_, _ = fmt.Fprint(stderr, usageText)
	return ExitUsage
}
