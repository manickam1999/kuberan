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
		{"nbsp_and_nel_are_whitespace", "Alex\u00a0\u0085Smith", "Alex Smith", "alex smith", false},
		{"nul_rejected", "Al\x00ex", "", "", true},
		{"bell_rejected", "Alex\a", "", "", true},
		{"invalid_utf8_rejected", "Al\xffex", "", "", true},
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

func TestNormalizeCounterpartyKey(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"Alex", "alex"},
		{"  ALEX \t  Smith\n", "alex smith"},
		{"A\u00a0B\u2003C", "a b c"}, // other Unicode spaces collapse like ASCII ones
		{"", ""},
		{" \t\n ", ""},
		{"Zoë  Ünal", "zoë ünal"},
	}
	for _, tt := range tests {
		if got := NormalizeCounterpartyKey(tt.raw); got != tt.want {
			t.Errorf("NormalizeCounterpartyKey(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
	// A name's key is always NormalizeCounterpartyKey of the name.
	for _, raw := range []string{"  Mixed   CASE ", "ACME\tCorp"} {
		_, key, err := NormalizeCounterpartyName(raw)
		if err != nil || key != NormalizeCounterpartyKey(raw) {
			t.Errorf("key(%q) = %q, %v; want %q", raw, key, err, NormalizeCounterpartyKey(raw))
		}
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
		wantOut                  int64
	}{
		{"pending", 1000, 0, 0, ReceivableStatusPending, 1000},
		{"partially_paid", 1000, 400, 0, ReceivableStatusPartiallyPaid, 600},
		{"partially_written_off", 1000, 0, 300, ReceivableStatusPartiallyPaid, 700},
		{"paid_and_written_off_partial", 1000, 400, 300, ReceivableStatusPartiallyPaid, 300},
		{"settled", 1000, 1000, 0, ReceivableStatusSettled, 0},
		{"written_off_fully", 1000, 0, 1000, ReceivableStatusWrittenOff, 0},
		{"paid_then_written_off_remainder", 1000, 850, 150, ReceivableStatusWrittenOff, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Receivable{Amount: tt.amount, PaidAmount: tt.paid, WrittenOffAmount: tt.writtenOff}
			if got := r.DeriveStatus(); got != tt.want {
				t.Errorf("DeriveStatus() = %q, want %q", got, tt.want)
			}
			if got := r.ComputeOutstanding(); got != tt.wantOut || r.Outstanding != tt.wantOut {
				t.Errorf("ComputeOutstanding() = %d (field %d), want %d", got, r.Outstanding, tt.wantOut)
			}
		})
	}
}
