package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// CategoryHandler handles category-related requests.
type CategoryHandler struct {
	categoryService services.CategoryServicer
	auditService    services.AuditServicer
}

// NewCategoryHandler creates a new CategoryHandler.
func NewCategoryHandler(categoryService services.CategoryServicer, auditService services.AuditServicer) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService, auditService: auditService}
}

// CreateCategoryRequest represents the request payload for creating a category
type CreateCategoryRequest struct {
	Name        string              `json:"name" binding:"required,min=1,max=100"`
	Type        models.CategoryType `json:"type" binding:"required,category_type"`
	Description string              `json:"description" binding:"max=500"`
	Icon        string              `json:"icon" binding:"max=50"`
	Color       string              `json:"color" binding:"omitempty,hex_color"`
	ParentID    *string             `json:"parent_id"`
}

// UpdateCategoryRequest represents the request payload for updating a category
type UpdateCategoryRequest struct {
	Name        string  `json:"name" binding:"omitempty,min=1,max=100"`
	Description string  `json:"description" binding:"max=500"`
	Icon        string  `json:"icon" binding:"max=50"`
	Color       string  `json:"color" binding:"omitempty,hex_color"`
	ParentID    *string `json:"parent_id"`
}

// CategoryResponse represents a category in the response
type CategoryResponse struct {
	ID          uint                `json:"id"`
	UserID      uint                `json:"user_id"`
	Name        string              `json:"name"`
	Type        models.CategoryType `json:"type"`
	Description string              `json:"description"`
	Icon        string              `json:"icon"`
	Color       string              `json:"color"`
	ParentID    *string             `json:"parent_id,omitempty"`
}

// CreateCategory handles the creation of a new category
// @Summary     Create a category
// @Description Create a new transaction category
// @Tags        categories
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateCategoryRequest true "Category details"
// @Success     201 {object} CategoryResponse "Category created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /categories [post]
func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	category, err := h.categoryService.CreateCategory(
		userID,
		req.Name,
		req.Type,
		req.Description,
		req.Icon,
		req.Color,
		req.ParentID,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_CATEGORY", "category", category.ID, c.ClientIP(),
		map[string]interface{}{"name": req.Name, "type": req.Type})

	c.JSON(http.StatusCreated, gin.H{"category": category})
}

// GetUserCategories handles the retrieval of categories for a user
// @Summary     Get categories
// @Description Get a paginated list of transaction categories for the authenticated user
// @Tags        categories
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       type      query string false "Filter by category type (income/expense)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Category] "Paginated categories"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /categories [get]
func (h *CategoryHandler) GetUserCategories(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	categoryType := c.Query("type")
	if categoryType != "" && categoryType != "income" && categoryType != "expense" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid category type: must be 'income' or 'expense'"))
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	var result *pagination.PageResponse[models.Category]

	if categoryType != "" {
		result, err = h.categoryService.GetUserCategoriesByType(userID, models.CategoryType(categoryType), page)
	} else {
		result, err = h.categoryService.GetUserCategories(userID, page)
	}

	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetCategoryByID handles the retrieval of a specific category
// @Summary     Get category by ID
// @Description Get a specific transaction category by ID
// @Tags        categories
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Category ID"
// @Success     200 {object} CategoryResponse "Category details"
// @Failure     400 {object} ErrorResponse "Invalid category ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /categories/{id} [get]
func (h *CategoryHandler) GetCategoryByID(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	categoryID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	category, err := h.categoryService.GetCategoryByID(userID, categoryID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"category": category})
}

// UpdateCategory handles updating a category
// @Summary     Update category
// @Description Update an existing transaction category
// @Tags        categories
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Category ID"
// @Param       request body UpdateCategoryRequest true "Updated category details"
// @Success     200 {object} CategoryResponse "Updated category"
// @Failure     400 {object} ErrorResponse "Invalid input or category ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /categories/{id} [put]
func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	categoryID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	category, err := h.categoryService.UpdateCategory(
		userID,
		categoryID,
		req.Name,
		req.Description,
		req.Icon,
		req.Color,
		req.ParentID,
	)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_CATEGORY", "category", categoryID, c.ClientIP(),
		map[string]interface{}{"name": req.Name})

	c.JSON(http.StatusOK, gin.H{"category": category})
}

// DeleteCategory handles deleting a category
// @Summary     Delete category
// @Description Delete a transaction category by ID
// @Tags        categories
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path int true "Category ID"
// @Success     200 {object} MessageResponse "Category deleted"
// @Failure     400 {object} ErrorResponse "Invalid category ID or cannot delete category in use"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /categories/{id} [delete]
func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	categoryID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.categoryService.DeleteCategory(userID, categoryID); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_CATEGORY", "category", categoryID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}
