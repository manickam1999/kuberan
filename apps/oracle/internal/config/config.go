// Package config loads oracle configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config holds all oracle configuration values.
type Config struct {
	KuberanAPIURL        string
	PipelineAPIKey       string
	LogLevel             slog.Level
	RequestTimeout       time.Duration
	ComputeSnapshots     bool
	ProcessFixedDeposits bool
	TargetCurrency       string // Target currency for all prices (default: "MYR")
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.KuberanAPIURL = os.Getenv("KUBERAN_API_URL")
	if cfg.KuberanAPIURL == "" {
		return nil, fmt.Errorf("KUBERAN_API_URL is required")
	}

	cfg.PipelineAPIKey = os.Getenv("PIPELINE_API_KEY")
	if cfg.PipelineAPIKey == "" {
		return nil, fmt.Errorf("PIPELINE_API_KEY is required")
	}

	level, err := parseLogLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = level

	timeout, err := parseTimeout(os.Getenv("REQUEST_TIMEOUT"))
	if err != nil {
		return nil, err
	}
	cfg.RequestTimeout = timeout

	snapshots, err := parseBool(os.Getenv("COMPUTE_SNAPSHOTS"), true)
	if err != nil {
		return nil, fmt.Errorf("invalid COMPUTE_SNAPSHOTS value: %w", err)
	}
	cfg.ComputeSnapshots = snapshots

	processFixedDeposits, err := parseBool(os.Getenv("PROCESS_FIXED_DEPOSITS"), true)
	if err != nil {
		return nil, fmt.Errorf("invalid PROCESS_FIXED_DEPOSITS value: %w", err)
	}
	cfg.ProcessFixedDeposits = processFixedDeposits

	cfg.TargetCurrency = strings.ToUpper(os.Getenv("TARGET_CURRENCY"))
	if cfg.TargetCurrency == "" {
		cfg.TargetCurrency = "MYR"
	}

	return cfg, nil
}

func parseLogLevel(s string) (slog.Level, error) {
	if s == "" {
		return slog.LevelInfo, nil
	}
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q: must be debug, info, warn, or error", s)
	}
}

func parseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 30 * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid REQUEST_TIMEOUT %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("REQUEST_TIMEOUT must be positive, got %v", d)
	}
	return d, nil
}

func parseBool(s string, defaultVal bool) (bool, error) {
	if s == "" {
		return defaultVal, nil
	}
	switch strings.ToLower(s) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("must be true, false, 1, or 0, got %q", s)
	}
}
