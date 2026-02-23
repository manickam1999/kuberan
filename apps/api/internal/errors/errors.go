// Package errors provides custom error types for the Kuberan API.
// All service-layer errors should use AppError to ensure consistent,
// secure error responses that never leak internal details to clients.
package errors

import "net/http"

// AppError represents a structured application error with an error code,
// human-readable message, HTTP status code, and optional internal error.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	StatusCode int    `json:"-"`
	Internal   error  `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string { return e.Message }

// Unwrap returns the internal error for use with errors.Is/As.
func (e *AppError) Unwrap() error { return e.Internal }

// Wrap creates a new AppError with the same code/message/status but wraps an internal error.
func Wrap(sentinel *AppError, internal error) *AppError {
	return &AppError{
		Code:       sentinel.Code,
		Message:    sentinel.Message,
		StatusCode: sentinel.StatusCode,
		Internal:   internal,
	}
}

// WithMessage creates a new AppError with a custom message.
func WithMessage(sentinel *AppError, message string) *AppError {
	return &AppError{
		Code:       sentinel.Code,
		Message:    message,
		StatusCode: sentinel.StatusCode,
		Internal:   sentinel.Internal,
	}
}

// Authentication & authorization errors.
var (
	ErrUnauthorized       = &AppError{Code: "UNAUTHORIZED", Message: "Authentication required", StatusCode: http.StatusUnauthorized}
	ErrInvalidCredentials = &AppError{Code: "INVALID_CREDENTIALS", Message: "Invalid email or password", StatusCode: http.StatusUnauthorized}
	ErrForbidden          = &AppError{Code: "FORBIDDEN", Message: "Access denied", StatusCode: http.StatusForbidden}
	ErrAccountLocked      = &AppError{Code: "ACCOUNT_LOCKED", Message: "Account is temporarily locked", StatusCode: http.StatusLocked}
)

// General errors.
var (
	ErrInvalidInput   = &AppError{Code: "INVALID_INPUT", Message: "Invalid input", StatusCode: http.StatusBadRequest}
	ErrNotFound       = &AppError{Code: "NOT_FOUND", Message: "Resource not found", StatusCode: http.StatusNotFound}
	ErrInternalServer = &AppError{Code: "INTERNAL_ERROR", Message: "An internal error occurred", StatusCode: http.StatusInternalServerError}
)

// User errors.
var (
	ErrUserNotFound   = &AppError{Code: "USER_NOT_FOUND", Message: "User not found", StatusCode: http.StatusNotFound}
	ErrDuplicateEmail = &AppError{Code: "DUPLICATE_EMAIL", Message: "A user with this email already exists", StatusCode: http.StatusConflict}
)

// Account errors.
var (
	ErrAccountNotFound = &AppError{Code: "ACCOUNT_NOT_FOUND", Message: "Account not found", StatusCode: http.StatusNotFound}
)

// Category errors.
var (
	ErrCategoryNotFound    = &AppError{Code: "CATEGORY_NOT_FOUND", Message: "Category not found", StatusCode: http.StatusNotFound}
	ErrCategoryInUse       = &AppError{Code: "CATEGORY_IN_USE", Message: "Category is used by existing transactions", StatusCode: http.StatusConflict}
	ErrCategoryHasChildren = &AppError{Code: "CATEGORY_HAS_CHILDREN", Message: "Category has child categories", StatusCode: http.StatusConflict}
	ErrSelfParentCategory  = &AppError{Code: "SELF_PARENT_CATEGORY", Message: "A category cannot be its own parent", StatusCode: http.StatusBadRequest}
)

// Transaction errors.
var (
	ErrTransactionNotFound    = &AppError{Code: "TRANSACTION_NOT_FOUND", Message: "Transaction not found", StatusCode: http.StatusNotFound}
	ErrInvalidTransactionType = &AppError{Code: "INVALID_TRANSACTION_TYPE", Message: "Unsupported transaction type", StatusCode: http.StatusBadRequest}
	ErrInsufficientBalance    = &AppError{Code: "INSUFFICIENT_BALANCE", Message: "Insufficient account balance", StatusCode: http.StatusBadRequest}
	ErrSameAccountTransfer    = &AppError{Code: "SAME_ACCOUNT_TRANSFER", Message: "Cannot transfer to the same account", StatusCode: http.StatusBadRequest}
	ErrTransactionNotEditable = &AppError{Code: "TRANSACTION_NOT_EDITABLE", Message: "This transaction type cannot be edited", StatusCode: http.StatusBadRequest}
	ErrInvalidTypeChange      = &AppError{Code: "INVALID_TYPE_CHANGE", Message: "Cannot change transaction type to or from transfer/investment", StatusCode: http.StatusBadRequest}
)

// Budget errors.
var (
	ErrBudgetNotFound = &AppError{Code: "BUDGET_NOT_FOUND", Message: "Budget not found", StatusCode: http.StatusNotFound}
)

// Investment errors.
var (
	ErrInvestmentNotFound = &AppError{Code: "INVESTMENT_NOT_FOUND", Message: "Investment not found", StatusCode: http.StatusNotFound}
	ErrInsufficientShares = &AppError{Code: "INSUFFICIENT_SHARES", Message: "Insufficient shares for this sale", StatusCode: http.StatusBadRequest}
)

// Security errors.
var (
	ErrSecurityNotFound  = &AppError{Code: "SECURITY_NOT_FOUND", Message: "Security not found", StatusCode: http.StatusNotFound}
	ErrDuplicateSecurity = &AppError{Code: "DUPLICATE_SECURITY", Message: "A security with this symbol and exchange already exists", StatusCode: http.StatusConflict}
)

// Telegram errors.
var (
	ErrInvalidLinkCode       = &AppError{Code: "INVALID_LINK_CODE", Message: "Invalid or expired link code", StatusCode: http.StatusBadRequest}
	ErrLinkCodeExpired       = &AppError{Code: "LINK_CODE_EXPIRED", Message: "Link code has expired", StatusCode: http.StatusBadRequest}
	ErrTelegramAlreadyLinked = &AppError{Code: "TELEGRAM_ALREADY_LINKED", Message: "This Telegram account is already linked to another user", StatusCode: http.StatusConflict}
)
