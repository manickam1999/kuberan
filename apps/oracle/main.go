package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kuberan/oracle/internal/cli"
	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/config"
	"github.com/kuberan/oracle/internal/oracle"
	"github.com/kuberan/oracle/internal/provider"
)

// exitPartial is the exit code of a run that completed but had fetch errors.
const exitPartial = 2

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, newDeps))
}

// newDeps loads the configuration and builds the collaborators shared by the
// normal run and the subcommands. Subcommands print their tables on stdout, so
// their logs go to stderr.
func newDeps() (*cli.Deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logOut := os.Stdout
	if len(os.Args) > 1 {
		logOut = os.Stderr
	}
	logger := slog.New(slog.NewJSONHandler(logOut, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}

	// Snapshot computation and fixed-deposit processing can run for minutes on
	// large installs, so they get their own client with a longer timeout.
	pipelineClient := &http.Client{Timeout: cfg.PipelineTimeout}

	kuberanClient := client.NewKuberanClient(cfg.KuberanAPIURL, cfg.PipelineAPIKey, httpClient).
		WithLongRunningClient(pipelineClient)

	for _, w := range cfg.Warnings {
		logger.Warn(w)
	}

	rateFetcher := provider.NewForexConverter(httpClient)
	yahoo := provider.NewYahooProvider(httpClient)

	providers := []provider.Provider{
		yahoo,
		provider.NewCoinGeckoProvider(httpClient),
	}

	return &cli.Deps{
		API:    kuberanClient,
		Yahoo:  yahoo,
		FX:     rateFetcher,
		Logger: logger,
		Now:    time.Now,
		Normal: func(ctx context.Context) int {
			return runOnce(ctx, cfg, logger, kuberanClient, providers, rateFetcher)
		},
	}, nil
}

// runOnce is the normal scheduled cycle and returns the process exit code.
func runOnce(ctx context.Context, cfg *config.Config, logger *slog.Logger, kuberanClient *client.KuberanClient, providers []provider.Provider, rateFetcher *provider.ForexConverter) int {
	logger.Info("oracle starting",
		"compute_snapshots", cfg.ComputeSnapshots,
		"process_fixed_deposits", cfg.ProcessFixedDeposits,
		"publish_fx", cfg.PublishFX,
	)

	orc := oracle.NewOracle(kuberanClient, providers, rateFetcher, cfg, logger)
	result, err := orc.Run(ctx)
	if err != nil {
		logger.Error("oracle run failed", "error", err)
		return cli.ExitFail
	}

	logger.Info("oracle run completed",
		"securities_fetched", result.SecuritiesFetched,
		"prices_recorded", result.PricesRecorded,
		"snapshots_recorded", result.SnapshotsRecorded,
		"fd_payouts_posted", result.FDPayoutsPosted,
		"fds_matured", result.FDsMatured,
		"fx_rates_recorded", result.FXRatesRecorded,
		"errors", len(result.Errors),
		"duration", result.Duration.String(),
	)

	for _, fetchErr := range result.Errors {
		logger.Warn("price fetch failed",
			"symbol", fetchErr.Symbol,
			"security_id", fetchErr.SecurityID,
			"error", fetchErr.Err.Error(),
		)
	}

	if len(result.Errors) > 0 {
		return exitPartial
	}
	return cli.ExitOK
}
