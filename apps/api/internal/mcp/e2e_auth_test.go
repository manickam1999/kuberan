package mcp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
)

// initStatus sends a bare initialize with token and returns the HTTP status and
// the WWW-Authenticate challenge.
func initStatus(e *e2eEnv, token string) (int, string) {
	s := &rpcSession{e: e, token: token}
	code, hdr, _ := s.post(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "e2e", "version": "1"},
		},
	})
	return code, hdr.Get("WWW-Authenticate")
}

func TestE2EBearerGate(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})

	t.Run("no token or an unknown token is a 401 with the discovery challenge", func(t *testing.T) {
		for _, token := range []string{"", "nope"} {
			code, challenge := initStatus(e, token)
			if code != http.StatusUnauthorized {
				t.Errorf("token %q: HTTP %d, want 401", token, code)
			}
			if !strings.Contains(challenge, `resource_metadata="http://mcp.test/.well-known/oauth-protected-resource"`) {
				t.Errorf("token %q: challenge = %q", token, challenge)
			}
		}
	})

	t.Run("a token with a grant row is accepted", func(t *testing.T) {
		if code, _ := initStatus(e, "tok"); code != http.StatusOK {
			t.Errorf("HTTP %d, want 200", code)
		}
	})

	t.Run("a legacy connection with no grant row is refused", func(t *testing.T) {
		e.tokens.add("legacy", &AccessClaims{Subject: e.user.ID, ClientID: "legacy-client", Scopes: allReadScopes})
		if code, _ := initStatus(e, "legacy"); code != http.StatusUnauthorized {
			t.Errorf("HTTP %d, want 401", code)
		}
	})

	t.Run("a token without a client_id is refused", func(t *testing.T) {
		e.tokens.add("anon", &AccessClaims{Subject: e.user.ID, Scopes: allReadScopes})
		if code, _ := initStatus(e, "anon"); code != http.StatusUnauthorized {
			t.Errorf("HTTP %d, want 401", code)
		}
	})

	t.Run("discovery and health need no token", func(t *testing.T) {
		for _, path := range []string{ProtectedResourceMetadataPath, "/health"} {
			resp, err := http.Get(e.ts.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s: HTTP %d", path, resp.StatusCode)
			}
		}
	})
}

func TestE2ERevokedGrantIsRefusedAfterTheCacheExpires(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	s := e.as("tok")
	s.call("list_accounts", nil).ok(t)

	if err := e.grants.Revoke(e.user.ID, e2eClientID); err != nil {
		t.Fatal(err)
	}

	// Within the cache window a read still passes; a write is refused at once
	// because it re-checks the grant in the database.
	s.call("list_accounts", nil).ok(t)
	s.call("create_transaction", map[string]any{
		"account_id": e.acc["maybank"].ID, "type": "expense", "amount": 1, "date": dateStr(0),
	}).fails(t, "not allowed to make changes")

	e.clock.advance(grantTTL + time.Second)
	if code, challenge := initStatus(e, "tok"); code != http.StatusUnauthorized || challenge == "" {
		t.Errorf("after the cache expired: HTTP %d, challenge %q; want 401 with a challenge", code, challenge)
	}
	if r := s.call("list_accounts", nil); r.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("reads on an existing session: HTTP %d, want 401", r.HTTPStatus)
	}
}

func TestE2EReconnectingRestoresAccess(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	if err := e.grants.Revoke(e.user.ID, e2eClientID); err != nil {
		t.Fatal(err)
	}
	e.clock.advance(grantTTL + time.Second)
	if code, _ := initStatus(e, "tok"); code != http.StatusUnauthorized {
		t.Fatalf("revoked: HTTP %d, want 401", code)
	}
	// Consenting again clears revoked_at.
	e.connect("tok", e2eClientID, "Claude Desktop", append(append([]string{}, allReadScopes...), allWriteScopes...), allWriteScopes)
	e.clock.advance(grantTTL + time.Second)
	if code, _ := initStatus(e, "tok"); code != http.StatusOK {
		t.Errorf("after reconnecting: HTTP %d, want 200", code)
	}
}

func TestE2EDeactivatedUserIsRefused(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	e.as("tok").call("list_accounts", nil).ok(t)

	if err := e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	e.clock.advance(activeUserTTL + time.Second)
	if code, _ := initStatus(e, "tok"); code != http.StatusUnauthorized {
		t.Errorf("HTTP %d, want 401", code)
	}
}

func TestE2EReadsRecordLastUse(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	e.connect("tok2", "reader-app", "Reader", allReadScopes, nil)

	lastUsed := func(clientID string) *time.Time {
		var g models.OAuthClientGrant
		if err := e.db.Where("user_id = ? AND client_id = ?", e.user.ID, clientID).First(&g).Error; err != nil {
			t.Fatal(err)
		}
		return g.LastUsedAt
	}
	if lastUsed("reader-app") != nil {
		t.Fatal("a fresh grant must not have a last use")
	}
	e.as("tok2").call("list_categories", nil).ok(t)
	if lastUsed("reader-app") == nil {
		t.Error("a read must record the app's last use")
	}
	if lastUsed(e2eClientID) != nil {
		t.Error("another app's grant must not be touched")
	}

	// A refused (revoked) app is not touched.
	e.connect("tok3", "gone-app", "Gone", allReadScopes, nil)
	if err := e.grants.Revoke(e.user.ID, "gone-app"); err != nil {
		t.Fatal(err)
	}
	if code, _ := initStatus(e, "tok3"); code != http.StatusUnauthorized {
		t.Fatalf("HTTP %d, want 401", code)
	}
	if lastUsed("gone-app") != nil {
		t.Error("a refused request must not record a use")
	}
}

func TestE2EReadScopes(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})

	readTools := map[string]struct {
		args  map[string]any
		scope string
	}{
		"list_accounts":            {nil, "read:accounts"},
		"list_categories":          {nil, "read:categories"},
		"list_transactions":        {nil, "read:transactions"},
		"get_spending_by_category": {map[string]any{"from_date": dateStr(30), "to_date": dateStr(0)}, "read:transactions"},
		"get_monthly_summary":      {nil, "read:transactions"},
		"get_daily_spending":       {map[string]any{"from_date": dateStr(3), "to_date": dateStr(0)}, "read:transactions"},
		"get_daily_summary":        {map[string]any{"from_date": dateStr(3), "to_date": dateStr(0)}, "read:transactions"},
		"get_top_expenses":         {map[string]any{"from_date": dateStr(30), "to_date": dateStr(0)}, "read:transactions"},
		"list_budgets":             {nil, "read:budgets"},
		"get_budget_progress":      {map[string]any{"budget_id": "x"}, "read:budgets"},
		"get_portfolio":            {nil, "read:investments"},
		"get_net_worth_history":    {map[string]any{"from_date": dateStr(30), "to_date": dateStr(0)}, "read:snapshots"},
		"list_receivables":         {nil, "read:transactions"},
		"get_receivables_summary":  {nil, "read:transactions"},
	}

	i := 0
	for tool, tc := range readTools {
		// A token holding only the tool's scope passes the scope check; one holding
		// only an unrelated scope is refused.
		i++
		onlyThis := "scope-" + tool
		e.connect(onlyThis, "app-"+tool, tool, []string{tc.scope}, nil)
		other := "other-" + tool
		unrelated := "read:categories"
		if tc.scope == unrelated {
			unrelated = "read:budgets"
		}
		e.connect(other, "other-app-"+tool, tool, []string{unrelated}, nil)

		r := e.as(onlyThis).call(tool, tc.args)
		if r.IsError && strings.Contains(r.Text, "missing required scope") {
			t.Errorf("%s: refused with its own scope %s: %s", tool, tc.scope, r.Text)
		}
		e.as(other).call(tool, tc.args).fails(t, "forbidden: missing required scope")
	}

	t.Run("get_portfolio also accepts read:portfolio", func(t *testing.T) {
		e.connect("pf", "pf-app", "pf", []string{"read:portfolio"}, nil)
		e.as("pf").call("get_portfolio", nil).ok(t)
	})
}

func TestE2EWriteGating(t *testing.T) {
	args := func(e *e2eEnv) map[string]any {
		return map[string]any{"account_id": e.acc["maybank"].ID, "type": "expense", "amount": 1.5, "date": dateStr(0)}
	}

	t.Run("a token without the write scope is refused", func(t *testing.T) {
		e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
		e.connect("ro", "ro-app", "RO", allReadScopes, nil)
		before := e.txnCount()
		e.as("ro").call("create_transaction", args(e)).fails(t, "missing required scope (write:transactions)")
		if e.txnCount() != before {
			t.Error("a refused write must not create a row")
		}
	})

	t.Run("a write scope in the token that the user never approved is refused", func(t *testing.T) {
		e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
		e.connect("tampered", "t-app", "T", append(append([]string{}, allReadScopes...), "write:transactions"), nil)
		before := e.txnCount()
		e.as("tampered").call("create_transaction", args(e)).fails(t, "not allowed to make changes")
		if e.txnCount() != before {
			t.Error("a refused write must not create a row")
		}
	})

	t.Run("each write tool needs its own scope", func(t *testing.T) {
		e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
		e.connect("budgets-only", "b-app", "B", append(append([]string{}, allReadScopes...), "write:budgets"), []string{"write:budgets"})
		s := e.as("budgets-only")
		s.call("create_category", map[string]any{"name": "X", "type": "expense"}).fails(t, "write:categories")
		s.call("create_transaction", args(e)).fails(t, "write:transactions")
		s.call("write_off_receivable", map[string]any{"receivable_id": "r"}).fails(t, "write:transactions")
		s.call("create_budget", map[string]any{
			"category_id": e.cat["transport"].ID, "name": "Transport", "amount": 50, "period": "monthly",
		}).ok(t)
	})

	t.Run("a deactivated user cannot write even inside the cache window", func(t *testing.T) {
		e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
		s := e.as("tok")
		s.call("list_accounts", nil).ok(t) // primes the user cache as active
		if err := e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("is_active", false).Error; err != nil {
			t.Fatal(err)
		}
		s.call("create_transaction", args(e)).fails(t, "not allowed to make changes")
	})

	t.Run("with writes disabled the write tools do not exist", func(t *testing.T) {
		e := newE2E(t, e2eOptions{write: WriteConfig{}})
		s := e.as("tok")
		tools := s.listTools()
		for _, name := range []string{
			"create_transaction", "create_transfer", "update_transaction", "categorize_transactions",
			"create_category", "update_category", "create_budget", "update_budget",
			"record_repayment", "write_off_receivable",
		} {
			if _, ok := tools[name]; ok {
				t.Errorf("%s is listed although writes are disabled", name)
			}
		}
		if r := s.call("create_transaction", args(e)); r.RPCError == "" && !r.IsError {
			t.Errorf("create_transaction succeeded with writes disabled: %+v", r)
		}
	})
}

func TestE2EToolInventory(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	tools := e.as("tok").listTools()
	want := []string{
		"list_accounts", "list_categories", "list_transactions", "get_spending_by_category", "get_monthly_summary",
		"get_daily_spending", "get_daily_summary", "get_top_expenses", "list_budgets", "get_budget_progress",
		"get_portfolio", "get_net_worth_history", "list_receivables", "get_receivables_summary",
		"create_transaction", "create_transfer", "update_transaction", "categorize_transactions",
		"create_category", "update_category", "create_budget", "update_budget",
		"record_repayment", "write_off_receivable",
	}
	for _, name := range want {
		if tools[name] == "" {
			t.Errorf("tool %s is missing or has no description", name)
		}
	}
	if len(tools) != len(want) {
		t.Errorf("%d tools registered, want %d", len(tools), len(want))
	}
}
