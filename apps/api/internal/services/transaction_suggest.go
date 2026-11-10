package services

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// Reasons a category was suggested.
const (
	// SuggestReasonRule means one of the user's transaction rules matched.
	SuggestReasonRule = "rule"
	// SuggestReasonHistory means the user's most recent categorisation of the
	// same description was reused.
	SuggestReasonHistory = "history"
)

// CategorySuggestionInput describes the transaction to suggest a category for.
// Amount and AccountID are optional; they only matter to rules that condition
// on them.
type CategorySuggestionInput struct {
	Description string
	Type        models.TransactionType
	Amount      int64
	AccountID   string
}

// CategorySuggestion is a suggested category and why it was chosen.
type CategorySuggestion struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	// Reason is SuggestReasonRule or SuggestReasonHistory.
	Reason string `json:"reason"`
}

// SuggestCategory suggests a category for a not-yet-created income or expense:
// first the user's transaction rules (the same resolution CreateTransaction
// applies), then the most recent category the user gave a transaction of the
// same type with the same normalised description. It returns nil when nothing
// applies. Everything is scoped to userID.
func (s *transactionService) SuggestCategory(userID string, in CategorySuggestionInput) (*CategorySuggestion, error) {
	if !isCategorizableType(in.Type) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "type must be income or expense")
	}
	description := strings.Join(strings.Fields(in.Description), " ")
	if description == "" {
		return nil, nil
	}

	if s.ruleService != nil {
		res, err := s.ruleService.ResolveForUser(userID, RuleInput{
			Description: description,
			Amount:      in.Amount,
			AccountID:   in.AccountID,
			Type:        in.Type,
		})
		if err != nil {
			return nil, err
		}
		if res.CategoryID != nil {
			cat, err := s.liveCategory(userID, *res.CategoryID)
			if err != nil {
				return nil, err
			}
			if cat != nil {
				return &CategorySuggestion{CategoryID: cat.ID, CategoryName: cat.Name, Reason: SuggestReasonRule}, nil
			}
		}
	}

	var catID string
	err := s.db.Model(&models.Transaction{}).
		Select("transactions.category_id").
		Joins("JOIN categories ON categories.id = transactions.category_id AND categories.user_id = transactions.user_id AND categories.deleted_at IS NULL").
		Where("transactions.user_id = ? AND transactions.type = ? AND transactions.category_id IS NOT NULL", userID, in.Type).
		Where("LOWER(TRIM(transactions.description)) = ?", strings.ToLower(description)).
		Order("transactions.date DESC, transactions.created_at DESC").
		Limit(1).
		Scan(&catID).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if catID == "" {
		return nil, nil
	}
	cat, err := s.liveCategory(userID, catID)
	if err != nil || cat == nil {
		return nil, err
	}
	return &CategorySuggestion{CategoryID: cat.ID, CategoryName: cat.Name, Reason: SuggestReasonHistory}, nil
}

// liveCategory returns the user's non-deleted category, or nil if it is gone.
func (s *transactionService) liveCategory(userID, categoryID string) (*models.Category, error) {
	var cat models.Category
	err := s.db.Where("id = ? AND user_id = ?", categoryID, userID).First(&cat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &cat, nil
}
