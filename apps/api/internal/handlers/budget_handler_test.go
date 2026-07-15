package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// --- mock budget service ---

type mockBudgetService struct {
	createBudgetFn             func(userID, categoryID string, name string, amount int64, period models.BudgetPeriod) (*models.Budget, error)
	getUserBudgetsFn           func(userID string, page pagination.PageRequest, isActive *bool, period *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error)
	getBudgetByIDFn            func(userID, budgetID string) (*models.Budget, error)
	updateBudgetFn             func(userID, budgetID string, name string, amount *int64, period *models.BudgetPeriod, isActive *bool) (*models.Budget, error)
	deleteBudgetFn             func(userID, budgetID string) error
	getBudgetProgressFn        func(userID, budgetID string) (*services.BudgetProgress, error)
	getActiveBudgetsProgressFn func(userID string) ([]services.BudgetProgress, error)
}

func (m *mockBudgetService) CreateBudget(userID, categoryID, name string, amount int64, period models.BudgetPeriod) (*models.Budget, error) {
	if m.createBudgetFn != nil {
		return m.createBudgetFn(userID, categoryID, name, amount, period)
	}
	return &models.Budget{}, nil
}

func (m *mockBudgetService) GetUserBudgets(userID string, page pagination.PageRequest, isActive *bool, period *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error) {
	if m.getUserBudgetsFn != nil {
		return m.getUserBudgetsFn(userID, page, isActive, period)
	}
	resp := pagination.NewPageResponse([]models.Budget{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockBudgetService) GetBudgetByID(userID, budgetID string) (*models.Budget, error) {
	if m.getBudgetByIDFn != nil {
		return m.getBudgetByIDFn(userID, budgetID)
	}
	return &models.Budget{}, nil
}

func (m *mockBudgetService) UpdateBudget(userID, budgetID, name string, amount *int64, period *models.BudgetPeriod, isActive *bool) (*models.Budget, error) {
	if m.updateBudgetFn != nil {
		return m.updateBudgetFn(userID, budgetID, name, amount, period, isActive)
	}
	return &models.Budget{}, nil
}

func (m *mockBudgetService) DeleteBudget(userID, budgetID string) error {
	if m.deleteBudgetFn != nil {
		return m.deleteBudgetFn(userID, budgetID)
	}
	return nil
}

func (m *mockBudgetService) GetBudgetProgress(userID, budgetID string) (*services.BudgetProgress, error) {
	if m.getBudgetProgressFn != nil {
		return m.getBudgetProgressFn(userID, budgetID)
	}
	return &services.BudgetProgress{}, nil
}

func (m *mockBudgetService) GetActiveBudgetsProgress(userID string) ([]services.BudgetProgress, error) {
	if m.getActiveBudgetsProgressFn != nil {
		return m.getActiveBudgetsProgressFn(userID)
	}
	return []services.BudgetProgress{}, nil
}

var _ services.BudgetServicer = (*mockBudgetService)(nil)

func setupBudgetRouter(handler *BudgetHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.POST("/budgets", handler.CreateBudget)
	auth.GET("/budgets", handler.GetBudgets)
	auth.GET("/budgets/progress", handler.GetBudgetsProgress)
	auth.GET("/budgets/:id", handler.GetBudget)
	auth.PUT("/budgets/:id", handler.UpdateBudget)
	auth.DELETE("/budgets/:id", handler.DeleteBudget)
	auth.GET("/budgets/:id/progress", handler.GetBudgetProgress)
	return r
}

func TestBudgetHandler_CreateBudget(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		svc := &mockBudgetService{
			createBudgetFn: func(_ string, categoryID string, name string, amount int64, period models.BudgetPeriod) (*models.Budget, error) {
				return &models.Budget{
					Base:       models.Base{ID: "1"},
					UserID:     "1",
					CategoryID: categoryID,
					Name:       name,
					Amount:     amount,
					Period:     period,
					IsActive:   true,
				}, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":50000,"period":"monthly"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		budget := result["budget"].(map[string]interface{})
		if budget["name"] != "Groceries" {
			t.Errorf("expected Groceries, got %v", budget["name"])
		}
		if budget["amount"].(float64) != 50000 {
			t.Errorf("expected amount 50000, got %v", budget["amount"])
		}
	})

	t.Run("returns 400 on missing name", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","amount":50000,"period":"monthly"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns 400 on missing period", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":50000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid period", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":50000,"period":"weekly"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on zero amount", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":0,"period":"monthly"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 404 on invalid category", func(t *testing.T) {
		svc := &mockBudgetService{
			createBudgetFn: func(_, _ string, _ string, _ int64, _ models.BudgetPeriod) (*models.Budget, error) {
				return nil, apperrors.ErrCategoryNotFound
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000999","name":"Groceries","amount":50000,"period":"monthly"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "CATEGORY_NOT_FOUND")
	})

	t.Run("returns 409 on duplicate active budget", func(t *testing.T) {
		svc := &mockBudgetService{
			createBudgetFn: func(_, _ string, _ string, _ int64, _ models.BudgetPeriod) (*models.Budget, error) {
				return nil, apperrors.ErrBudgetAlreadyExists
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":50000,"period":"monthly"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "BUDGET_ALREADY_EXISTS")
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/budgets", handler.CreateBudget)

		rec := doRequest(r, "POST", "/budgets",
			`{"category_id":"00000000-0000-0000-0000-000000000001","name":"Groceries","amount":50000,"period":"monthly"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestBudgetHandler_GetBudgets(t *testing.T) {
	t.Run("returns 200 with paginated budgets", func(t *testing.T) {
		svc := &mockBudgetService{
			getUserBudgetsFn: func(_ string, _ pagination.PageRequest, _ *bool, _ *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error) {
				resp := pagination.NewPageResponse([]models.Budget{
					{Base: models.Base{ID: "1"}, Name: "Groceries"},
					{Base: models.Base{ID: "2"}, Name: "Entertainment"},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 budgets, got %d", len(data))
		}
		if result["total_items"].(float64) != 2 {
			t.Errorf("expected total_items=2, got %v", result["total_items"])
		}
	})

	t.Run("passes filter params to service", func(t *testing.T) {
		var capturedIsActive *bool
		var capturedPeriod *models.BudgetPeriod
		svc := &mockBudgetService{
			getUserBudgetsFn: func(_ string, _ pagination.PageRequest, isActive *bool, period *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error) {
				capturedIsActive = isActive
				capturedPeriod = period
				resp := pagination.NewPageResponse([]models.Budget{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		doRequest(r, "GET", "/budgets?is_active=true&period=monthly", "")

		if capturedIsActive == nil || !*capturedIsActive {
			t.Error("expected is_active=true to be passed")
		}
		if capturedPeriod == nil || *capturedPeriod != models.BudgetPeriodMonthly {
			t.Error("expected period=monthly to be passed")
		}
	})

	t.Run("returns 400 on invalid is_active", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets?is_active=maybe", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns 400 on invalid period", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets?period=weekly", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})
}

func TestBudgetHandler_GetBudget(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		svc := &mockBudgetService{
			getBudgetByIDFn: func(_, budgetID string) (*models.Budget, error) {
				return &models.Budget{
					Base:   models.Base{ID: budgetID},
					Name:   "Groceries",
					Amount: 50000,
				}, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/00000000-0000-0000-0000-000000000001", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		budget := result["budget"].(map[string]interface{})
		if budget["name"] != "Groceries" {
			t.Errorf("expected Groceries, got %v", budget["name"])
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := &mockBudgetService{
			getBudgetByIDFn: func(_, _ string) (*models.Budget, error) {
				return nil, apperrors.ErrBudgetNotFound
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/00000000-0000-0000-0000-000000000999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "BUDGET_NOT_FOUND")
	})

	t.Run("returns 400 on invalid ID", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestBudgetHandler_UpdateBudget(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		svc := &mockBudgetService{
			updateBudgetFn: func(_, budgetID string, name string, amount *int64, _ *models.BudgetPeriod, _ *bool) (*models.Budget, error) {
				b := &models.Budget{
					Base: models.Base{ID: budgetID},
					Name: name,
				}
				if amount != nil {
					b.Amount = *amount
				}
				return b, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "PUT", "/budgets/00000000-0000-0000-0000-000000000001", `{"name":"Updated Budget","amount":75000}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		budget := result["budget"].(map[string]interface{})
		if budget["name"] != "Updated Budget" {
			t.Errorf("expected Updated Budget, got %v", budget["name"])
		}
	})

	t.Run("passes is_active to service", func(t *testing.T) {
		var captured *bool
		svc := &mockBudgetService{
			updateBudgetFn: func(_, budgetID string, _ string, _ *int64, _ *models.BudgetPeriod, isActive *bool) (*models.Budget, error) {
				captured = isActive
				return &models.Budget{Base: models.Base{ID: budgetID}}, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "PUT", "/budgets/00000000-0000-0000-0000-000000000001", `{"is_active":false}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured == nil {
			t.Fatal("expected is_active to be passed to the service")
		}
		if *captured {
			t.Error("expected is_active=false to be passed")
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := &mockBudgetService{
			updateBudgetFn: func(_, _ string, _ string, _ *int64, _ *models.BudgetPeriod, _ *bool) (*models.Budget, error) {
				return nil, apperrors.ErrBudgetNotFound
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "PUT", "/budgets/00000000-0000-0000-0000-000000000999", `{"name":"Updated"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "BUDGET_NOT_FOUND")
	})
}

func TestBudgetHandler_DeleteBudget(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "DELETE", "/budgets/00000000-0000-0000-0000-000000000001", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["message"] != "Budget deleted successfully" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := &mockBudgetService{
			deleteBudgetFn: func(_, _ string) error {
				return apperrors.ErrBudgetNotFound
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "DELETE", "/budgets/00000000-0000-0000-0000-000000000999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "BUDGET_NOT_FOUND")
	})

	t.Run("returns 400 on invalid ID", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "DELETE", "/budgets/abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestBudgetHandler_GetBudgetProgress(t *testing.T) {
	t.Run("returns 200 with progress", func(t *testing.T) {
		svc := &mockBudgetService{
			getBudgetProgressFn: func(_, budgetID string) (*services.BudgetProgress, error) {
				return &services.BudgetProgress{
					BudgetID:   budgetID,
					Budgeted:   50000,
					Spent:      25000,
					Remaining:  25000,
					Percentage: 50.0,
				}, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/00000000-0000-0000-0000-000000000001/progress", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		progress := result["progress"].(map[string]interface{})
		if progress["budgeted"].(float64) != 50000 {
			t.Errorf("expected budgeted=50000, got %v", progress["budgeted"])
		}
		if progress["spent"].(float64) != 25000 {
			t.Errorf("expected spent=25000, got %v", progress["spent"])
		}
		if progress["percentage"].(float64) != 50.0 {
			t.Errorf("expected percentage=50, got %v", progress["percentage"])
		}
	})

	t.Run("returns 404 when budget not found", func(t *testing.T) {
		svc := &mockBudgetService{
			getBudgetProgressFn: func(_, _ string) (*services.BudgetProgress, error) {
				return nil, apperrors.ErrBudgetNotFound
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/00000000-0000-0000-0000-000000000999/progress", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "BUDGET_NOT_FOUND")
	})

	t.Run("returns 400 on invalid ID", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/abc/progress", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestBudgetHandler_GetBudgetsProgress(t *testing.T) {
	t.Run("returns 200 with progress list", func(t *testing.T) {
		svc := &mockBudgetService{
			getActiveBudgetsProgressFn: func(_ string) ([]services.BudgetProgress, error) {
				return []services.BudgetProgress{
					{BudgetID: "1", Budgeted: 50000, Spent: 25000, Remaining: 25000, Percentage: 50.0},
					{BudgetID: "2", Budgeted: 10000, Spent: 12000, Remaining: -2000, Percentage: 120.0},
				}, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/progress", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		progress := result["progress"].([]interface{})
		if len(progress) != 2 {
			t.Fatalf("expected 2 progress entries, got %d", len(progress))
		}
		first := progress[0].(map[string]interface{})
		if first["budget_id"] != "1" {
			t.Errorf("expected first budget_id=1, got %v", first["budget_id"])
		}
	})

	t.Run("routes static path before param path", func(t *testing.T) {
		// /budgets/progress must hit the batch handler, not GetBudget with id="progress".
		called := false
		svc := &mockBudgetService{
			getActiveBudgetsProgressFn: func(_ string) ([]services.BudgetProgress, error) {
				called = true
				return []services.BudgetProgress{}, nil
			},
			getBudgetByIDFn: func(_, _ string) (*models.Budget, error) {
				t.Fatal("GetBudgetByID should not be called for /budgets/progress")
				return nil, nil
			},
		}
		handler := NewBudgetHandler(svc, &mockAuditService{})
		r := setupBudgetRouter(handler)

		rec := doRequest(r, "GET", "/budgets/progress", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !called {
			t.Error("expected GetActiveBudgetsProgress to be called")
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewBudgetHandler(&mockBudgetService{}, &mockAuditService{})
		r := gin.New()
		r.GET("/budgets/progress", handler.GetBudgetsProgress)

		rec := doRequest(r, "GET", "/budgets/progress", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
