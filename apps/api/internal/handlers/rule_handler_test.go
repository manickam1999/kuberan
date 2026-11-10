package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// mockRuleService implements services.RuleServicer; only SuggestRules is scripted.
type mockRuleService struct {
	services.RuleServicer
	suggestFn func(userID string) ([]services.RuleSuggestion, error)
}

func (m *mockRuleService) SuggestRules(userID string) ([]services.RuleSuggestion, error) {
	return m.suggestFn(userID)
}

func setupRuleRouter(handler *RuleHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.GET("/rules/suggestions", handler.GetSuggestions)
	return r
}

func TestRuleHandler_GetSuggestions(t *testing.T) {
	t.Run("returns_200_with_suggestions", func(t *testing.T) {
		var gotUser string
		svc := &mockRuleService{suggestFn: func(userID string) ([]services.RuleSuggestion, error) {
			gotUser = userID
			return []services.RuleSuggestion{{
				DescriptionPattern: "kopi tiam", CategoryID: "cat-1", Type: models.TransactionTypeExpense, Count: 4,
			}}, nil
		}}
		r := setupRuleRouter(NewRuleHandler(svc, nil, &mockAuditService{}))

		rec := doRequest(r, "GET", "/rules/suggestions", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotUser != "test-user-1" {
			t.Errorf("expected the caller's user id, got %q", gotUser)
		}
		list := parseJSON(t, rec)["suggestions"].([]interface{})
		s := list[0].(map[string]interface{})
		if s["description_pattern"] != "kopi tiam" || s["category_id"] != "cat-1" || s["count"].(float64) != 4 {
			t.Errorf("unexpected suggestion %v", s)
		}
	})

	t.Run("returns_500_on_service_error", func(t *testing.T) {
		svc := &mockRuleService{suggestFn: func(string) ([]services.RuleSuggestion, error) {
			return nil, apperrors.ErrInternalServer
		}}
		r := setupRuleRouter(NewRuleHandler(svc, nil, &mockAuditService{}))

		rec := doRequest(r, "GET", "/rules/suggestions", "")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestTransactionChanges(t *testing.T) {
	cat1, cat2 := "cat-1", "cat-2"
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	before := &models.Transaction{AccountID: "a", Type: models.TransactionTypeExpense, Amount: 100,
		Description: "old", Date: day, CategoryID: &cat1, Notes: "n"}

	if got := transactionChanges(before, &models.Transaction{AccountID: "a", Type: models.TransactionTypeExpense,
		Amount: 100, Description: "old", Date: day, CategoryID: &cat1, Notes: "n"}); got != nil {
		t.Errorf("expected nil for no change, got %v", got)
	}

	after := &models.Transaction{AccountID: "a", Type: models.TransactionTypeExpense, Amount: 250,
		Description: "new", Date: day, CategoryID: &cat2, Notes: "changed"}
	got := transactionChanges(before, after)
	amount := got["amount"].(map[string]interface{})
	if amount["from"] != int64(100) || amount["to"] != int64(250) {
		t.Errorf("unexpected amount change %v", amount)
	}
	if _, ok := got["description"]; !ok {
		t.Error("expected description change")
	}
	cat := got["category_id"].(map[string]interface{})
	if *(cat["from"].(*string)) != "cat-1" || *(cat["to"].(*string)) != "cat-2" {
		t.Errorf("unexpected category change %v", cat)
	}
	if notes := got["notes"].(map[string]interface{}); notes["changed"] != true || notes["from"] != nil {
		t.Errorf("notes must record only that they changed, got %v", notes)
	}
	if _, ok := got["date"]; ok {
		t.Error("date did not change")
	}
}
