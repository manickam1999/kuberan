package services

import (
	"errors"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// categoryService handles category-related business logic.
type categoryService struct {
	db *gorm.DB
}

// NewCategoryService creates a new CategoryServicer.
func NewCategoryService(db *gorm.DB) CategoryServicer {
	return &categoryService{db: db}
}

// CreateCategory creates a new category
func (s *categoryService) CreateCategory(
	userID string,
	name string,
	categoryType models.CategoryType,
	description string,
	icon string,
	color string,
	parentID *string,
) (*models.Category, error) {
	// Validate input
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "category name is required")
	}

	// Check if a category with the same name already exists for this user
	var count int64
	if err := s.db.Model(&models.Category{}).
		Where("user_id = ? AND name = ?", userID, name).
		Count(&count).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if count > 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "category with this name already exists")
	}

	// If parentID is provided, check that it exists and belongs to the user
	if parentID != nil {
		var parent models.Category
		if err := s.db.Where("id = ? AND user_id = ?", *parentID, userID).First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.WithMessage(apperrors.ErrCategoryNotFound, "parent category not found")
			}
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	// Create category
	category := &models.Category{
		UserID:      userID,
		Name:        name,
		Type:        categoryType,
		Description: description,
		Icon:        icon,
		Color:       color,
		ParentID:    parentID,
	}

	if err := s.db.Create(category).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return category, nil
}

// GetUserCategories retrieves a paginated list of categories for a user.
func (s *categoryService) GetUserCategories(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Category{}).Where("user_id = ?", userID)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var categories []models.Category
	if err := base.Scopes(pagination.Paginate(page)).Find(&categories).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(categories, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetUserCategoriesByType retrieves a paginated list of categories of a specific type for a user.
func (s *categoryService) GetUserCategoriesByType(userID string, categoryType models.CategoryType, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Category{}).Where("user_id = ? AND type = ?", userID, categoryType)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var categories []models.Category
	if err := base.Scopes(pagination.Paginate(page)).Find(&categories).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(categories, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetCategoryByID retrieves a category by ID for a specific user
func (s *categoryService) GetCategoryByID(userID, categoryID string) (*models.Category, error) {
	var category models.Category
	if err := s.db.Where("id = ? AND user_id = ?", categoryID, userID).First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrCategoryNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &category, nil
}

// UpdateCategory updates an existing category
func (s *categoryService) UpdateCategory(
	userID string,
	categoryID string,
	name string,
	description string,
	icon string,
	color string,
	parentID *string,
) (*models.Category, error) {
	// Get the category
	category, err := s.GetCategoryByID(userID, categoryID)
	if err != nil {
		return nil, err
	}

	// If parentID is provided, check that it exists, belongs to the user, and is not the category itself
	if parentID != nil && *parentID != "" {
		if *parentID == categoryID {
			return nil, apperrors.ErrSelfParentCategory
		}

		var parent models.Category
		if err := s.db.Where("id = ? AND user_id = ?", *parentID, userID).First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.WithMessage(apperrors.ErrCategoryNotFound, "parent category not found")
			}
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	// Update fields if provided
	updates := make(map[string]interface{})
	if name != "" {
		updates["name"] = name
	}
	if description != "" {
		updates["description"] = description
	}
	if icon != "" {
		updates["icon"] = icon
	}
	if color != "" {
		updates["color"] = color
	}
	if parentID != nil {
		updates["parent_id"] = parentID
	}

	// Apply updates if any
	if len(updates) > 0 {
		if err := s.db.Model(category).Updates(updates).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	return category, nil
}

// DeleteCategory deletes a category
func (s *categoryService) DeleteCategory(userID, categoryID string) error {
	// Get the category to ensure it exists and belongs to the user
	category, err := s.GetCategoryByID(userID, categoryID)
	if err != nil {
		return err
	}

	// Check if there are any child categories
	var childCount int64
	if err := s.db.Model(&models.Category{}).Where("parent_id = ?", categoryID).Count(&childCount).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if childCount > 0 {
		return apperrors.ErrCategoryHasChildren
	}

	// Soft-delete the category and deactivate any auto-categorization rule that
	// targets it (plan 018). The matcher already skips soft-deleted targets, but
	// deactivating makes the dead rule visible in the UI rather than silently
	// inert. Existing transactions keep their category_id reference for history.
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(category).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		ruleIDs := tx.Model(&models.TransactionRuleAction{}).
			Select("rule_id").
			Where("category_id = ?", categoryID)
		if err := tx.Model(&models.TransactionRule{}).
			Where("user_id = ? AND id IN (?)", userID, ruleIDs).
			Update("is_active", false).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return nil
	})
}
