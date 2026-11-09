package integration

// Integration tests for loan accounts (plan 023, 5.3) and reminders (5.6):
// create a loan and a card with due days, watch the reminders appear, let the
// bot pull and confirm them, and check that another user sees none of it.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
)

// botRequest calls an /internal endpoint with the test bot secret.
func (app *testApp) botRequest(method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", "test-bot-secret")
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

func accountOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	return parseJSON(t, rec)["account"].(map[string]interface{})
}

func TestLoanAccountFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "loan@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/debt",
		`{"name":"Car loan","amount_owed":5000000,"interest_rate":3.5,"monthly_payment":120000,"due_day":15}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create loan: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	loan := accountOf(t, rec)
	id := loan["id"].(string)
	if loan["type"] != "debt" || loan["balance"].(float64) != 5000000 {
		t.Fatalf("unexpected loan: %v", loan)
	}
	if loan["monthly_payment"].(float64) != 120000 || loan["due_day"].(float64) != 15 {
		t.Fatalf("loan fields missing: %v", loan)
	}
	if loan["currency"] == "" {
		t.Fatalf("currency should default to the base currency: %v", loan)
	}

	// The amount owed is an opening balance, not an expense.
	rec = app.request("GET", "/api/v1/accounts/"+id+"/transactions", "", token)
	txs := parseJSON(t, rec)["data"].([]interface{})
	if len(txs) != 1 || txs[0].(map[string]interface{})["type"] != "opening_balance" {
		t.Fatalf("expected one opening_balance row, got %v", txs)
	}

	// Validation.
	if rec := app.request("POST", "/api/v1/accounts/debt", `{"name":"x","due_day":40}`, token); rec.Code != http.StatusBadRequest {
		t.Fatalf("due_day 40: expected 400, got %d", rec.Code)
	}

	// Edit the loan.
	rec = app.request("PUT", "/api/v1/accounts/"+id, `{"monthly_payment":150000,"due_day":20}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update loan: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	upd := accountOf(t, rec)
	if upd["monthly_payment"].(float64) != 150000 || upd["due_day"].(float64) != 20 {
		t.Fatalf("loan not updated: %v", upd)
	}
}

func TestReminderFlow_BotPullsAndConfirms(t *testing.T) {
	app := setupApp(t)
	tokenA, _, userA := app.registerUser(t, "rem-a@test.com", "password123")
	tokenB, _, userB := app.registerUser(t, "rem-b@test.com", "password123")

	// Both users link Telegram.
	for i, uid := range []string{userA, userB} {
		link := &models.TelegramLink{UserID: uid, TelegramUserID: int64(1000 + i), IsActive: true}
		if err := app.DB.Create(link).Error; err != nil {
			t.Fatalf("link: %v", err)
		}
	}

	rec := app.request("POST", "/api/v1/accounts/credit-card",
		`{"name":"Amex Gold","currency":"MYR","credit_limit":1000000,"current_balance":123450,"due_day":1}`, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create card: %d %s", rec.Code, rec.Body.String())
	}
	cardID := accountOf(t, rec)["id"].(string)

	// The user sees the reminder for the card, not yet due.
	rec = app.request("GET", "/api/v1/reminders?ref_id="+cardID, "", tokenA)
	list := parseJSON(t, rec)["reminders"].([]interface{})
	if len(list) != 1 {
		t.Fatalf("expected 1 reminder, got %v", list)
	}
	rem := list[0].(map[string]interface{})
	remID := rem["id"].(string)
	if rem["kind"] != "card_due" || rem["enabled"] != true {
		t.Fatalf("unexpected reminder: %v", rem)
	}

	// User B sees none and can't touch A's.
	rec = app.request("GET", "/api/v1/reminders", "", tokenB)
	if got := parseJSON(t, rec)["reminders"].([]interface{}); len(got) != 0 {
		t.Fatalf("user B must see no reminders, got %v", got)
	}
	if rec := app.request("PATCH", "/api/v1/reminders/"+remID, `{"enabled":false}`, tokenB); rec.Code != http.StatusNotFound {
		t.Fatalf("user B patch: expected 404, got %d", rec.Code)
	}

	// The internal endpoints need the bot secret.
	if rec := app.request("GET", "/api/v1/internal/telegram/reminders/due", "", tokenA); rec.Code == http.StatusOK {
		t.Fatalf("internal endpoint must reject a user token")
	}

	// Make it sendable now (the reminder is normally days away).
	if err := app.DB.Model(&models.Reminder{}).Where("id = ?", remID).
		Update("remind_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	rec = app.botRequest("GET", "/api/v1/internal/telegram/reminders/due")
	if rec.Code != http.StatusOK {
		t.Fatalf("due: %d %s", rec.Code, rec.Body.String())
	}
	due := parseJSON(t, rec)["reminders"].([]interface{})
	if len(due) != 1 {
		t.Fatalf("expected 1 due reminder, got %v", due)
	}
	d := due[0].(map[string]interface{})
	if d["account_name"] != "Amex Gold" || d["amount"].(float64) != 123450 || d["currency"] != "MYR" || d["telegram_user_id"].(float64) != 1000 {
		t.Fatalf("unexpected due payload: %v", d)
	}

	// Confirm twice: both 200, and it is gone from the due list.
	for i := 0; i < 2; i++ {
		rec = app.botRequest("POST", fmt.Sprintf("/api/v1/internal/telegram/reminders/%s/sent", remID))
		if rec.Code != http.StatusOK {
			t.Fatalf("sent #%d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec = app.botRequest("GET", "/api/v1/internal/telegram/reminders/due")
	if got := parseJSON(t, rec)["reminders"].([]interface{}); len(got) != 0 {
		t.Fatalf("a sent reminder must not come back: %v", got)
	}
	if rec := app.botRequest("POST", "/api/v1/internal/telegram/reminders/00000000-0000-0000-0000-000000000000/sent"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reminder: expected 404, got %d", rec.Code)
	}
}

func TestReminderFlow_Switches(t *testing.T) {
	app := setupApp(t)
	token, _, userID := app.registerUser(t, "rem-switch@test.com", "password123")
	if err := app.DB.Create(&models.TelegramLink{UserID: userID, TelegramUserID: 4242, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}

	rec := app.request("POST", "/api/v1/accounts/debt", `{"name":"Loan","currency":"MYR","amount_owed":10000,"due_day":5}`, token)
	loanID := accountOf(t, rec)["id"].(string)
	list := parseJSON(t, app.request("GET", "/api/v1/reminders?ref_id="+loanID, "", token))["reminders"].([]interface{})
	remID := list[0].(map[string]interface{})["id"].(string)
	makeDue := func() {
		if err := app.DB.Model(&models.Reminder{}).Where("id = ?", remID).Update("remind_at", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
	}
	dueCount := func() int {
		return len(parseJSON(t, app.botRequest("GET", "/api/v1/internal/telegram/reminders/due"))["reminders"].([]interface{}))
	}
	makeDue()
	if dueCount() != 1 {
		t.Fatal("expected the loan reminder to be due")
	}

	// Per-kind switch in Settings -> Telegram.
	rec = app.request("PUT", "/api/v1/telegram/reminder-settings", `{"loan_due":false}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
	link := parseJSON(t, rec)["link"].(map[string]interface{})
	if link["remind_loan_due"] != false || link["remind_card_due"] != true || link["remind_fd_maturity"] != true {
		t.Fatalf("unexpected switches: %v", link)
	}
	if dueCount() != 0 {
		t.Fatal("loan reminders are off; nothing should be due")
	}
	app.request("PUT", "/api/v1/telegram/reminder-settings", `{"loan_due":true}`, token)
	if dueCount() != 1 {
		t.Fatal("loan reminders are back on")
	}

	// Per-account switch.
	rec = app.request("PATCH", "/api/v1/reminders/"+remID, `{"enabled":false}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if dueCount() != 0 {
		t.Fatal("the account's reminder is off; nothing should be due")
	}

	// Clearing the due day removes the reminder.
	app.request("PUT", "/api/v1/accounts/"+loanID, `{"due_day":0}`, token)
	if got := parseJSON(t, app.request("GET", "/api/v1/reminders?ref_id="+loanID, "", token))["reminders"].([]interface{}); len(got) != 0 {
		t.Fatalf("expected the reminder to be gone, got %v", got)
	}

	// Settings need a Telegram link.
	tokenB, _, _ := app.registerUser(t, "rem-nolink@test.com", "password123")
	if rec := app.request("PUT", "/api/v1/telegram/reminder-settings", `{"card_due":false}`, tokenB); rec.Code != http.StatusNotFound {
		t.Fatalf("no link: expected 404, got %d", rec.Code)
	}
}
