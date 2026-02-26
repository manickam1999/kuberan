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

// --- mock category service ---

type mockCategoryService struct {
	createCategoryFn          func(userID string, name string, categoryType models.CategoryType, description, icon, color string, parentID *string) (*models.Category, error)
	getUserCategoriesFn       func(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error)
	getUserCategoriesByTypeFn func(userID string, categoryType models.CategoryType, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error)
	getCategoryByIDFn         func(userID, categoryID string) (*models.Category, error)
	updateCategoryFn          func(userID, categoryID string, name, description, icon, color string, parentID *string) (*models.Category, error)
	deleteCategoryFn          func(userID, categoryID string) error
}

func (m *mockCategoryService) CreateCategory(userID string, name string, categoryType models.CategoryType, description, icon, color string, parentID *string) (*models.Category, error) {
	if m.createCategoryFn != nil {
		return m.createCategoryFn(userID, name, categoryType, description, icon, color, parentID)
	}
	return &models.Category{}, nil
}

func (m *mockCategoryService) GetUserCategories(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
	if m.getUserCategoriesFn != nil {
		return m.getUserCategoriesFn(userID, page)
	}
	resp := pagination.NewPageResponse([]models.Category{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockCategoryService) GetUserCategoriesByType(userID string, categoryType models.CategoryType, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
	if m.getUserCategoriesByTypeFn != nil {
		return m.getUserCategoriesByTypeFn(userID, categoryType, page)
	}
	resp := pagination.NewPageResponse([]models.Category{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockCategoryService) GetCategoryByID(userID, categoryID string) (*models.Category, error) {
	if m.getCategoryByIDFn != nil {
		return m.getCategoryByIDFn(userID, categoryID)
	}
	return &models.Category{}, nil
}

func (m *mockCategoryService) UpdateCategory(userID, categoryID string, name, description, icon, color string, parentID *string) (*models.Category, error) {
	if m.updateCategoryFn != nil {
		return m.updateCategoryFn(userID, categoryID, name, description, icon, color, parentID)
	}
	return &models.Category{}, nil
}

func (m *mockCategoryService) DeleteCategory(userID, categoryID string) error {
	if m.deleteCategoryFn != nil {
		return m.deleteCategoryFn(userID, categoryID)
	}
	return nil
}

var _ services.CategoryServicer = (*mockCategoryService)(nil)

func setupCategoryRouter(handler *CategoryHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.POST("/categories", handler.CreateCategory)
	auth.GET("/categories", handler.GetUserCategories)
	auth.GET("/categories/:id", handler.GetCategoryByID)
	auth.PUT("/categories/:id", handler.UpdateCategory)
	auth.DELETE("/categories/:id", handler.DeleteCategory)
	return r
}

func TestCategoryHandler_CreateCategory(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		catSvc := &mockCategoryService{
			createCategoryFn: func(_ uint, name string, catType models.CategoryType, desc, icon, color string, _ *uint) (*models.Category, error) {
				return &models.Category{
					Base: models.Base{ID: 1},
					Name: name,
					Type: catType,
					Icon: icon,
				}, nil
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "POST", "/categories",
			`{"name":"Food","type":"expense","icon":"🍕","color":"#FF0000"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		cat := result["category"].(map[string]interface{})
		if cat["name"] != "Food" {
			t.Errorf("expected Food, got %v", cat["name"])
		}
	})

	t.Run("returns 400 on missing name", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "POST", "/categories", `{"type":"expense"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on missing type", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "POST", "/categories", `{"name":"Food"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid type", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "POST", "/categories", `{"name":"Food","type":"invalid"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid color format", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "POST", "/categories", `{"name":"Food","type":"expense","color":"red"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/categories", handler.CreateCategory)

		rec := doRequest(r, "POST", "/categories", `{"name":"Food","type":"expense"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestCategoryHandler_GetUserCategories(t *testing.T) {
	t.Run("returns 200 with all categories", func(t *testing.T) {
		catSvc := &mockCategoryService{
			getUserCategoriesFn: func(_ uint, _ pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
				resp := pagination.NewPageResponse([]models.Category{
					{Base: models.Base{ID: 1}, Name: "Food", Type: "expense"},
					{Base: models.Base{ID: 2}, Name: "Salary", Type: "income"},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "GET", "/categories", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 categories, got %d", len(data))
		}
	})

	t.Run("filters by type", func(t *testing.T) {
		var capturedType models.CategoryType
		catSvc := &mockCategoryService{
			getUserCategoriesByTypeFn: func(_ uint, catType models.CategoryType, _ pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
				capturedType = catType
				resp := pagination.NewPageResponse([]models.Category{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		doRequest(r, "GET", "/categories?type=income", "")

		if capturedType != "income" {
			t.Errorf("expected income, got %s", capturedType)
		}
	})

	t.Run("returns 400 on invalid type filter", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "GET", "/categories?type=invalid", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})
}

func TestCategoryHandler_GetCategoryByID(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		catSvc := &mockCategoryService{
			getCategoryByIDFn: func(_, catID uint) (*models.Category, error) {
				return &models.Category{Base: models.Base{ID: catID}, Name: "Food"}, nil
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "GET", "/categories/1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		catSvc := &mockCategoryService{
			getCategoryByIDFn: func(_, _ uint) (*models.Category, error) {
				return nil, apperrors.ErrCategoryNotFound
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "GET", "/categories/999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestCategoryHandler_UpdateCategory(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		catSvc := &mockCategoryService{
			updateCategoryFn: func(_, catID uint, name, _, _, _ string, _ *uint) (*models.Category, error) {
				return &models.Category{Base: models.Base{ID: catID}, Name: name}, nil
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "PUT", "/categories/1", `{"name":"Updated Food"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		cat := result["category"].(map[string]interface{})
		if cat["name"] != "Updated Food" {
			t.Errorf("expected Updated Food, got %v", cat["name"])
		}
	})

	t.Run("returns 400 on self-parent", func(t *testing.T) {
		catSvc := &mockCategoryService{
			updateCategoryFn: func(_, _ uint, _, _, _, _ string, _ *uint) (*models.Category, error) {
				return nil, apperrors.ErrSelfParentCategory
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		parentID := uint(1)
		_ = parentID
		rec := doRequest(r, "PUT", "/categories/1", `{"parent_id":1}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "SELF_PARENT_CATEGORY")
	})
}

func TestCategoryHandler_DeleteCategory(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		handler := NewCategoryHandler(&mockCategoryService{}, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "DELETE", "/categories/1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		if result["message"] != "Category deleted successfully" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns 409 when has children", func(t *testing.T) {
		catSvc := &mockCategoryService{
			deleteCategoryFn: func(_, _ uint) error {
				return apperrors.ErrCategoryHasChildren
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "DELETE", "/categories/1", "")

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "CATEGORY_HAS_CHILDREN")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		catSvc := &mockCategoryService{
			deleteCategoryFn: func(_, _ uint) error {
				return apperrors.ErrCategoryNotFound
			},
		}
		handler := NewCategoryHandler(catSvc, &mockAuditService{})
		r := setupCategoryRouter(handler)

		rec := doRequest(r, "DELETE", "/categories/999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
