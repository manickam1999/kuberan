package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestBudgetFlow_CreateAndCheckProgress(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "budget@test.com", "password123")

	// Step 1: Create an expense category
	rec := app.request("POST", "/api/v1/categories",
		`{"name":"Groceries","type":"expense"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating category, got %d: %s", rec.Code, rec.Body.String())
	}
	catResult := parseJSON(t, rec)
	category := catResult["category"].(map[string]interface{})
	categoryID := category["id"].(string)

	// Step 2: Create a cash account with $500
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Checking","initial_balance":50000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating account, got %d: %s", rec.Code, rec.Body.String())
	}
	acctResult := parseJSON(t, rec)
	account := acctResult["account"].(map[string]interface{})
	accountID := account["id"].(string)

	// Step 3: Create a monthly budget of $200 for the category
	now := time.Now()
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Grocery Budget","amount":20000,"period":"monthly"}`,
			categoryID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating budget, got %d: %s", rec.Code, rec.Body.String())
	}
	budgetResult := parseJSON(t, rec)
	budget := budgetResult["budget"].(map[string]interface{})
	budgetID := budget["id"].(string)

	// Step 4: Check progress before any spending (should be 0 spent)
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s/progress", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	progressResult := parseJSON(t, rec)
	progress := progressResult["progress"].(map[string]interface{})
	if progress["spent"].(float64) != 0 {
		t.Errorf("expected 0 spent before transactions, got %.0f", progress["spent"].(float64))
	}
	if progress["remaining"].(float64) != 20000 {
		t.Errorf("expected 20000 remaining, got %.0f", progress["remaining"].(float64))
	}
	if progress["percentage"].(float64) != 0 {
		t.Errorf("expected 0%% spent, got %.2f%%", progress["percentage"].(float64))
	}

	// Step 5: Add expense transactions in the current month for this category
	// Expense 1: $80
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":8000,"category_id":%q,"description":"Weekly groceries","date":%q}`,
			accountID, categoryID, now.Format(time.RFC3339)), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Expense 2: $50
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":5000,"category_id":%q,"description":"More groceries","date":%q}`,
			accountID, categoryID, now.Format(time.RFC3339)), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 6: Check progress (should be $130 spent out of $200)
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s/progress", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	progressResult = parseJSON(t, rec)
	progress = progressResult["progress"].(map[string]interface{})
	if progress["spent"].(float64) != 13000 {
		t.Errorf("expected 13000 spent (8000+5000), got %.0f", progress["spent"].(float64))
	}
	if progress["remaining"].(float64) != 7000 {
		t.Errorf("expected 7000 remaining (20000-13000), got %.0f", progress["remaining"].(float64))
	}
	if progress["percentage"].(float64) != 65 {
		t.Errorf("expected 65%% spent, got %.2f%%", progress["percentage"].(float64))
	}
}

func TestBudgetFlow_OverBudget(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "overbudget@test.com", "password123")

	// Create category, account, budget
	rec := app.request("POST", "/api/v1/categories",
		`{"name":"Dining","type":"expense"}`, token)
	catID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Wallet","initial_balance":100000}`, token)
	acctID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	now := time.Now()
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Dining Budget","amount":5000,"period":"monthly"}`,
			catID), token)
	budgetID := parseJSON(t, rec)["budget"].(map[string]interface{})["id"].(string)

	// Spend $75 on a $50 budget (over budget)
	app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":7500,"category_id":%q,"date":%q}`,
			acctID, catID, now.Format(time.RFC3339)), token)

	// Check progress: over budget
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s/progress", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	progress := parseJSON(t, rec)["progress"].(map[string]interface{})
	if progress["spent"].(float64) != 7500 {
		t.Errorf("expected 7500 spent, got %.0f", progress["spent"].(float64))
	}
	if progress["remaining"].(float64) != -2500 {
		t.Errorf("expected -2500 remaining, got %.0f", progress["remaining"].(float64))
	}
	if progress["percentage"].(float64) != 150 {
		t.Errorf("expected 150%%, got %.2f%%", progress["percentage"].(float64))
	}
}

func TestBudgetFlow_CRUDOperations(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "budgetcrud@test.com", "password123")

	// Create category
	rec := app.request("POST", "/api/v1/categories",
		`{"name":"Utilities","type":"expense"}`, token)
	catID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	// Create budget
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Utility Budget","amount":15000,"period":"monthly"}`,
			catID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	budgetID := parseJSON(t, rec)["budget"].(map[string]interface{})["id"].(string)

	// Get budget
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	budget := parseJSON(t, rec)["budget"].(map[string]interface{})
	if budget["name"] != "Utility Budget" {
		t.Errorf("expected name 'Utility Budget', got %v", budget["name"])
	}

	// Update budget name and amount
	rec = app.request("PUT", fmt.Sprintf("/api/v1/budgets/%s", budgetID),
		`{"name":"Updated Utilities","amount":20000}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated := parseJSON(t, rec)["budget"].(map[string]interface{})
	if updated["name"] != "Updated Utilities" {
		t.Errorf("expected name 'Updated Utilities', got %v", updated["name"])
	}
	if updated["amount"].(float64) != 20000 {
		t.Errorf("expected amount 20000, got %.0f", updated["amount"].(float64))
	}

	// List budgets
	rec = app.request("GET", "/api/v1/budgets", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	listResult := parseJSON(t, rec)
	if listResult["total_items"].(float64) != 1 {
		t.Errorf("expected 1 budget in list, got %.0f", listResult["total_items"].(float64))
	}

	// Delete budget
	rec = app.request("DELETE", fmt.Sprintf("/api/v1/budgets/%s", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted (should 404)
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s", budgetID), "", token)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", rec.Code)
	}
}

func TestBudgetFlow_IncomeIgnored(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "budgetincome@test.com", "password123")

	// Create category, account, budget
	rec := app.request("POST", "/api/v1/categories",
		`{"name":"Side Income","type":"expense"}`, token)
	catID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Cash","initial_balance":50000}`, token)
	acctID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	now := time.Now()
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Income Budget","amount":10000,"period":"monthly"}`,
			catID), token)
	budgetID := parseJSON(t, rec)["budget"].(map[string]interface{})["id"].(string)

	// Add income transaction with same category
	app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"income","amount":5000,"category_id":%q,"date":%q}`,
			acctID, catID, now.Format(time.RFC3339)), token)

	// Check progress: income should not count as spending
	rec = app.request("GET", fmt.Sprintf("/api/v1/budgets/%s/progress", budgetID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	progress := parseJSON(t, rec)["progress"].(map[string]interface{})
	if progress["spent"].(float64) != 0 {
		t.Errorf("expected 0 spent (income should be ignored), got %.0f", progress["spent"].(float64))
	}
}

func TestBudgetFlow_DuplicateActiveRejected(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "budgetdup@test.com", "password123")

	rec := app.request("POST", "/api/v1/categories",
		`{"name":"Groceries","type":"expense"}`, token)
	catID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Groceries","amount":20000,"period":"monthly"}`, catID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating first budget, got %d: %s", rec.Code, rec.Body.String())
	}

	// D4: a second active monthly budget for the same category is a 409.
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Groceries Again","amount":25000,"period":"monthly"}`, catID), token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate active budget, got %d: %s", rec.Code, rec.Body.String())
	}
	if code := parseJSON(t, rec)["error"].(map[string]interface{})["code"]; code != "BUDGET_ALREADY_EXISTS" {
		t.Errorf("expected BUDGET_ALREADY_EXISTS, got %v", code)
	}

	// A different period for the same category is allowed.
	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Groceries Yearly","amount":240000,"period":"yearly"}`, catID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for yearly budget, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBudgetFlow_BatchProgressExcludesInactive(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "budgetbatch@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Checking","initial_balance":100000}`, token)
	acctID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Two expense categories, each with an active monthly budget.
	rec = app.request("POST", "/api/v1/categories", `{"name":"Food","type":"expense"}`, token)
	foodCat := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/categories", `{"name":"Transport","type":"expense"}`, token)
	transportCat := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Food","amount":20000,"period":"monthly"}`, foodCat), token)
	foodBudget := parseJSON(t, rec)["budget"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/budgets",
		fmt.Sprintf(`{"category_id":%q,"name":"Transport","amount":10000,"period":"monthly"}`, transportCat), token)
	transportBudget := parseJSON(t, rec)["budget"].(map[string]interface{})["id"].(string)

	now := time.Now()
	app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":5000,"category_id":%q,"date":%q}`,
			acctID, foodCat, now.Format(time.RFC3339)), token)

	// Batch progress: both active budgets appear.
	rec = app.request("GET", "/api/v1/budgets/progress", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	progress := parseJSON(t, rec)["progress"].([]interface{})
	if len(progress) != 2 {
		t.Fatalf("expected 2 active budgets in batch, got %d", len(progress))
	}
	// Verify the food budget shows the $50 spend.
	foundFood := false
	for _, p := range progress {
		entry := p.(map[string]interface{})
		if entry["budget_id"] == foodBudget && entry["spent"].(float64) != 5000 {
			t.Errorf("expected food spent 5000, got %.0f", entry["spent"].(float64))
		}
		if entry["budget_id"] == foodBudget {
			foundFood = true
		}
	}
	if !foundFood {
		t.Error("food budget missing from batch progress")
	}

	// Pause the transport budget; it should drop out of the batch.
	rec = app.request("PUT", fmt.Sprintf("/api/v1/budgets/%s", transportBudget), `{"is_active":false}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 pausing budget, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = app.request("GET", "/api/v1/budgets/progress", "", token)
	progress = parseJSON(t, rec)["progress"].([]interface{})
	if len(progress) != 1 {
		t.Fatalf("expected 1 active budget after pause, got %d", len(progress))
	}
	if progress[0].(map[string]interface{})["budget_id"] != foodBudget {
		t.Errorf("expected remaining budget to be food, got %v", progress[0].(map[string]interface{})["budget_id"])
	}
}
