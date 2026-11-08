package ibkr

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// EventKind is the normalised kind of an Event.
type EventKind string

// Event kinds.
const (
	// KindBuy acquires shares (a trade, or a stock award grant).
	KindBuy EventKind = "buy"
	// KindSell disposes of shares (a trade, a grant return or tax withholding).
	KindSell EventKind = "sell"
	// KindSplit is a stock split with ratio SplitNumerator:SplitDenominator.
	KindSplit EventKind = "split"
	// KindDividend is a dividend; TotalAmount is gross, Fee is tax plus fees.
	KindDividend EventKind = "dividend"
	// KindExpense is a standalone cash expense (e.g. an ADR fee).
	KindExpense EventKind = "expense"
	// KindDeposit is cash arriving in the account.
	KindDeposit EventKind = "deposit"
	// KindWithdrawal is cash leaving the account.
	KindWithdrawal EventKind = "withdrawal"
)

// Event sources, used as the first component of an Event key.
const (
	// SourceTrade is a Trades/Order row.
	SourceTrade = "trade"
	// SourceGrant is a "Stock Award Grant for Cash Deposit" row.
	SourceGrant = "grant"
	// SourceGrantReturn is a "Stock Award Return for Cash Withdrawal" row.
	SourceGrantReturn = "grant_return"
	// SourceGrantWithholding is a "Stock Award Withholding" row.
	SourceGrantWithholding = "grant_withholding"
	// SourceSplit is a Corporate Actions split row.
	SourceSplit = "split"
	// SourceDividend is an aggregated dividend (with tax and fees netted in).
	SourceDividend = "dividend"
	// SourceFee is a standalone Fees row.
	SourceFee = "fee"
	// SourceWithholding is withholding tax with no matching dividend.
	SourceWithholding = "withholding"
	// SourceDeposit is a Deposits & Withdrawals row.
	SourceDeposit = "deposit"
)

// SourceRef points at a CSV row for error messages.
type SourceRef struct {
	// File is the 0-based index of the statement in the slice given to BuildEvents.
	File int
	// Line is the 1-based CSV line number.
	Line int
}

// String renders the reference as "file N line M" (N is 1-based).
func (r SourceRef) String() string {
	return fmt.Sprintf("file %d line %d", r.File+1, r.Line)
}

// Event is one normalised, ordered import event.
type Event struct {
	// Key is unique within an EventSet and deterministic across runs:
	// "<source>:<yyyymmdd[Thhmmss]>:<SYMBOL|cash>:<n>" with the New York local
	// time and n counting identical prefixes from 1. Callers store it as
	// "ibkr:<account>:<Key>".
	Key string
	// Kind is the normalised event kind.
	Kind EventKind
	// Source is the section the event came from (one of the Source constants).
	Source string
	// Symbol is the ticker; empty for expenses-without-symbol and deposits.
	Symbol string
	// Time is the event time in UTC. Date-only events are 00:00 New York time.
	Time time.Time
	// Date is the New York local calendar date, "YYYY-MM-DD".
	Date string
	// DateOnly is true when the source only carried a date.
	DateOnly bool
	// Quantity is the (always positive) share quantity, 0 for cash-only kinds.
	Quantity float64
	// PricePerUnit is cents per share (buys, sells).
	PricePerUnit int64
	// Fee is cents: commission for trades, tax plus fees for dividends.
	Fee int64
	// TotalAmount is cents. Buys: abs(Proceeds)+fee. Sells: Proceeds-fee.
	// Dividends: gross. Expenses, deposits, withdrawals: absolute amount.
	TotalAmount int64
	// SplitNumerator and SplitDenominator hold N and M of a "Split N for M".
	SplitNumerator, SplitDenominator int
	// SplitRatio is N/M for splits (the factor by which quantity multiplies).
	SplitRatio float64
	// Currency is the event currency.
	Currency string
	// Note is a kind-specific note, or IBKR's description for expenses and cash flows.
	Note string
	// NoCash is true when the event has no cash leg (stock award grants,
	// returns and tax withholding).
	NoCash bool
	// Ref is the source row (the first row for aggregated dividends).
	Ref SourceRef
}

// CashDelta returns the signed effect of the event on the cash balance in cents.
func (e Event) CashDelta() int64 {
	if e.NoCash {
		return 0
	}
	switch e.Kind {
	case KindBuy, KindExpense, KindWithdrawal:
		return -e.TotalAmount
	case KindSell, KindDeposit:
		return e.TotalAmount
	case KindDividend:
		return e.TotalAmount - e.Fee
	default:
		return 0
	}
}

// Period is one statement's inclusive date range.
type Period struct {
	// File is the 0-based index of the statement in the BuildEvents input.
	File int
	// Start and End are civil dates (UTC midnight).
	Start, End time.Time
}

// Checkpoint is the broker-reported state at the end of one statement.
type Checkpoint struct {
	// File is the 0-based index of the statement in the BuildEvents input.
	File int
	// Date is the statement end date, "YYYY-MM-DD".
	Date string
	// Positions are the Open Positions summary rows by symbol; symbols not
	// present are flat (quantity 0).
	Positions map[string]Position
	// EndingCashCents is the base-currency Ending Cash in cents.
	EndingCashCents int64
	// HasEndingCash is false when the statement had no Ending Cash row.
	HasEndingCash bool
}

// EventSet is the result of BuildEvents.
type EventSet struct {
	// Account is the IBKR account id shared by all statements.
	Account string
	// BaseCurrency is the account base currency shared by all statements.
	BaseCurrency string
	// Periods are the statement periods in chronological order.
	Periods []Period
	// Events are ordered by time, then section rank, then file and line.
	Events []Event
	// Checkpoints are in chronological order, one per statement.
	Checkpoints []Checkpoint
	// Instruments merges Financial Instrument Information by symbol (later wins).
	Instruments map[string]Instrument
	// IgnoredSections lists the ignored section names in first-seen order.
	IgnoredSections []string
}

// Symbols returns the sorted symbols appearing in any event or checkpoint.
func (s *EventSet) Symbols() []string {
	set := map[string]bool{}
	for i := range s.Events {
		if sym := s.Events[i].Symbol; sym != "" {
			set[sym] = true
		}
	}
	for _, c := range s.Checkpoints {
		for sym := range c.Positions {
			set[sym] = true
		}
	}
	out := make([]string, 0, len(set))
	for sym := range set {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

// EventError is a validation error tied to a source row.
type EventError struct {
	// Ref is the offending row.
	Ref SourceRef
	// Msg describes the problem.
	Msg string
}

// Error implements error.
func (e *EventError) Error() string {
	return fmt.Sprintf("ibkr: %s: %s", e.Ref, e.Msg)
}

type pending struct {
	ev    Event
	rank  int
	order int // position of the statement in period order
}

type builder struct {
	items []pending
	order int
	file  int
}

func (b *builder) errf(line int, format string, args ...any) error {
	return &EventError{Ref: SourceRef{File: b.file, Line: line}, Msg: fmt.Sprintf(format, args...)}
}

func nyMidnight(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, NYLocation).UTC()
}

func (b *builder) add(e Event, rank, line int) {
	e.Ref = SourceRef{File: b.file, Line: line}
	e.Date = e.Time.In(NYLocation).Format("2006-01-02")
	b.items = append(b.items, pending{ev: e, rank: rank, order: b.order})
}

const (
	rankAdjust = iota // grants and splits
	rankTrade
	rankCashIncomeExpense
	rankCashFlow
)

// BuildEvents validates that the statements are for one account and base
// currency and contiguous (sorted by period, the input slice is not modified),
// and returns the ordered, normalised events and per-statement checkpoints.
func BuildEvents(stmts []*Statement) (*EventSet, error) {
	if len(stmts) == 0 {
		return nil, fmt.Errorf("ibkr: no statements")
	}
	idx := make([]int, len(stmts))
	for i, s := range stmts {
		if s == nil {
			return nil, fmt.Errorf("ibkr: statement %d is nil", i+1)
		}
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, c int) bool {
		return stmts[idx[a]].PeriodStart.Before(stmts[idx[c]].PeriodStart)
	})
	first := stmts[idx[0]]
	for k := 1; k < len(idx); k++ {
		prev, cur := stmts[idx[k-1]], stmts[idx[k]]
		if cur.AccountID != first.AccountID {
			return nil, fmt.Errorf("ibkr: file %d is for a different account than file %d", idx[k]+1, idx[0]+1)
		}
		if cur.BaseCurrency != first.BaseCurrency {
			return nil, fmt.Errorf("ibkr: file %d has base currency %s, file %d has %s",
				idx[k]+1, cur.BaseCurrency, idx[0]+1, first.BaseCurrency)
		}
		if !cur.PeriodStart.Equal(prev.PeriodEnd.AddDate(0, 0, 1)) {
			return nil, fmt.Errorf("ibkr: files %d and %d are not contiguous (%s - %s, then %s - %s)",
				idx[k-1]+1, idx[k]+1, prev.PeriodStart.Format("2006-01-02"), prev.PeriodEnd.Format("2006-01-02"),
				cur.PeriodStart.Format("2006-01-02"), cur.PeriodEnd.Format("2006-01-02"))
		}
	}

	set := &EventSet{Account: first.AccountID, BaseCurrency: first.BaseCurrency, Instruments: map[string]Instrument{}}
	b := &builder{}
	seenIgnored := map[string]bool{}
	for order, i := range idx {
		s := stmts[i]
		b.order, b.file = order, i
		if err := b.statement(s); err != nil {
			return nil, err
		}
		set.Periods = append(set.Periods, Period{File: i, Start: s.PeriodStart, End: s.PeriodEnd})
		cp := Checkpoint{File: i, Date: s.PeriodEnd.Format("2006-01-02"), Positions: map[string]Position{},
			EndingCashCents: s.EndingCashCents, HasEndingCash: s.HasEndingCash}
		for sym, p := range s.Positions {
			cp.Positions[sym] = p
		}
		set.Checkpoints = append(set.Checkpoints, cp)
		for sym, in := range s.Instruments {
			set.Instruments[sym] = in
		}
		for _, name := range s.IgnoredSections {
			if !seenIgnored[name] {
				seenIgnored[name] = true
				set.IgnoredSections = append(set.IgnoredSections, name)
			}
		}
	}

	sort.SliceStable(b.items, func(x, y int) bool {
		a, c := b.items[x], b.items[y]
		if !a.ev.Time.Equal(c.ev.Time) {
			return a.ev.Time.Before(c.ev.Time)
		}
		if a.rank != c.rank {
			return a.rank < c.rank
		}
		if a.order != c.order {
			return a.order < c.order
		}
		return a.ev.Ref.Line < c.ev.Ref.Line
	})
	counts := map[string]int{}
	set.Events = make([]Event, len(b.items))
	for i := range b.items {
		e := b.items[i].ev
		stamp := e.Time.In(NYLocation).Format("20060102T150405")
		if e.DateOnly {
			stamp = e.Time.In(NYLocation).Format("20060102")
		}
		who := e.Symbol
		if who == "" {
			who = "cash"
		}
		prefix := fmt.Sprintf("%s:%s:%s", e.Source, stamp, who)
		counts[prefix]++
		e.Key = fmt.Sprintf("%s:%d", prefix, counts[prefix])
		set.Events[i] = e
	}
	return set, nil
}

func (b *builder) statement(s *Statement) error {
	if err := b.trades(s); err != nil {
		return err
	}
	if err := b.grants(s); err != nil {
		return err
	}
	if err := b.splits(s); err != nil {
		return err
	}
	if err := b.dividendsAndFees(s); err != nil {
		return err
	}
	return b.deposits(s)
}

func (b *builder) trades(s *Statement) error {
	for _, t := range s.Trades {
		fee := absRat(t.Fee)
		price, err := cents(t.Price)
		if err != nil {
			return b.errf(t.Line, "price: %v", err)
		}
		feeC, err := cents(fee)
		if err != nil {
			return b.errf(t.Line, "fee: %v", err)
		}
		e := Event{Source: SourceTrade, Symbol: t.Symbol, Time: t.Time, Currency: t.Currency,
			PricePerUnit: price, Fee: feeC, Note: "IBKR trade"}
		var total *big.Rat
		if t.Quantity > 0 {
			e.Kind, e.Quantity = KindBuy, t.Quantity
			total = new(big.Rat).Add(absRat(t.Proceeds), fee)
		} else {
			e.Kind, e.Quantity = KindSell, -t.Quantity
			total = new(big.Rat).Sub(t.Proceeds, fee)
		}
		if e.TotalAmount, err = cents(total); err != nil {
			return b.errf(t.Line, "total: %v", err)
		}
		b.add(e, rankTrade, t.Line)
	}
	return nil
}

func (b *builder) grants(s *Statement) error {
	for _, g := range s.Grants {
		price, err := cents(g.Price)
		if err != nil {
			return b.errf(g.Line, "price: %v", err)
		}
		e := Event{Symbol: g.Symbol, Time: nyMidnight(g.ReportDate), DateOnly: true, Currency: s.BaseCurrency,
			PricePerUnit: price, NoCash: true}
		switch {
		case strings.Contains(g.Description, "Stock Award Grant for Cash Deposit"):
			e.Kind, e.Source, e.Quantity, e.Note = KindBuy, SourceGrant, g.Quantity, "Stock award grant"
			e.TotalAmount, err = cents(g.Value)
		case strings.Contains(g.Description, "Stock Award Return for Cash Withdrawal"):
			e.Kind, e.Source, e.Quantity, e.Note = KindSell, SourceGrantReturn, absF(g.Quantity), "Stock award return"
			e.TotalAmount, err = cents(absRat(g.Value))
		case strings.Contains(g.Description, "Stock Award Vesting"):
			continue // shares were already counted from the grant
		case strings.Contains(g.Description, "Stock Award Withholding"):
			e.Kind, e.Source, e.Quantity, e.Note = KindSell, SourceGrantWithholding, absF(g.Quantity), "Stock award tax withholding"
			e.TotalAmount, err = cents(absRat(g.Value))
		default:
			return b.errf(g.Line, "unsupported grant activity %q", g.Description)
		}
		if err != nil {
			return b.errf(g.Line, "value: %v", err)
		}
		b.add(e, rankAdjust, g.Line)
	}
	return nil
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func (b *builder) splits(s *Statement) error {
	for _, sp := range s.CorporateActions {
		b.add(Event{Kind: KindSplit, Source: SourceSplit, Symbol: sp.Symbol, Time: sp.Time, Currency: sp.Currency,
			SplitNumerator: sp.Numerator, SplitDenominator: sp.Denominator,
			SplitRatio: float64(sp.Numerator) / float64(sp.Denominator),
			Note:       fmt.Sprintf("Split %d for %d", sp.Numerator, sp.Denominator), NoCash: true}, rankAdjust, sp.Line)
	}
	return nil
}

type divKey struct {
	symbol, currency string
	date             time.Time
}

type divAgg struct {
	hasDiv   bool
	gross    *big.Rat
	cost     *big.Rat // tax plus fees, positive = cost
	kinds    []string
	reversal bool
	line     int // first dividend row
	costLine int
	costDesc string
}

func (b *builder) dividendsAndFees(s *Statement) error {
	aggs := map[divKey]*divAgg{}
	var order []divKey
	get := func(k divKey) *divAgg {
		a := aggs[k]
		if a == nil {
			a = &divAgg{gross: new(big.Rat), cost: new(big.Rat)}
			aggs[k] = a
			order = append(order, k)
		}
		return a
	}

	for _, d := range s.Dividends {
		lower := strings.ToLower(d.Description)
		var kind string
		switch {
		case strings.Contains(lower, "cash dividend"):
			kind = "Cash dividend"
		case strings.Contains(lower, "payment in lieu of dividend"):
			kind = "Payment in lieu of dividend"
		default:
			return b.errf(d.Line, "unsupported dividend description %q", d.Description)
		}
		a := get(divKey{d.Symbol, d.Currency, d.Date})
		if !a.hasDiv {
			a.hasDiv, a.line = true, d.Line
		}
		a.gross.Add(a.gross, d.Amount)
		if strings.Contains(lower, "reversal") {
			a.reversal = true
		}
		if !containsStr(a.kinds, kind) {
			a.kinds = append(a.kinds, kind)
		}
	}
	addCost := func(c CashRow) {
		a := get(divKey{c.Symbol, c.Currency, c.Date})
		a.cost.Sub(a.cost, c.Amount)
		if a.costLine == 0 {
			a.costLine, a.costDesc = c.Line, c.Description
		}
	}
	for _, w := range s.Withholdings {
		addCost(w)
	}
	for _, f := range s.Fees {
		if strings.Contains(f.Description, "Cash Dividend") {
			addCost(f)
			continue
		}
		// Standalone fee (e.g. ADR fee).
		if f.Amount.Sign() == 0 {
			continue
		}
		if f.Amount.Sign() > 0 {
			return b.errf(f.Line, "unsupported fee credit %q", f.Description)
		}
		total, err := cents(absRat(f.Amount))
		if err != nil {
			return b.errf(f.Line, "fee: %v", err)
		}
		b.add(Event{Kind: KindExpense, Source: SourceFee, Symbol: f.Symbol, Time: nyMidnight(f.Date), DateOnly: true,
			Currency: f.Currency, TotalAmount: total, Note: f.Description}, rankCashIncomeExpense, f.Line)
	}

	for _, k := range order {
		a := aggs[k]
		if a.hasDiv && a.gross.Sign() > 0 {
			if a.cost.Sign() < 0 {
				return b.errf(a.costLine, "unsupported tax or fee refund for %s", k.symbol)
			}
			gross, err := cents(a.gross)
			if err != nil {
				return b.errf(a.line, "dividend: %v", err)
			}
			fee, err := cents(a.cost)
			if err != nil {
				return b.errf(a.line, "dividend fee: %v", err)
			}
			note := strings.Join(a.kinds, ", ")
			if a.reversal {
				note += " (net of reversal)"
			}
			b.add(Event{Kind: KindDividend, Source: SourceDividend, Symbol: k.symbol, Time: nyMidnight(k.date),
				DateOnly: true, Currency: k.currency, Fee: fee, TotalAmount: gross, Note: note}, rankCashIncomeExpense, a.line)
			continue
		}
		switch a.cost.Sign() {
		case 0:
		case 1:
			total, err := cents(a.cost)
			if err != nil {
				return b.errf(a.costLine, "withholding: %v", err)
			}
			b.add(Event{Kind: KindExpense, Source: SourceWithholding, Symbol: k.symbol, Time: nyMidnight(k.date),
				DateOnly: true, Currency: k.currency, TotalAmount: total, Note: a.costDesc}, rankCashIncomeExpense, a.costLine)
		default:
			return b.errf(a.costLine, "unsupported tax refund for %s", k.symbol)
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (b *builder) deposits(s *Statement) error {
	for _, d := range s.Deposits {
		if d.Amount.Sign() == 0 {
			continue
		}
		total, err := cents(absRat(d.Amount))
		if err != nil {
			return b.errf(d.Line, "amount: %v", err)
		}
		kind := KindDeposit
		if d.Amount.Sign() < 0 {
			kind = KindWithdrawal
		}
		b.add(Event{Kind: kind, Source: SourceDeposit, Time: nyMidnight(d.Date), DateOnly: true,
			Currency: d.Currency, TotalAmount: total, Note: d.Description}, rankCashFlow, d.Line)
	}
	return nil
}
