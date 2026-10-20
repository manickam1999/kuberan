package models

import (
	"strings"
	"testing"
)

func TestNormalizeCounterpartyName(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantName string
		wantKey  string
		wantErr  bool
	}{
		{"simple", "Alex", "Alex", "alex", false},
		{"trims_and_collapses_whitespace", "  Alex \t  Smith\n", "Alex Smith", "alex smith", false},
		{"keeps_display_case_lowers_key", "ACME Corp", "ACME Corp", "acme corp", false},
		{"unicode_runes", "Zoë Ünal", "Zoë Ünal", "zoë ünal", false},
		{"empty", "", "", "", true},
		{"whitespace_only", " \t\n ", "", "", true},
		{"exactly_100_runes", strings.Repeat("é", 100), strings.Repeat("é", 100), strings.Repeat("é", 100), false},
		{"101_runes", strings.Repeat("é", 101), "", "", true},
		{"length_checked_after_normalization", strings.Repeat("a   ", 30), strings.TrimSpace(strings.Repeat("a ", 30)), strings.TrimSpace(strings.Repeat("a ", 30)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, key, err := NormalizeCounterpartyName(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if name != tt.wantName || key != tt.wantKey {
				t.Errorf("got (%q, %q), want (%q, %q)", name, key, tt.wantName, tt.wantKey)
			}
		})
	}
}

func TestIsValidReceivableKind(t *testing.T) {
	for _, k := range []ReceivableKind{"split", "on_behalf", "claim", "loan", "deposit", "refund"} {
		if !IsValidReceivableKind(k) {
			t.Errorf("expected %q to be valid", k)
		}
	}
	for _, k := range []ReceivableKind{"", "Split", "gift", "reimbursement"} {
		if IsValidReceivableKind(k) {
			t.Errorf("expected %q to be invalid", k)
		}
	}
}

func TestReceivableDeriveStatus(t *testing.T) {
	tests := []struct {
		name                     string
		amount, paid, writtenOff int64
		want                     ReceivableStatus
	}{
		{"pending", 1000, 0, 0, ReceivableStatusPending},
		{"partially_paid", 1000, 400, 0, ReceivableStatusPartiallyPaid},
		{"partially_written_off", 1000, 0, 300, ReceivableStatusPartiallyPaid},
		{"paid_and_written_off_partial", 1000, 400, 300, ReceivableStatusPartiallyPaid},
		{"settled", 1000, 1000, 0, ReceivableStatusSettled},
		{"written_off_fully", 1000, 0, 1000, ReceivableStatusWrittenOff},
		{"paid_then_written_off_remainder", 1000, 850, 150, ReceivableStatusWrittenOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Receivable{Amount: tt.amount, PaidAmount: tt.paid, WrittenOffAmount: tt.writtenOff}
			if got := r.DeriveStatus(); got != tt.want {
				t.Errorf("DeriveStatus() = %q, want %q", got, tt.want)
			}
			if got, want := r.ComputeOutstanding(), tt.amount-tt.paid-tt.writtenOff; got != want || r.Outstanding != want {
				t.Errorf("ComputeOutstanding() = %d (field %d), want %d", got, r.Outstanding, want)
			}
		})
	}
}
