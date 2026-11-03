// Package client provides an HTTP client for the Kuberan pipeline API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Security represents a security returned by the Kuberan pipeline API.
type Security struct {
	ID             string `json:"id"`
	Symbol         string `json:"symbol"`
	Name           string `json:"name"`
	AssetType      string `json:"asset_type"`
	Currency       string `json:"currency"`
	Exchange       string `json:"exchange"`
	ProviderSymbol string `json:"provider_symbol"`
	Network        string `json:"network"`
}

// RecordPriceEntry represents a single price entry to submit to the pipeline API.
type RecordPriceEntry struct {
	SecurityID string `json:"security_id"`
	Price      int64  `json:"price"`
	RecordedAt string `json:"recorded_at"` // RFC3339
}

// KuberanClient communicates with the Kuberan pipeline API.
type KuberanClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	// longClient serves the long-running pipeline calls (snapshot computation,
	// fixed-deposit processing). Defaults to httpClient.
	longClient *http.Client
}

// NewKuberanClient creates a new Kuberan pipeline API client.
func NewKuberanClient(baseURL, apiKey string, httpClient *http.Client) *KuberanClient {
	return &KuberanClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
		longClient: httpClient,
	}
}

// WithLongRunningClient sets the HTTP client (typically with a longer timeout)
// used for ComputeSnapshots and ProcessFixedDeposits, and returns c.
func (c *KuberanClient) WithLongRunningClient(httpClient *http.Client) *KuberanClient {
	if httpClient != nil {
		c.longClient = httpClient
	}
	return c
}

// GetSecurities fetches all active securities from the pipeline API.
func (c *KuberanClient) GetSecurities(ctx context.Context) ([]Security, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/pipeline/securities", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching securities: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching securities: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Securities []Security `json:"securities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding securities response: %w", err)
	}
	return result.Securities, nil
}

// RecordPrices submits price entries to the pipeline API and returns the count recorded.
func (c *KuberanClient) RecordPrices(ctx context.Context, prices []RecordPriceEntry) (int, error) {
	body := struct {
		Prices []RecordPriceEntry `json:"prices"`
	}{Prices: prices}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshaling prices: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/pipeline/securities/prices", strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("recording prices: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("recording prices: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		PricesRecorded int `json:"prices_recorded"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding prices response: %w", err)
	}
	return result.PricesRecorded, nil
}

// ProcessFixedDepositsResult holds the counts returned by the pipeline FD processing endpoint.
type ProcessFixedDepositsResult struct {
	PayoutsPosted int
	FDsMatured    int
}

// ProcessFixedDeposits triggers scheduled fixed-deposit processing and returns the result counts.
func (c *KuberanClient) ProcessFixedDeposits(ctx context.Context) (ProcessFixedDepositsResult, error) {
	body := struct {
		AsOf string `json:"as_of"`
	}{AsOf: time.Now().UTC().Format(time.RFC3339)}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return ProcessFixedDepositsResult{}, fmt.Errorf("marshaling fixed deposits request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/pipeline/fixed-deposits/process", strings.NewReader(string(jsonBody)))
	if err != nil {
		return ProcessFixedDepositsResult{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.longClient.Do(req)
	if err != nil {
		return ProcessFixedDepositsResult{}, fmt.Errorf("processing fixed deposits: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ProcessFixedDepositsResult{}, fmt.Errorf("processing fixed deposits: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		PayoutsPosted int `json:"payouts_posted"`
		FDsMatured    int `json:"fds_matured"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ProcessFixedDepositsResult{}, fmt.Errorf("decoding fixed deposits response: %w", err)
	}
	return ProcessFixedDepositsResult{
		PayoutsPosted: result.PayoutsPosted,
		FDsMatured:    result.FDsMatured,
	}, nil
}

// ComputeSnapshots triggers portfolio snapshot computation and returns the count recorded.
func (c *KuberanClient) ComputeSnapshots(ctx context.Context) (int, error) {
	body := struct {
		RecordedAt string `json:"recorded_at"`
	}{RecordedAt: time.Now().UTC().Format(time.RFC3339)}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshaling snapshot request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/pipeline/snapshots", strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.longClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("computing snapshots: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("computing snapshots: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		SnapshotsRecorded int `json:"snapshots_recorded"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding snapshots response: %w", err)
	}
	return result.SnapshotsRecorded, nil
}

// CurrencyPair is an ordered (base, quote) ISO 4217 currency pair.
type CurrencyPair struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
}

// FXPairs is the response of the pipeline FX pairs endpoint: the pairs the
// pipeline wants rates for, and the base currencies of active users.
type FXPairs struct {
	Pairs          []CurrencyPair `json:"pairs"`
	BaseCurrencies []string       `json:"base_currencies"`
}

// FXRateEntry is a single exchange rate to submit to the pipeline API. Rate is
// a decimal string so no precision is lost in transit.
type FXRateEntry struct {
	Base       string `json:"base"`
	Quote      string `json:"quote"`
	Rate       string `json:"rate"`
	RecordedAt string `json:"recorded_at"` // RFC3339
	Source     string `json:"source"`
}

// maxFXRatesPerRequest is the largest batch the pipeline FX rates endpoint accepts.
const maxFXRatesPerRequest = 5000

// GetFXPairs fetches the currency pairs and base currencies the pipeline needs rates for.
func (c *KuberanClient) GetFXPairs(ctx context.Context) (FXPairs, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/pipeline/fx-pairs", nil)
	if err != nil {
		return FXPairs{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return FXPairs{}, fmt.Errorf("fetching fx pairs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return FXPairs{}, fmt.Errorf("fetching fx pairs: unexpected status %d", resp.StatusCode)
	}

	var result FXPairs
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return FXPairs{}, fmt.Errorf("decoding fx pairs response: %w", err)
	}
	return result, nil
}

// RecordFXRates submits FX rates to the pipeline API, in batches of at most
// 5000 entries, and returns the total number recorded. On a failed batch it
// returns the count recorded by earlier batches together with the error.
func (c *KuberanClient) RecordFXRates(ctx context.Context, rates []FXRateEntry) (int, error) {
	total := 0
	for start := 0; start < len(rates); start += maxFXRatesPerRequest {
		end := min(start+maxFXRatesPerRequest, len(rates))
		n, err := c.recordFXRatesBatch(ctx, rates[start:end])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// recordFXRatesBatch posts a single batch of FX rates.
func (c *KuberanClient) recordFXRatesBatch(ctx context.Context, rates []FXRateEntry) (int, error) {
	body := struct {
		Rates []FXRateEntry `json:"rates"`
	}{Rates: rates}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshaling fx rates: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/pipeline/fx-rates", strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("recording fx rates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("recording fx rates: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Recorded int `json:"recorded"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding fx rates response: %w", err)
	}
	return result.Recorded, nil
}
