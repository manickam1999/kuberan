package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/kuberan/oracle/internal/client"
	"github.com/kuberan/oracle/internal/config"
	"github.com/kuberan/oracle/internal/oracle"
	"github.com/kuberan/oracle/internal/provider"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}

	kuberanClient := client.NewKuberanClient(cfg.KuberanAPIURL, cfg.PipelineAPIKey, httpClient)

	forexConverter := provider.NewForexConverter(httpClient, cfg.TargetCurrency)

	providers := []provider.Provider{
		provider.NewYahooProvider(httpClient),
		provider.NewCoinGeckoProvider(httpClient, cfg.TargetCurrency),
	}

	logger.Info("oracle starting",
		"target_currency", cfg.TargetCurrency,
		"compute_snapshots", cfg.ComputeSnapshots,
		"process_fixed_deposits", cfg.ProcessFixedDeposits,
	)

	orc := oracle.NewOracle(kuberanClient, providers, forexConverter, cfg, logger)
	ctx := context.Background()
	result, err := orc.Run(ctx)
	if err != nil {
		logger.Error("oracle run failed", "error", err)
		os.Exit(1)
	}

	logger.Info("oracle run completed",
		"securities_fetched", result.SecuritiesFetched,
		"prices_recorded", result.PricesRecorded,
		"snapshots_recorded", result.SnapshotsRecorded,
		"fd_payouts_posted", result.FDPayoutsPosted,
		"fds_matured", result.FDsMatured,
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
		os.Exit(2)
	}
}
