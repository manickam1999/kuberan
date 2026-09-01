package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestAutoCategorizeOnCreate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	transport := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	other := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	_, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(transport.ID)}})
	testutil.AssertNoError(t, err)

	t.Run("matching transaction is auto-categorized", func(t *testing.T) {
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2500, "GRAB *RIDE KL", time.Now())
		testutil.AssertNoError(t, err)
		if tx.CategoryID == nil || *tx.CategoryID != transport.ID {
			t.Errorf("expected auto-category %s, got %v", transport.ID, tx.CategoryID)
		}
	})

	t.Run("explicit category is never overridden", func(t *testing.T) {
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &other.ID, models.TransactionTypeExpense, 2500, "GRAB *RIDE KL", time.Now())
		testutil.AssertNoError(t, err)
		if tx.CategoryID == nil || *tx.CategoryID != other.ID {
			t.Errorf("expected explicit category %s preserved, got %v", other.ID, tx.CategoryID)
		}
	})

	t.Run("non-matching transaction stays uncategorized", func(t *testing.T) {
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2500, "STARBUCKS", time.Now())
		testutil.AssertNoError(t, err)
		if tx.CategoryID != nil {
			t.Errorf("expected no category, got %v", *tx.CategoryID)
		}
	})
}

func TestApplyRule_Backfill(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	// Three matching + one non-matching uncategorized expense transactions,
	// created directly (bypassing rules) to simulate pre-existing history.
	for _, desc := range []string{"GRAB RIDE 1", "GRAB RIDE 2", "GRAB RIDE 3", "STARBUCKS"} {
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
		var last models.Transaction
		db.Where("user_id = ?", user.ID).Order("created_at DESC").First(&last)
		db.Model(&last).Update("description", desc)
	}
	balanceBefore := account.Balance

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)

	t.Run("dry run counts without writing", func(t *testing.T) {
		res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized, DryRun: true})
		testutil.AssertNoError(t, err)
		if res.Count != 3 {
			t.Errorf("expected 3 matches, got %d", res.Count)
		}
		if res.Applied != 0 {
			t.Errorf("expected 0 applied on dry run, got %d", res.Applied)
		}
		var categorized int64
		db.Model(&models.Transaction{}).Where("user_id = ? AND category_id IS NOT NULL", user.ID).Count(&categorized)
		if categorized != 0 {
			t.Errorf("expected nothing written on dry run, got %d categorized", categorized)
		}
	})

	t.Run("commit applies and is balance-neutral", func(t *testing.T) {
		res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized, DryRun: false})
		testutil.AssertNoError(t, err)
		if res.Applied != 3 {
			t.Errorf("expected 3 applied, got %d", res.Applied)
		}
		var categorized int64
		db.Model(&models.Transaction{}).Where("user_id = ? AND category_id = ?", user.ID, cat.ID).Count(&categorized)
		if categorized != 3 {
			t.Errorf("expected 3 transactions categorized, got %d", categorized)
		}

		// Balance must be untouched by a category-only backfill.
		var reloaded models.Account
		db.Where("id = ?", account.ID).First(&reloaded)
		if reloaded.Balance != balanceBefore {
			t.Errorf("expected balance unchanged (%d), got %d", balanceBefore, reloaded.Balance)
		}
	})
}

func TestApplyRule_PausedRuleStillBackfills(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	tx := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
	db.Model(tx).Update("description", "GRAB RIDE")

	// Rule is created paused (inactive).
	inactive := false
	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab", IsActive: &inactive,
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)

	// Explicit backfill must honor the paused rule.
	res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized, DryRun: false})
	testutil.AssertNoError(t, err)
	if res.Applied != 1 {
		t.Errorf("expected paused rule backfill to apply 1, got %d", res.Applied)
	}
}

func TestApplyRule_OverwriteSemantics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	target := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	existing := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	// A matching transaction that already has a (different) category.
	tx := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
	db.Model(tx).Updates(map[string]interface{}{"description": "GRAB RIDE", "category_id": existing.ID})

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(target.ID)}})
	testutil.AssertNoError(t, err)

	t.Run("scope=uncategorized skips already-categorized", func(t *testing.T) {
		res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized, DryRun: false})
		testutil.AssertNoError(t, err)
		if res.Applied != 0 {
			t.Errorf("expected 0 applied (uncategorized scope), got %d", res.Applied)
		}
	})

	t.Run("scope=all without overwrite skips categorized", func(t *testing.T) {
		res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeAll, Overwrite: false, DryRun: false})
		testutil.AssertNoError(t, err)
		if res.Applied != 0 {
			t.Errorf("expected 0 applied (no overwrite), got %d", res.Applied)
		}
	})

	t.Run("scope=all with overwrite replaces category", func(t *testing.T) {
		res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeAll, Overwrite: true, DryRun: false})
		testutil.AssertNoError(t, err)
		if res.Applied != 1 {
			t.Errorf("expected 1 applied (overwrite), got %d", res.Applied)
		}
		var reloaded models.Transaction
		db.Where("id = ?", tx.ID).First(&reloaded)
		if reloaded.CategoryID == nil || *reloaded.CategoryID != target.ID {
			t.Errorf("expected category overwritten to %s, got %v", target.ID, reloaded.CategoryID)
		}
	})
}

func TestPreviewRuleMatches(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)

	for _, desc := range []string{"GRAB RIDE 1", "GRAB RIDE 2", "STARBUCKS"} {
		tx := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
		db.Model(tx).Update("description", desc)
	}

	preview, err := txSvc.PreviewRuleMatches(user.ID, []RuleConditionInput{descContains("grab")})
	testutil.AssertNoError(t, err)
	if preview.Count != 2 {
		t.Errorf("expected 2 preview matches, got %d", preview.Count)
	}
	if len(preview.Sample) != 2 {
		t.Errorf("expected 2 sample rows, got %d", len(preview.Sample))
	}
}
