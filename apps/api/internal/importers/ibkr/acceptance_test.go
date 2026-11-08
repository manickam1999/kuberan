package ibkr

import (
	"math"
	"os"
	"testing"
)

func loadFixture(t *testing.T, name string) *Statement {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()
	st, err := Parse(f)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return st
}

func fixtureSet(t *testing.T) *EventSet {
	t.Helper()
	// Deliberately passed out of order: BuildEvents must sort by period.
	set, err := BuildEvents([]*Statement{loadFixture(t, "statement_2026.csv"), loadFixture(t, "statement_2025.csv")})
	if err != nil {
		t.Fatalf("BuildEvents: %v", err)
	}
	return set
}

// replayQuantities replays buys, sells and splits up to and including date.
func replayQuantities(events []Event, date string) map[string]float64 {
	q := map[string]float64{}
	for i := range events {
		e := &events[i]
		if e.Date > date {
			continue
		}
		switch e.Kind {
		case KindBuy:
			q[e.Symbol] += e.Quantity
		case KindSell:
			q[e.Symbol] -= e.Quantity
		case KindSplit:
			q[e.Symbol] *= e.SplitRatio
		case KindDividend, KindExpense, KindDeposit, KindWithdrawal:
		}
		q[e.Symbol] = math.Round(q[e.Symbol]*1e10) / 1e10
		if math.Abs(q[e.Symbol]) < 1e-9 {
			q[e.Symbol] = 0
		}
	}
	return q
}

func TestFixturesAcceptanceQuantities(t *testing.T) {
	set := fixtureSet(t)
	if set.Account != "U00000000" || set.BaseCurrency != "USD" || len(set.Checkpoints) != 2 {
		t.Fatalf("unexpected set metadata: %q %q %d", set.Account, set.BaseCurrency, len(set.Checkpoints))
	}
	want := map[string]map[string]float64{
		"2025-12-31": {"CCJ": 30.0511, "CHA": 102.9936, "IBKR": 2.3791, "NVO": 71.2295, "VUAA": 28, "VWRA": 15, "RBOT": 0, "SOUN": 0},
		"2026-11-02": {"CCJ": 30.0511, "CHA": 0, "IBKR": 1.6706, "NVDA": 10.2137, "NVO": 30.2548, "SPCX": 10, "VUAA": 45.2, "VWRA": 37},
	}
	for _, cp := range set.Checkpoints {
		got := replayQuantities(set.Events, cp.Date)
		for sym, w := range want[cp.Date] {
			if math.Abs(got[sym]-w) > 1e-4 {
				t.Errorf("%s %s: replay %.10f, want %v", cp.Date, sym, got[sym], w)
			}
			t.Logf("%s %-5s replay=%.6f ibkr=%.6f", cp.Date, sym, got[sym], cp.Positions[sym].Quantity)
		}
		// Every replayed symbol must match the parsed Open Positions checkpoint (absent = flat).
		for _, sym := range set.Symbols() {
			if math.Abs(got[sym]-cp.Positions[sym].Quantity) > 1e-4 {
				t.Errorf("%s %s: replay %.10f, parsed checkpoint %v", cp.Date, sym, got[sym], cp.Positions[sym].Quantity)
			}
		}
	}
}

func TestFixturesAcceptanceCash(t *testing.T) {
	set := fixtureSet(t)
	cp25, cp26 := set.Checkpoints[0], set.Checkpoints[1]
	if !cp25.HasEndingCash || cp25.EndingCashCents != 465 || !cp26.HasEndingCash || cp26.EndingCashCents != 12055 {
		t.Fatalf("ending cash checkpoints: %+v %+v", cp25, cp26)
	}
	ledger := cp25.EndingCashCents
	rows := 0
	for _, e := range set.Events {
		if e.Date < "2026-01-01" || e.CashDelta() == 0 {
			continue
		}
		ledger += e.CashDelta()
		rows++
	}
	diff := ledger - cp26.EndingCashCents
	t.Logf("2026 cash rows=%d ledger=%d ibkr=%d diff=%d", rows, ledger, cp26.EndingCashCents, diff)
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(rows) {
		t.Errorf("ledger %d vs ibkr %d: |diff| %d exceeds %d rows", ledger, cp26.EndingCashCents, diff, rows)
	}
	// Measured value: the ledger lands exactly on IBKR's Ending Cash (120.55).
	const wantRows, wantLedger = 26, int64(12055)
	if rows != wantRows || ledger != wantLedger {
		t.Errorf("rows=%d ledger=%d, want rows=%d ledger=%d", rows, ledger, wantRows, wantLedger)
	}
}

func TestFixturesAcceptanceCash2025(t *testing.T) {
	set := fixtureSet(t)
	ledger := int64(0)
	rows := 0
	for _, e := range set.Events {
		if e.Date > "2025-12-31" || e.CashDelta() == 0 {
			continue
		}
		ledger += e.CashDelta()
		rows++
	}
	diff := ledger - set.Checkpoints[0].EndingCashCents
	t.Logf("2025 cash rows=%d ledger=%d ibkr=%d diff=%d", rows, ledger, set.Checkpoints[0].EndingCashCents, diff)
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(rows) {
		t.Errorf("2025 ledger %d vs ibkr %d exceeds %d rows", ledger, set.Checkpoints[0].EndingCashCents, rows)
	}
}
