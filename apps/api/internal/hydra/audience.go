package hydra

import (
	"context"
	"fmt"
	"slices"

	"kuberan/internal/logger"
)

// EnsureClientAudience adds audience to every registered client that lacks
// it, and returns how many it changed.
//
// The consent bridge grants the MCP resource URL as the audience of every
// access token, and Hydra re-checks a token's granted audience against the
// client's allowed audiences on each refresh. Clients registered before the
// DCR proxy set that audience can log in but never refresh, so they are
// disconnected every 15 minutes. Patching them makes their existing refresh
// tokens valid again without a new login. Safe to run on every start.
func EnsureClientAudience(ctx context.Context, admin AdminClient, audience string) (int, error) {
	if audience == "" {
		return 0, nil
	}
	clients, err := admin.ListClients(ctx)
	if err != nil {
		return 0, fmt.Errorf("list OAuth clients: %w", err)
	}
	if len(clients) >= maxListedClients {
		logger.Get().Warnw("more OAuth clients than one page; only the first page was checked for the MCP audience",
			"page_size", maxListedClients)
	}

	patched := 0
	for i := range clients {
		cl := &clients[i]
		if slices.Contains(cl.Audience, audience) {
			continue
		}
		next := append(slices.Clone(cl.Audience), audience)
		if err := admin.SetClientAudience(ctx, cl.ClientID, next); err != nil {
			return patched, fmt.Errorf("set audience on OAuth client %s: %w", cl.ClientID, err)
		}
		patched++
	}
	return patched, nil
}
