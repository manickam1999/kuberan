package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

func TestAppErrorIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"same sentinel", ErrNotFound, ErrNotFound, true},
		{"Wrap keeps the code", Wrap(ErrNotFound, fmt.Errorf("db")), ErrNotFound, true},
		{"WithMessage keeps the code", WithMessage(ErrCategoryNotFound, "parent category not found"), ErrCategoryNotFound, true},
		{"wrapped by fmt.Errorf", fmt.Errorf("context: %w", WithMessage(ErrNotFound, "x")), ErrNotFound, true},
		{"different code", ErrNotFound, ErrCategoryNotFound, false},
		{"not an AppError", stderrors.New("boom"), ErrNotFound, false},
		{"internal error still reachable", Wrap(ErrInternalServer, errSentinel), errSentinel, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stderrors.Is(tc.err, tc.target); got != tc.want {
				t.Errorf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}

var errSentinel = stderrors.New("sentinel")

func TestWithDetails(t *testing.T) {
	d := map[string]any{"currency": "MYR", "suggested_amount": int64(447120)}
	e := WithDetails(ErrFXAmountRequired, d)
	if e == ErrFXAmountRequired {
		t.Fatal("WithDetails must return a copy")
	}
	if ErrFXAmountRequired.Details != nil {
		t.Fatal("sentinel was mutated")
	}
	d["currency"] = "EUR" // the copy must not alias the caller's map
	if e.Details["currency"] != "MYR" {
		t.Errorf("details aliased the input map: %v", e.Details)
	}
	if !stderrors.Is(e, ErrFXAmountRequired) {
		t.Error("copy must still match the sentinel by code")
	}
	if got := WithMessage(e, "other").Details["currency"]; got != "MYR" {
		t.Errorf("WithMessage dropped details: %v", got)
	}
	if ErrFXAmountRequired.Body()["details"] != nil {
		t.Error("sentinel body must not carry details")
	}
	if e.Body()["details"] == nil {
		t.Error("body must carry details")
	}
}
