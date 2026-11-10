package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/testutil"
)

// End-to-end harness: the real MCP HTTP handler (bearer gate, user gate, grant
// gate, JSON-RPC transport, tools) with the real services on the SQLite test
// DB. Only token validation is replaced (no Hydra): a table maps bearer strings
// to claims. Every test calls tools through JSON-RPC tools/call.

// e2eClientID is the OAuth client the default token belongs to.
const e2eClientID = "claude-desktop"

// allReadScopes and allWriteScopes are the scopes a fully connected app holds.
var (
	allReadScopes = []string{
		"read:accounts", "read:transactions", "read:categories", "read:budgets",
		"read:investments", "read:portfolio", "read:snapshots",
	}
	allWriteScopes = []string{"write:transactions", "write:categories", "write:budgets"}
)

// tokenTable is the test TokenValidator.
type tokenTable struct {
	mu     sync.Mutex
	tokens map[string]*AccessClaims
}

func (t *tokenTable) Validate(_ context.Context, raw string) (*AccessClaims, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.tokens[raw]
	if !ok {
		return nil, errors.New("unknown token")
	}
	return c, nil
}

func (t *tokenTable) add(token string, c *AccessClaims) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens[token] = c
}

// fakeClock drives the user and grant caches.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// e2eEnv is a running MCP server with a seeded multi-currency user.
type e2eEnv struct {
	t      *testing.T
	db     *gorm.DB
	ts     *httptest.Server
	tokens *tokenTable
	clock  *fakeClock
	svc    Services
	grants services.OAuthGrantServicer
	user   *models.User

	// seeded records, by short name
	acc map[string]*models.Account
	cat map[string]*models.Category
}

// e2eOptions tune the server under test.
type e2eOptions struct {
	write WriteConfig
	// noSeed skips the household (an empty MYR user).
	noSeed bool
}

func defaultWriteConfig() WriteConfig {
	return WriteConfig{Enabled: true, MaxAmount: 1_000_000, RatePerMinute: 30, RatePerDay: 500}
}

// newE2E starts the server. The default token ("tok") belongs to the user and
// e2eClientID, holds every read and write scope, and the user has granted the
// client every scope.
func newE2E(t *testing.T, opts e2eOptions) *e2eEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })

	// Mirror cmd/mcp/main.go: one FX service shared by all services.
	fxService := services.NewFXService(db, 168*time.Hour)
	accounts := services.NewAccountService(db, services.WithFXService(fxService))
	categories := services.NewCategoryService(db)
	rules := services.NewRuleService(db, categories)
	grants := services.NewOAuthGrantService(db)
	svc := Services{
		Users:        services.NewUserService(db, fxService),
		Accounts:     accounts,
		Categories:   categories,
		Transactions: services.NewTransactionService(db, accounts, rules, services.WithTransactionFXService(fxService)),
		Budgets:      services.NewBudgetService(db, services.WithBudgetFXService(fxService)),
		Investments:  services.NewInvestmentService(db, accounts, fxService),
		Snapshots:    services.NewPortfolioSnapshotService(db, fxService),
		Receivables:  services.NewReceivableService(db, accounts, rules, services.WithReceivableFXService(fxService)),
		FX:           fxService,
		Audit:        services.NewAuditService(db),
		Grants:       grants,
	}

	user := testutil.CreateTestUser(t, db)
	if err := db.Model(user).Update("base_currency", "MYR").Error; err != nil {
		t.Fatal(err)
	}
	user.BaseCurrency = "MYR"

	tokens := &tokenTable{tokens: map[string]*AccessClaims{}}
	clock := &fakeClock{now: time.Now()}
	handler, transport := buildHandler(svc,
		OAuthConfig{Issuer: "http://hydra.test", ResourceURL: "http://mcp.test", Scopes: strings.Join(allReadScopes, " ")},
		tokens, opts.write, nil,
		handlerOptions{now: clock.Now, background: func(f func()) { f() }})
	ts := httptest.NewServer(handler)
	t.Cleanup(func() {
		ts.Close()
		_ = transport.Shutdown(context.Background())
	})

	e := &e2eEnv{
		t: t, db: db, ts: ts, tokens: tokens, clock: clock, svc: svc, grants: grants, user: user,
		acc: map[string]*models.Account{}, cat: map[string]*models.Category{},
	}
	e.connect("tok", e2eClientID, "Claude Desktop", append(append([]string{}, allReadScopes...), allWriteScopes...), allWriteScopes)
	if !opts.noSeed {
		e.seedHousehold()
	}
	return e
}

// connect issues a token and records the user's consent grant for clientID.
// approvedWrites are the write scopes the user ticked (reads are implied).
func (e *e2eEnv) connect(token, clientID, name string, tokenScopes, approvedWrites []string) {
	e.t.Helper()
	e.tokens.add(token, &AccessClaims{Subject: e.user.ID, Scopes: tokenScopes, ClientID: clientID})
	approved := append(append([]string{}, allReadScopes...), approvedWrites...)
	if _, err := e.grants.RecordConsent(services.RecordConsentInput{
		UserID: e.user.ID, ClientID: clientID, ClientName: name, Approved: approved, Reviewed: approved,
	}); err != nil {
		e.t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// JSON-RPC client

// rpcSession is an initialized MCP session for one bearer token.
type rpcSession struct {
	e     *e2eEnv
	token string
	sid   string
	next  int
}

// as opens a session for the token (initialize + initialized).
func (e *e2eEnv) as(token string) *rpcSession {
	e.t.Helper()
	s := &rpcSession{e: e, token: token}
	code, hdr, body := s.post(map[string]any{
		"jsonrpc": "2.0", "id": s.id(), "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "e2e", "version": "1"},
		},
	})
	if code != http.StatusOK {
		e.t.Fatalf("initialize: HTTP %d: %s", code, body)
	}
	s.sid = hdr.Get("Mcp-Session-Id")
	s.post(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return s
}

func (s *rpcSession) id() int {
	s.next++
	return s.next
}

// post sends one JSON-RPC message and returns the status, headers and the JSON
// body (an SSE reply is unwrapped to its data payload).
func (s *rpcSession) post(msg map[string]any) (status int, header http.Header, body []byte) {
	s.e.t.Helper()
	payload, _ := json.Marshal(msg)
	req, err := http.NewRequest(http.MethodPost, s.e.ts.URL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		s.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	if s.sid != "" {
		req.Header.Set("Mcp-Session-Id", s.sid)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		sc := bufio.NewScanner(bytes.NewReader(raw))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data:"); ok {
				raw = []byte(strings.TrimSpace(data))
				break
			}
		}
	}
	return resp.StatusCode, resp.Header, raw
}

// toolResult is a decoded tools/call result.
type toolResult struct {
	Text    string
	IsError bool
	// HTTPStatus is the transport status; anything but 200 means the request
	// never reached the tool.
	HTTPStatus int
	// RPCError is the JSON-RPC error message, if any (unknown tool and so on).
	RPCError string
}

// call runs tools/call and decodes the result.
func (s *rpcSession) call(tool string, args map[string]any) toolResult {
	s.e.t.Helper()
	code, _, body := s.post(map[string]any{
		"jsonrpc": "2.0", "id": s.id(), "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	out := toolResult{HTTPStatus: code}
	if code != http.StatusOK {
		out.Text = string(body)
		return out
	}
	var resp struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		s.e.t.Fatalf("%s: undecodable reply %q: %v", tool, body, err)
	}
	if resp.Error != nil {
		out.RPCError = resp.Error.Message
		return out
	}
	if resp.Result == nil {
		s.e.t.Fatalf("%s: reply has neither result nor error: %s", tool, body)
	}
	var texts []string
	for _, c := range resp.Result.Content {
		texts = append(texts, c.Text)
	}
	out.Text = strings.Join(texts, "\n")
	out.IsError = resp.Result.IsError
	return out
}

// listTools returns the registered tool names and their descriptions.
func (s *rpcSession) listTools() map[string]string {
	s.e.t.Helper()
	code, _, body := s.post(map[string]any{"jsonrpc": "2.0", "id": s.id(), "method": "tools/list"})
	if code != http.StatusOK {
		s.e.t.Fatalf("tools/list: HTTP %d: %s", code, body)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		s.e.t.Fatal(err)
	}
	out := map[string]string{}
	for _, tl := range resp.Result.Tools {
		out[tl.Name] = tl.Description
	}
	return out
}

// ok asserts a successful tool call and returns its text.
func (r toolResult) ok(t *testing.T) string {
	t.Helper()
	if r.HTTPStatus != http.StatusOK || r.RPCError != "" || r.IsError {
		t.Fatalf("expected success, got HTTP %d, rpc error %q, isError %t:\n%s", r.HTTPStatus, r.RPCError, r.IsError, r.Text)
	}
	return r.Text
}

// fails asserts a tool-level error containing want and returns its text.
func (r toolResult) fails(t *testing.T, want string) string {
	t.Helper()
	if r.HTTPStatus != http.StatusOK || r.RPCError != "" || !r.IsError {
		t.Fatalf("expected a tool error containing %q, got HTTP %d, rpc error %q, isError %t:\n%s",
			want, r.HTTPStatus, r.RPCError, r.IsError, r.Text)
	}
	if !strings.Contains(r.Text, want) {
		t.Fatalf("tool error %q does not contain %q", r.Text, want)
	}
	return r.Text
}

// contains asserts every fragment appears in text.
func contains(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if !strings.Contains(text, f) {
			t.Errorf("output is missing %q:\n%s", f, text)
		}
	}
}

// lacks asserts no fragment appears in text.
func lacks(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if strings.Contains(text, f) {
			t.Errorf("output must not contain %q:\n%s", f, text)
		}
	}
}

// idAfter returns the first "ID: <id>" value following marker in text.
func idAfter(t *testing.T, text, marker string) string {
	t.Helper()
	i := strings.Index(text, marker)
	if i < 0 {
		t.Fatalf("%q not found in:\n%s", marker, text)
	}
	rest := text[i:]
	j := strings.Index(rest, "ID: ")
	if j < 0 {
		t.Fatalf("no ID after %q in:\n%s", marker, text)
	}
	rest = rest[j+len("ID: "):]
	end := strings.IndexAny(rest, " \n")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}

// ---------------------------------------------------------------------------
// Seed data

func mustRate(t *testing.T, s string) fx.Rate {
	t.Helper()
	r, err := fx.ParseRate(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// day returns midnight UTC n days ago.
func day(n int) time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -n)
}

func dateStr(n int) string { return day(n).Format("2006-01-02") }

// seedHousehold builds an MYR-base household:
//
//	cash: Maybank MYR 5,000.00; Wise USD 1,000.00; DBS SGD 2,000.00; Revolut EUR 300.00 (no rate)
//	      Old Wallet MYR 50.00 (inactive)
//	card: Visa MYR owing 1,200.00; loan: Car loan MYR owing 30,000.00
//	fixed deposit: FD MYR 10,000.00; investment: IBKR with a USD and a MYR holding
//	rates: USD/MYR 4.5, SGD/MYR 3.3
func (e *e2eEnv) seedHousehold() {
	t := e.t
	uid := e.user.ID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	rateAt := time.Now().Add(-48 * time.Hour)
	_, err := e.svc.FX.RecordRates([]services.FXRateInput{
		{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.5"), RecordedAt: rateAt},
		{Base: "SGD", Quote: "MYR", Rate: mustRate(t, "3.3"), RecordedAt: rateAt},
	})
	must(err)

	mk := func(key string, a *models.Account, err error) {
		t.Helper()
		must(err)
		e.acc[key] = a
	}
	a, err := e.svc.Accounts.CreateCashAccount(uid, "Maybank", "", "MYR", 500_000)
	mk("maybank", a, err)
	a, err = e.svc.Accounts.CreateCashAccount(uid, "Wise USD", "", "USD", 100_000)
	mk("wise", a, err)
	a, err = e.svc.Accounts.CreateCashAccount(uid, "DBS SGD", "", "SGD", 200_000)
	mk("dbs", a, err)
	a, err = e.svc.Accounts.CreateCashAccount(uid, "Revolut EUR", "", "EUR", 30_000)
	mk("revolut", a, err)
	a, err = e.svc.Accounts.CreateCashAccount(uid, "Old Wallet", "", "MYR", 5_000)
	mk("old", a, err)
	must(e.db.Model(a).Update("is_active", false).Error)
	a, err = e.svc.Accounts.CreateCreditCardAccountWithParams(uid, services.CreateCreditCardParams{
		Name: "Visa", Currency: "MYR", CreditLimit: 1_000_000, CurrentBalance: 120_000, DueDay: 20,
	})
	mk("visa", a, err)
	a, err = e.svc.Accounts.CreateDebtAccount(uid, services.CreateDebtParams{
		Name: "Car loan", Currency: "MYR", AmountOwed: 3_000_000, InterestRate: 3.2, MonthlyPayment: 90_000, DueDay: 5,
	})
	mk("loan", a, err)
	a, err = e.svc.Accounts.CreateFixedDepositAccount(uid, services.CreateFixedDepositParams{
		Name: "FD", Currency: "MYR", SourceAccountID: e.acc["maybank"].ID, Principal: 100_000, InterestRate: 3.5,
		StartDate: day(30), MaturityDate: day(-335), PayoutType: models.PayoutTypeCumulative, Compounding: models.CompoundingAnnual,
	})
	mk("fd", a, err)
	a, err = e.svc.Accounts.CreateInvestmentAccount(uid, "IBKR", "", "USD", "IBKR", "U123")
	mk("ibkr", a, err)

	// Categories.
	for _, c := range []struct {
		key, name string
		typ       models.CategoryType
	}{
		{"groceries", "Groceries", models.CategoryTypeExpense},
		{"dining", "Dining", models.CategoryTypeExpense},
		{"transport", "Transport", models.CategoryTypeExpense},
		{"salary", "Salary", models.CategoryTypeIncome},
	} {
		cat, err := e.svc.Categories.CreateCategory(uid, c.name, c.typ, "", "", "", nil)
		must(err)
		e.cat[c.key] = cat
	}

	// Securities and holdings: AAPL in USD, MAYBANK in MYR, both priced.
	aapl := testutil.CreateTestSecurityWithParams(t, e.db, "AAPL", "Apple Inc", models.AssetTypeStock, "NASDAQ")
	must(e.db.Model(aapl).Update("currency", "USD").Error)
	bank := testutil.CreateTestSecurityWithParams(t, e.db, "1155", "Malayan Banking", models.AssetTypeStock, "BURSA")
	for _, p := range []struct {
		sec   *models.Security
		cents int64
		cur   string
	}{{aapl, 20_000, "USD"}, {bank, 1_000, "MYR"}} {
		must(e.db.Create(&models.SecurityPrice{SecurityID: p.sec.ID, Price: p.cents, Currency: p.cur, RecordedAt: time.Now().Add(-time.Hour)}).Error)
	}
	_, err = e.svc.Investments.AddInvestment(uid, services.AddInvestmentParams{
		AccountID: e.acc["ibkr"].ID, SecurityID: aapl.ID, Quantity: 10, PurchasePrice: 15_000, Date: ptrTime(day(20)),
		FXRate: "4.4",
	})
	must(err)
	_, err = e.svc.Investments.AddInvestment(uid, services.AddInvestmentParams{
		AccountID: e.acc["ibkr"].ID, SecurityID: bank.ID, Quantity: 100, PurchasePrice: 900, Date: ptrTime(day(20)),
	})
	must(err)

	// Transactions: salary, groceries, a USD expense, a cross-currency transfer.
	meta := services.TransactionMeta{}
	_, err = e.svc.Transactions.CreateTransaction(uid, e.acc["maybank"].ID, ptr(e.cat["salary"].ID), models.TransactionTypeIncome,
		600_000, "Monthly salary", day(5), meta)
	must(err)
	_, err = e.svc.Transactions.CreateTransaction(uid, e.acc["maybank"].ID, ptr(e.cat["groceries"].ID), models.TransactionTypeExpense,
		25_050, "Jaya Grocer", day(3), meta)
	must(err)
	_, err = e.svc.Transactions.CreateTransaction(uid, e.acc["wise"].ID, ptr(e.cat["dining"].ID), models.TransactionTypeExpense,
		4_000, "Cafe in Austin", day(2), meta)
	must(err)
	_, err = e.svc.Transactions.CreateTransaction(uid, e.acc["revolut"].ID, ptr(e.cat["dining"].ID), models.TransactionTypeExpense,
		1_500, "Paris bakery", day(2), meta)
	must(err)
	toAmt := int64(10_000)
	_, err = e.svc.Transactions.CreateTransfer(uid, e.acc["maybank"].ID, e.acc["wise"].ID, 45_000, &toAmt, "Top up Wise", day(4), meta)
	must(err)

	// A rule: descriptions containing "grab" are Transport.
	rules := services.NewRuleService(e.db, e.svc.Categories)
	_, err = rules.CreateRule(uid, services.CreateRuleInput{
		Name:       "Grab is transport",
		Conditions: []services.RuleConditionInput{{Field: models.RuleFieldDescription, Operator: models.RuleOpContains, ValueText: "grab"}},
		Actions:    []services.RuleActionInput{{ActionType: models.RuleActionSetCategory, CategoryID: ptr(e.cat["transport"].ID)}},
	})
	must(err)

	// A budget and a shared expense (Alice owes half of a dinner).
	_, err = e.svc.Budgets.CreateBudget(uid, e.cat["groceries"].ID, "Groceries budget", 80_000, models.BudgetPeriodMonthly)
	must(err)
	_, err = e.svc.Receivables.CreateSharedExpense(uid, services.CreateSharedExpenseInput{
		AccountID: e.acc["maybank"].ID, CategoryID: ptr(e.cat["dining"].ID), Amount: 12_000, Description: "Team dinner",
		Date: day(1), Kind: models.ReceivableKindSplit, Mode: services.ShareModeEqual,
		Counterparties: []services.SharedCounterpartyInput{{Name: "Alice"}},
	})
	must(err)
	// A USD loan to Bob: lent from the USD account (a receivable in USD).
	_, err = e.svc.Receivables.CreateSharedExpense(uid, services.CreateSharedExpenseInput{
		AccountID: e.acc["wise"].ID, Amount: 20_000, Description: "Lent to Bob", Date: day(1),
		Kind: models.ReceivableKindLoan, Mode: services.ShareModeFull,
		Counterparties: []services.SharedCounterpartyInput{{Name: "Bob"}},
	})
	must(err)
}

func ptr[T any](v T) *T { return &v }

func ptrTime(t time.Time) *time.Time { return &t }

// reload returns the account's current row.
func (e *e2eEnv) reload(a *models.Account) *models.Account {
	e.t.Helper()
	var out models.Account
	if err := e.db.First(&out, "id = ?", a.ID).Error; err != nil {
		e.t.Fatal(err)
	}
	return &out
}

// auditRows returns audit rows for an action, oldest first.
func (e *e2eEnv) auditRows(action string) []models.AuditLog {
	e.t.Helper()
	var rows []models.AuditLog
	if err := e.db.Where("action = ?", action).Order("created_at ASC").Find(&rows).Error; err != nil {
		e.t.Fatal(err)
	}
	return rows
}

// auditChanges decodes an audit row's changes.
func auditChanges(t *testing.T, row models.AuditLog) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(row.Changes), &m); err != nil {
		t.Fatalf("audit changes %q: %v", row.Changes, err)
	}
	return m
}

func (e *e2eEnv) txnCount() int64 {
	var n int64
	e.db.Model(&models.Transaction{}).Count(&n)
	return n
}
