package hydra

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeClients is an AdminClient stub holding a client list in memory.
type fakeClients struct {
	AdminClient
	clients []OAuth2Client
	listErr error
	setErr  error
	set     map[string][]string
}

func (f *fakeClients) ListClients(context.Context) ([]OAuth2Client, error) {
	return f.clients, f.listErr
}

func (f *fakeClients) SetClientAudience(_ context.Context, id string, audience []string) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.set == nil {
		f.set = map[string][]string{}
	}
	f.set[id] = audience
	return nil
}

func TestEnsureClientAudience(t *testing.T) {
	const aud = "https://mcp.example"

	t.Run("adds the audience only where it is missing, keeping others", func(t *testing.T) {
		f := &fakeClients{clients: []OAuth2Client{
			{ClientID: "has", Audience: []string{aud}},
			{ClientID: "none"},
			{ClientID: "other", Audience: []string{"https://other.example"}},
		}}
		n, err := EnsureClientAudience(context.Background(), f, aud)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("patched %d clients, want 2", n)
		}
		if _, touched := f.set["has"]; touched {
			t.Error("a client that already allows the audience must not be patched")
		}
		if !slices.Equal(f.set["none"], []string{aud}) {
			t.Errorf("none: %v", f.set["none"])
		}
		if !slices.Equal(f.set["other"], []string{"https://other.example", aud}) {
			t.Errorf("other: %v, want existing audience kept", f.set["other"])
		}
	})

	t.Run("no audience configured does nothing", func(t *testing.T) {
		f := &fakeClients{clients: []OAuth2Client{{ClientID: "none"}}}
		if n, err := EnsureClientAudience(context.Background(), f, ""); n != 0 || err != nil || f.set != nil {
			t.Errorf("got %d, %v, %v", n, err, f.set)
		}
	})

	t.Run("errors are returned", func(t *testing.T) {
		boom := errors.New("boom")
		if _, err := EnsureClientAudience(context.Background(), &fakeClients{listErr: boom}, aud); !errors.Is(err, boom) {
			t.Errorf("list error: %v", err)
		}
		f := &fakeClients{clients: []OAuth2Client{{ClientID: "none"}}, setErr: boom}
		if _, err := EnsureClientAudience(context.Background(), f, aud); !errors.Is(err, boom) {
			t.Errorf("patch error: %v", err)
		}
	})
}
