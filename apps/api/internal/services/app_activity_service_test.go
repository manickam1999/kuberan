package services

import (
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// activityEnv is a database with two users, each with a grant for client
// "claude", an audit service and the activity service under test.
type activityEnv struct {
	db     *gorm.DB
	audit  AuditServicer
	svc    AppActivityServicer
	userA  *models.User
	userB  *models.User
	grants OAuthGrantServicer
}

func newActivityEnv(t *testing.T) *activityEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	env := &activityEnv{
		db:     db,
		audit:  NewAuditService(db),
		svc:    NewAppActivityService(db),
		userA:  testutil.CreateTestUser(t, db),
		userB:  testutil.CreateTestUser(t, db),
		grants: NewOAuthGrantService(db),
	}
	for _, u := range []*models.User{env.userA, env.userB} {
		_, err := env.grants.RecordConsent(RecordConsentInput{
			UserID: u.ID, ClientID: "claude", ClientName: "Claude",
			Approved: []string{"write:transactions"}, Reviewed: []string{"write:transactions"},
		})
		testutil.AssertNoError(t, err)
	}
	return env
}

// mcpLog writes an MCP audit row the way the MCP write wrapper does.
func (e *activityEnv) mcpLog(userID, clientID, action, resourceType, resourceID string, changes map[string]any) {
	e.audit.Log(userID, action, resourceType, resourceID, "127.0.0.1", changes,
		LogOpts{Source: models.AuditSourceMCP, ClientID: clientID})
}

func TestAuditLogSource(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	user := testutil.CreateTestUser(t, db)
	audit := NewAuditService(db)

	audit.Log(user.ID, "LOGIN", "user", user.ID, "1.1.1.1", nil)
	audit.Log(user.ID, "MCP_X", "transaction", "", "1.1.1.1", nil, LogOpts{Source: "mcp", ClientID: "cid"})

	var rows []models.AuditLog
	testutil.AssertNoError(t, db.Order("created_at ASC, id ASC").Find(&rows).Error)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	byAction := map[string]models.AuditLog{rows[0].Action: rows[0], rows[1].Action: rows[1]}
	if got := byAction["LOGIN"]; got.Source != "web" || got.ClientID != "" {
		t.Errorf("a call without opts must record web and no client, got %q/%q", got.Source, got.ClientID)
	}
	if got := byAction["MCP_X"]; got.Source != "mcp" || got.ClientID != "cid" {
		t.Errorf("expected mcp/cid, got %q/%q", got.Source, got.ClientID)
	}
}

// TestAuditSourceBackfill runs the migration's backfill UPDATE (translated from
// Postgres jsonb operators to SQLite json_extract) over rows that include the
// empty changes a logout writes, to prove the cast guard.
func TestAuditSourceBackfill(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000050_add_source_to_audit_logs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	start := strings.Index(sql, "UPDATE audit_logs")
	if start < 0 {
		t.Fatal("backfill UPDATE not found in the migration")
	}
	update := sql[start:]
	if !strings.Contains(update, "CASE WHEN changes LIKE '{%' THEN changes::jsonb->>'via' END = 'mcp'") {
		t.Fatal("the backfill must guard the jsonb cast with CASE WHEN changes LIKE '{%'")
	}
	update = strings.ReplaceAll(update, "changes::jsonb->>'via'", "json_extract(changes, '$.via')")
	update = strings.ReplaceAll(update, "changes::jsonb->>'client_id'", "json_extract(changes, '$.client_id')")

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	user := testutil.CreateTestUser(t, db)
	insert := func(action, changes string) string {
		row := &models.AuditLog{UserID: &user.ID, Action: action, ResourceType: "x", Changes: changes, Source: "web"}
		testutil.AssertNoError(t, db.Create(row).Error)
		return row.ID
	}
	mcpID := insert("MCP_CREATE_TRANSACTION", `{"via":"mcp","client_id":"cid-1","amount":5}`)
	mcpNoClient := insert("MCP_UPDATE_BUDGET", `{"via":"mcp"}`)
	emptyID := insert("LOGOUT", "")
	webID := insert("LOGIN", `{"via":"password"}`)
	plainID := insert("ODD", "not json at all")
	noViaID := insert("OTHER", `{"amount":5}`)

	testutil.AssertNoError(t, db.Exec(strings.TrimSuffix(strings.TrimSpace(update), ";")).Error)

	check := func(id, wantSource, wantClient string) {
		t.Helper()
		var row models.AuditLog
		testutil.AssertNoError(t, db.First(&row, "id = ?", id).Error)
		if row.Source != wantSource || row.ClientID != wantClient {
			t.Errorf("%s: got %q/%q, want %q/%q", row.Action, row.Source, row.ClientID, wantSource, wantClient)
		}
	}
	check(mcpID, "mcp", "cid-1")
	check(mcpNoClient, "mcp", "")
	check(emptyID, "web", "")
	check(webID, "web", "")
	check(plainID, "web", "")
	check(noViaID, "web", "")
}

func TestAppActivity_Scoping(t *testing.T) {
	env := newActivityEnv(t)
	env.mcpLog(env.userA.ID, "claude", "MCP_CREATE_BUDGET", "budget", "", map[string]any{"name": "Food"})
	env.mcpLog(env.userB.ID, "claude", "MCP_CREATE_BUDGET", "budget", "", map[string]any{"name": "Secret B"})
	// A web audit row for A and another app's row are not in claude's log.
	env.audit.Log(env.userA.ID, "LOGIN", "user", env.userA.ID, "", nil)
	env.mcpLog(env.userA.ID, "other", "MCP_CREATE_CATEGORY", "category", "", map[string]any{"name": "Fun"})

	perApp, err := env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if perApp.TotalItems != 1 || perApp.Data[0].Summary != "Created budget · Food" {
		t.Fatalf("user A must see only their own claude rows, got %+v", perApp.Data)
	}

	// User B asking for A's data via the same client id sees only their own.
	perAppB, err := env.svc.ListByClient(env.userB.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if perAppB.TotalItems != 1 || strings.Contains(perAppB.Data[0].Summary, "Food") {
		t.Fatalf("user B must not see user A's rows, got %+v", perAppB.Data)
	}

	all, err := env.svc.ListAll(env.userA.ID, pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if all.TotalItems != 2 {
		t.Fatalf("expected A's 2 app rows across apps, got %d", all.TotalItems)
	}
	for _, item := range all.Data {
		if strings.Contains(item.Summary, "Secret B") {
			t.Fatalf("user B's row leaked across apps: %+v", item)
		}
		if item.ClientID == "claude" && item.ClientName != "Claude" {
			t.Errorf("expected the grant's name, got %q", item.ClientName)
		}
		if item.ClientID == "other" && item.ClientName != "an app" {
			t.Errorf("an app with no grant is 'an app', got %q", item.ClientName)
		}
	}

	allB, err := env.svc.ListAll(env.userB.ID, pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if allB.TotalItems != 1 {
		t.Fatalf("expected B's 1 row, got %d", allB.TotalItems)
	}
}

func TestAppActivity_PagingNewestFirst(t *testing.T) {
	env := newActivityEnv(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i, name := range []string{"one", "two", "three"} {
		env.mcpLog(env.userA.ID, "claude", "MCP_CREATE_CATEGORY", "category", "", map[string]any{"name": name})
		testutil.AssertNoError(t, env.db.Model(&models.AuditLog{}).
			Where("changes LIKE ?", "%"+name+"%").
			UpdateColumn("created_at", base.Add(time.Duration(i)*time.Hour)).Error)
	}
	page, err := env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{Page: 1, PageSize: 2})
	testutil.AssertNoError(t, err)
	if page.TotalItems != 3 || page.TotalPages != 2 || len(page.Data) != 2 {
		t.Fatalf("unexpected paging: %+v", page)
	}
	if page.Data[0].Summary != "Created category · three" || page.Data[1].Summary != "Created category · two" {
		t.Errorf("expected newest first, got %q, %q", page.Data[0].Summary, page.Data[1].Summary)
	}
}

func TestBuildActivitySummary(t *testing.T) {
	long := strings.Repeat("é", 120)
	tests := []struct {
		name    string
		action  string
		changes map[string]any
		txn     *txnInfo
		want    string
	}{
		{"create from changes", "MCP_CREATE_TRANSACTION",
			map[string]any{"description": "Blue Bottle", "amount": float64(1871), "currency": "MYR"}, nil,
			"Added transaction · Blue Bottle · MYR 18.71"},
		{"create without description", "MCP_CREATE_TRANSACTION",
			map[string]any{"description": "", "amount": float64(500), "currency": "USD"}, nil,
			"Added transaction · USD 5.00"},
		{"older row borrows the transaction", "MCP_CREATE_TRANSACTION",
			map[string]any{"amount": float64(1871)}, &txnInfo{Description: "Latte", Currency: "MYR"},
			"Added transaction · Latte · MYR 18.71"},
		{"older row, transaction gone: action alone", "MCP_CREATE_TRANSACTION",
			map[string]any{}, nil, "Added a transaction"},
		{"update uses the transaction", "MCP_UPDATE_TRANSACTION",
			map[string]any{"description": map[string]any{"from": "a", "to": "b"}}, &txnInfo{Description: "Now"},
			"Edited transaction · Now"},
		{"update, transaction gone", "MCP_UPDATE_TRANSACTION", map[string]any{}, nil, "Edited a transaction"},
		{"transfer", "MCP_CREATE_TRANSFER",
			map[string]any{"description": "Rent", "amount": float64(100000), "currency": "MYR"}, nil,
			"Added transfer · Rent · MYR 1000.00"},
		{"budget name", "MCP_CREATE_BUDGET", map[string]any{"name": "Food"}, nil, "Created budget · Food"},
		{"category fallback", "MCP_UPDATE_CATEGORY", map[string]any{}, nil, "Edited a category"},
		{"repayment", "MCP_RECORD_REPAYMENT", map[string]any{"amount": float64(250), "currency": "EUR"}, nil,
			"Recorded repayment · EUR 2.50"},
		{"write-off without amount", "MCP_WRITE_OFF_RECEIVABLE", map[string]any{}, nil, "Wrote off a receivable"},
		{"unknown action", "MCP_SOMETHING_NEW", map[string]any{"description": "x"}, nil, "Made a change"},
		{"truncates user text to 80 characters", "MCP_CREATE_BUDGET", map[string]any{"name": long}, nil,
			"Created budget · " + strings.Repeat("é", 80) + "…"},
		{"strips control characters, newlines and bidi overrides", "MCP_CREATE_CATEGORY",
			map[string]any{"name": "Fo\x00o\n\n  d\u202eevil\x1b[31m"}, nil,
			"Created category · Foo devil[31m"},
		{"markup is left as plain text", "MCP_CREATE_BUDGET", map[string]any{"name": "<img src=x onerror=1>"}, nil,
			"Created budget · <img src=x onerror=1>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildActivitySummary(tt.action, tt.changes, tt.txn); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppActivity_SummaryFromTransaction(t *testing.T) {
	env := newActivityEnv(t)
	acct := testutil.CreateTestCashAccount(t, env.db, env.userA.ID)
	tx := testutil.CreateTestTransaction(t, env.db, env.userA.ID, acct.ID, models.TransactionTypeExpense, 1871)
	testutil.AssertNoError(t, env.db.Model(tx).UpdateColumn("description", "Blue Bottle").Error)

	// An older-style row: no description or currency in changes.
	env.mcpLog(env.userA.ID, "claude", "MCP_CREATE_TRANSACTION", "transaction", tx.ID,
		map[string]any{"type": "expense", "amount": 1871, "account_id": acct.ID})

	page, err := env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if got := page.Data[0].Summary; got != "Added transaction · Blue Bottle · USD 18.71" {
		t.Errorf("got %q", got)
	}
	if page.Data[0].ResourceID != tx.ID || page.Data[0].ResourceType != "transaction" {
		t.Errorf("unexpected resource: %+v", page.Data[0])
	}

	// The transaction is deleted (soft): its description is still found.
	testutil.AssertNoError(t, env.db.Delete(tx).Error)
	page, err = env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if got := page.Data[0].Summary; got != "Added transaction · Blue Bottle · USD 18.71" {
		t.Errorf("a deleted transaction should still resolve, got %q", got)
	}

	// Gone for good: fall back to the action and the changes' own amount only.
	testutil.AssertNoError(t, env.db.Unscoped().Delete(&models.Transaction{}, "id = ?", tx.ID).Error)
	page, err = env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if got := page.Data[0].Summary; got != "Added a transaction" {
		t.Errorf("expected the fallback, got %q", got)
	}
}

func TestAppActivity_TransactionJoinIsUserScoped(t *testing.T) {
	env := newActivityEnv(t)
	acctB := testutil.CreateTestCashAccount(t, env.db, env.userB.ID)
	txB := testutil.CreateTestTransaction(t, env.db, env.userB.ID, acctB.ID, models.TransactionTypeExpense, 100)
	testutil.AssertNoError(t, env.db.Model(txB).UpdateColumn("description", "B private").Error)

	// A's audit row pointing at B's transaction id must not borrow B's text.
	env.mcpLog(env.userA.ID, "claude", "MCP_UPDATE_TRANSACTION", "transaction", txB.ID, map[string]any{})
	page, err := env.svc.ListByClient(env.userA.ID, "claude", pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if got := page.Data[0].Summary; got != "Edited a transaction" {
		t.Errorf("got %q", got)
	}
}

func TestSourceClientName(t *testing.T) {
	env := newActivityEnv(t)
	acctSvc := NewAccountService(env.db)
	txSvc := NewTransactionService(env.db, acctSvc, nil)
	acct := testutil.CreateTestCashAccount(t, env.db, env.userA.ID)

	made, err := txSvc.CreateTransaction(env.userA.ID, acct.ID, nil, models.TransactionTypeExpense, 500, "Via app", time.Now(),
		TransactionMeta{Source: models.TransactionSourceMCP, SourceClientID: "claude"})
	testutil.AssertNoError(t, err)
	web, err := txSvc.CreateTransaction(env.userA.ID, acct.ID, nil, models.TransactionTypeExpense, 500, "Via web", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)

	list := func() map[string]models.Transaction {
		t.Helper()
		page, err := txSvc.GetUserTransactions(env.userA.ID, pagination.PageRequest{}, TransactionFilter{})
		testutil.AssertNoError(t, err)
		out := map[string]models.Transaction{}
		for _, tx := range page.Data {
			out[tx.ID] = tx
		}
		return out
	}

	got := list()
	if got[made.ID].SourceClientName != "Claude" {
		t.Errorf("list: expected the grant's name, got %q", got[made.ID].SourceClientName)
	}
	if got[web.ID].SourceClientName != "" {
		t.Errorf("a web transaction has no app name, got %q", got[web.ID].SourceClientName)
	}
	byID, err := txSvc.GetTransactionByID(env.userA.ID, made.ID)
	testutil.AssertNoError(t, err)
	if byID.SourceClientName != "Claude" {
		t.Errorf("detail: got %q", byID.SourceClientName)
	}
	acctTxns, err := txSvc.GetAccountTransactions(env.userA.ID, acct.ID, pagination.PageRequest{}, TransactionFilter{})
	testutil.AssertNoError(t, err)
	for _, tx := range acctTxns.Data {
		if tx.ID == made.ID && tx.SourceClientName != "Claude" {
			t.Errorf("account list: got %q", tx.SourceClientName)
		}
	}

	// Another user's grant with the same client id does not name A's rows.
	testutil.AssertNoError(t, env.db.Model(&models.OAuthClientGrant{}).
		Where("user_id = ?", env.userB.ID).UpdateColumn("client_name", "B's name").Error)
	if list()[made.ID].SourceClientName != "Claude" {
		t.Error("the name must come from the owner's own grant")
	}

	// The grant is deleted: fall back to "an app".
	testutil.AssertNoError(t, env.db.Where("user_id = ?", env.userA.ID).Delete(&models.OAuthClientGrant{}).Error)
	if got := list()[made.ID].SourceClientName; got != "an app" {
		t.Errorf("expected the fallback after the grant is deleted, got %q", got)
	}
	byID, err = txSvc.GetTransactionByID(env.userA.ID, made.ID)
	testutil.AssertNoError(t, err)
	if byID.SourceClientName != "an app" {
		t.Errorf("detail fallback: got %q", byID.SourceClientName)
	}
}
