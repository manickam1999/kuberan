package services

import (
	"fmt"
	"unicode/utf8"

	apperrors "kuberan/internal/errors"
)

// Text limits for user-entered fields, enforced by the services so every
// caller (REST handlers, the MCP server, the bot via REST) gets the same rule.
// They count characters, not bytes. The REST request structs repeat them as
// binding:"max=N" tags to reject oversized input before it reaches a service;
// keep the two in sync.
const (
	// MaxDescriptionLen bounds transaction, receivable and category descriptions.
	MaxDescriptionLen = 500
	// MaxNotesLen bounds a transaction's notes (also the result of appending to them).
	MaxNotesLen = 2000
	// MaxSearchLen bounds the free-text transaction search term.
	MaxSearchLen = 100
	// MaxNameLen bounds category and budget names.
	MaxNameLen = 100
	// MaxIconLen bounds a category's icon name.
	MaxIconLen = 50
)

// CheckTextLength returns INVALID_INPUT when v is longer than limit
// characters. Callers that validate before reaching a service (the MCP tools,
// which must reject bad input before charging the rate limit) use it with the
// constants above so they apply exactly the service's rule.
func CheckTextLength(field, v string, limit int) error {
	if utf8.RuneCountInString(v) > limit {
		return apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("%s must be at most %d characters", field, limit))
	}
	return nil
}

// validateCategoryText checks a category's name, description and icon.
func validateCategoryText(name, description, icon string) error {
	if err := CheckTextLength("name", name, MaxNameLen); err != nil {
		return err
	}
	if err := CheckTextLength("description", description, MaxDescriptionLen); err != nil {
		return err
	}
	return CheckTextLength("icon", icon, MaxIconLen)
}
