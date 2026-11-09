package services

import (
	"errors"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// ensureCategoryMatchesType returns CATEGORY_NOT_FOUND unless categoryID (when
// set) is a live category owned by userID, and CATEGORY_TYPE_MISMATCH unless
// its type (income/expense) equals txType.
func ensureCategoryMatchesType(db *gorm.DB, userID string, categoryID *string, txType models.TransactionType) error {
	if categoryID == nil {
		return nil
	}
	var category models.Category
	if err := db.Where("id = ? AND user_id = ?", *categoryID, userID).First(&category).Error; err != nil {
		return apperrors.ErrCategoryNotFound
	}
	if string(category.Type) != string(txType) {
		return apperrors.ErrCategoryTypeMismatch
	}
	return nil
}

// ensureCategoryOwned returns ErrCategoryNotFound unless categoryID (when set)
// is a live category owned by userID. Transactions reference categories by ID,
// so without this check a caller could attach, and then read back the name of,
// another user's category.
func ensureCategoryOwned(db *gorm.DB, userID string, categoryID *string) error {
	if categoryID == nil {
		return nil
	}
	var n int64
	if err := db.Model(&models.Category{}).
		Where("id = ? AND user_id = ?", *categoryID, userID).Count(&n).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if n == 0 {
		return apperrors.ErrCategoryNotFound
	}
	return nil
}

// ensureCategoryMatchesType rejects attaching a category whose type (income or
// expense) differs from the transaction's type, which would corrupt spending
// reports. A nil categoryID (clearing the category) always passes.
func ensureCategoryMatchesType(db *gorm.DB, userID string, categoryID *string, txType models.TransactionType) error {
	if categoryID == nil {
		return nil
	}
	var cat models.Category
	if err := db.Select("id", "type").
		Where("id = ? AND user_id = ?", *categoryID, userID).First(&cat).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrCategoryNotFound
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if !categoryTypeMatchesTxType(cat.Type, txType) {
		return apperrors.ErrCategoryTypeMismatch
	}
	return nil
}
