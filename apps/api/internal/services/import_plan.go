package services

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/importers/ibkr"
	"kuberan/internal/models"
)

// ibkrExchange maps an IBKR listing exchange to Kuberan's exchange code; other
// values pass through unchanged.
func ibkrExchange(listing string) string {
	switch strings.ToUpper(strings.TrimSpace(listing)) {
	case "LSEETF", "LSE":
		return "LSE"
	case "NASDAQ":
		return "NASDAQ"
	case "NYSE":
		return "NYSE"
	default:
		return strings.TrimSpace(listing)
	}
}

// resolveSecurity finds the security for an IBKR symbol: by (symbol, exchange)
// case-insensitively, else by symbol alone when exactly one security has it.
// It never creates securities. The bool is false when none could be chosen.
func resolveSecurity(db *gorm.DB, symbol, listing string) (*models.Security, bool, error) {
	var cands []models.Security
	if err := db.Where("UPPER(symbol) = ?", strings.ToUpper(symbol)).Order("created_at ASC, id ASC").Find(&cands).Error; err != nil {
		return nil, false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if ex := ibkrExchange(listing); ex != "" {
		for i := range cands {
			if strings.EqualFold(cands[i].Exchange, ex) {
				return &cands[i], true, nil
			}
		}
	}
	if len(cands) == 1 {
		return &cands[0], true, nil
	}
	return nil, false, nil
}

func isInvestmentKind(k ibkr.EventKind) bool {
	switch k {
	case ibkr.KindBuy, ibkr.KindSell, ibkr.KindSplit, ibkr.KindDividend:
		return true
	default:
		return false
	}
}

func investmentTypeOf(k ibkr.EventKind) models.InvestmentTransactionType {
	switch k {
	case ibkr.KindBuy:
		return models.InvestmentTransactionBuy
	case ibkr.KindSell:
		return models.InvestmentTransactionSell
	case ibkr.KindSplit:
		return models.InvestmentTransactionSplit
	default:
		return models.InvestmentTransactionDividend
	}
}

// planSecurities resolves the securities and investments, builds the new
// investment transaction rows and replays them to compare with each checkpoint.
func (s *importService) planSecurities(db *gorm.DB, pl *importPlan, rates *importRates) error {
	set := pl.set
	bySymbol := map[string][]ibkr.Event{}
	for i := range set.Events {
		if e := set.Events[i]; isInvestmentKind(e.Kind) {
			bySymbol[e.Symbol] = append(bySymbol[e.Symbol], e)
		}
	}
	for _, cp := range set.Checkpoints {
		for sym := range cp.Positions {
			if _, ok := bySymbol[sym]; !ok {
				bySymbol[sym] = nil
			}
		}
	}
	symbols := make([]string, 0, len(bySymbol))
	for sym := range bySymbol {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	for _, sym := range symbols {
		events := bySymbol[sym]
		sec, found, err := resolveSecurity(db, sym, set.Instruments[sym].ListingExchange)
		if err != nil {
			return err
		}
		if !found {
			pl.addError("unknown_security", "no security matches symbol %s (exchange %q); create it first, the import never creates securities",
				sym, ibkrExchange(set.Instruments[sym].ListingExchange))
			continue
		}
		secCcy := normalizeCode(sec.Currency)
		currencies := map[string]bool{}
		for i := range events {
			if events[i].Kind != ibkr.KindSplit {
				currencies[normalizeCode(events[i].Currency)] = true
			}
		}
		for _, cp := range set.Checkpoints {
			if pos, ok := cp.Positions[sym]; ok {
				currencies[normalizeCode(pos.Currency)] = true
			}
		}
		mismatch := false
		for c := range currencies {
			if c != secCcy {
				mismatch = true
			}
		}
		if mismatch {
			pl.addError("currency_mismatch", "%s trades in %s but the security is stored in %s; run the Phase 2 currency reconcile first",
				sym, joinKeys(currencies), secCcy)
			continue
		}

		sp := &symbolPlan{symbol: sym, sec: sec}
		var invs []models.Investment
		if err := db.Where("account_id = ? AND security_id = ?", pl.invAcct.ID, sec.ID).
			Order("created_at ASC, id ASC").Limit(1).Find(&invs).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if len(invs) > 0 {
			sp.inv = &invs[0]
			if err := s.classifyExisting(db, pl, sp); err != nil {
				return err
			}
		}
		if err := s.replaySymbol(pl, rates, sp, events); err != nil {
			return err
		}
		pl.symbols = append(pl.symbols, sp)
	}
	return nil
}

// coverageEnd is the instant the statements stop covering: the end of the last
// period's day in America/New_York. Existing transactions dated at or after it
// are outside the import and must never be replaced.
func (pl *importPlan) coverageEnd() time.Time {
	last := pl.set.Periods[len(pl.set.Periods)-1].End
	return time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, ibkr.NYLocation)
}

// classifyExisting splits a holding's existing transactions into the ones the
// import replaces (before the coverage end) and the ones after it, which are
// reported as an error rather than deleted or mixed in.
func (s *importService) classifyExisting(db *gorm.DB, pl *importPlan, sp *symbolPlan) error {
	var rows []models.InvestmentTransaction
	if err := db.Select("id", "date", "type", "quantity").Where("investment_id = ?", sp.inv.ID).
		Order("date ASC, created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	cutoff := pl.coverageEnd()
	lastDay := pl.set.Periods[len(pl.set.Periods)-1].End
	for i := range rows {
		r := &rows[i]
		// Hand-entered trades carry a date only (UTC midnight), which for the day
		// after the statements lies before the New York cutoff; they still count
		// as after the coverage.
		dateOnlyAfter := r.Date.UTC().Equal(truncDay(r.Date)) && truncDay(r.Date).After(lastDay)
		if r.Date.Before(cutoff) && !dateOnlyAfter {
			sp.replaceIDs = append(sp.replaceIDs, r.ID)
			continue
		}
		pl.addError("trades_after_statement",
			"%s has a %s of %g on %s (id %s) after the statements end; export an Activity Statement that runs through today and include it",
			sp.symbol, r.Type, r.Quantity, r.Date.UTC().Format(importDateLayout), r.ID)
	}
	sp.existing = int64(len(sp.replaceIDs))
	return nil
}

func joinKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "/")
}

// replaySymbol builds the symbol's investment transaction rows and replays
// them with holdingState.apply, recording a checkpoint comparison at each
// statement end date. It writes nothing.
func (s *importService) replaySymbol(pl *importPlan, rates *importRates, sp *symbolPlan, events []ibkr.Event) error {
	var st holdingState
	failed := false
	cps := pl.set.Checkpoints
	ci := 0
	record := func(cp ibkr.Checkpoint) {
		pos := cp.Positions[sp.symbol]
		sp.checkpoints = append(sp.checkpoints, IBKRImportCheckpoint{
			Date:          cp.Date,
			IBKRQuantity:  pos.Quantity,
			Quantity:      st.Quantity,
			QuantityOK:    math.Abs(st.Quantity-pos.Quantity) <= importQuantityTolerance,
			IBKRCostBasis: pos.CostBasisCents,
			CostBasis:     st.CostBasis,
			CostBasisDiff: st.CostBasis - pos.CostBasisCents,
		})
		if d := st.CostBasis - pos.CostBasisCents; absInt64(d) > importCostWarnCents {
			pl.warn("%s cost basis at %s differs from IBKR by %d cents (average cost vs lot-based)", sp.symbol, cp.Date, d)
		}
	}

	for ei := range events {
		e := &events[ei]
		for ci < len(cps) && e.Date > cps[ci].Date {
			record(cps[ci])
			ci++
		}
		row := models.InvestmentTransaction{
			Type:         investmentTypeOf(e.Kind),
			Date:         e.Time,
			Notes:        e.Note,
			PricePerUnit: e.PricePerUnit,
			Fee:          e.Fee,
		}
		te := tradeEvent{Type: row.Type}
		switch e.Kind {
		case ibkr.KindBuy, ibkr.KindSell:
			row.Quantity, row.TotalAmount = e.Quantity, e.TotalAmount
			te.Quantity, te.TotalAmount = e.Quantity, e.TotalAmount
		case ibkr.KindDividend:
			row.TotalAmount, row.DividendType = e.TotalAmount, "Cash"
		case ibkr.KindSplit:
			row.Quantity, row.SplitRatio = st.Quantity, e.SplitRatio
			te.SplitRatio = e.SplitRatio
		}
		if e.Kind != ibkr.KindSplit {
			rate, ok, err := rates.at(e.Currency, e.Time)
			if err != nil {
				return err
			}
			if !ok {
				if pl.fxMissing == 0 || e.Date < pl.fxMissingFirst {
					pl.fxMissingFirst, pl.fxMissingCcy = e.Date, normalizeCode(e.Currency)
				}
				pl.fxMissing++
			} else {
				base, err := baseAmountFor(e.TotalAmount, rate)
				if err != nil {
					return err
				}
				row.FXRateToBase, row.BaseAmount, te.BaseAmount = &rate, &base, base
			}
		}
		sp.rows = append(sp.rows, row)
		if failed || e.Kind == ibkr.KindDividend {
			continue
		}
		res, err := st.apply(te)
		if err != nil {
			pl.addError("insufficient_shares", "%s: the %s on %s (%g shares) exceeds the position held at that point",
				sp.symbol, e.Kind, e.Date, e.Quantity)
			failed = true
			continue
		}
		sp.rows[len(sp.rows)-1].RealizedGainLoss = res.RealizedGainLoss
		sp.rows[len(sp.rows)-1].RealizedGainLossBase = res.RealizedGainLossBase
	}
	for ; ci < len(cps); ci++ {
		record(cps[ci])
	}
	sp.final = st
	return nil
}

// planCash builds the cash ledger: opening balance, trade legs, dividends,
// fees, and the deposit matching against the legacy account's transfers.
func (s *importService) planCash(db *gorm.DB, pl *importPlan) error {
	set, start := pl.set, pl.params.CashStartDate.UTC().Format(importDateLayout)
	prev := pl.params.CashStartDate.UTC().AddDate(0, 0, -1).Format(importDateLayout)
	prefix := importRefPrefix(set.Account)

	var openingCP *ibkr.Checkpoint
	for i := range set.Checkpoints {
		if set.Checkpoints[i].Date == prev {
			openingCP = &set.Checkpoints[i]
		}
	}
	switch {
	case openingCP == nil || !openingCP.HasEndingCash:
		pl.addError("opening_balance_missing", "no statement with an Ending Cash ends on %s, the day before cash_start_date", prev)
	case openingCP.EndingCashCents < 0:
		pl.addError("opening_balance_negative", "the Ending Cash on %s is negative; an opening balance cannot be", prev)
	default:
		pl.opening = openingCP.EndingCashCents
	}

	secByName := map[string]string{}
	for _, sp := range pl.symbols {
		secByName[sp.symbol] = sp.sec.Name
	}
	name := func(sym string) string {
		if n := secByName[sym]; n != "" {
			return n
		}
		return sym
	}

	var deposits []ibkr.Event
	badCcy := map[string]bool{}
	for i := range pl.set.Events {
		e := set.Events[i]
		if e.Date < start || e.NoCash {
			continue
		}
		if normalizeCode(e.Currency) != normalizeCode(pl.cash.Currency) {
			if !badCcy[e.Currency] {
				badCcy[e.Currency] = true
				pl.addError("unsupported_currency", "cash events in %s cannot go to the %s cash account", e.Currency, pl.cash.Currency)
			}
			continue
		}
		ref := prefix + e.Key
		row := cashRow{typ: models.TransactionTypeInvestment, date: e.Time}
		switch e.Kind {
		case ibkr.KindBuy:
			row.effect, row.amount = models.TransactionTypeExpense, e.TotalAmount
			row.desc = fmt.Sprintf("%s%g %s", importBuyDesc, e.Quantity, name(e.Symbol))
			pl.legs++
		case ibkr.KindSell:
			row.effect, row.amount = models.TransactionTypeIncome, e.TotalAmount
			row.desc = fmt.Sprintf("%s%g %s", importSellDesc, e.Quantity, name(e.Symbol))
			pl.legs++
		case ibkr.KindDividend:
			net := e.TotalAmount - e.Fee
			row.effect, row.amount = models.TransactionTypeIncome, net
			if net < 0 {
				row.effect, row.amount = models.TransactionTypeExpense, -net
			}
			row.desc = importDivDesc + name(e.Symbol)
			pl.dividends++
		case ibkr.KindExpense:
			row.typ, row.effect, row.amount = models.TransactionTypeExpense, models.TransactionTypeExpense, e.TotalAmount
			row.desc = importDescPrefix + e.Note
			pl.fees++
		case ibkr.KindDeposit, ibkr.KindWithdrawal:
			deposits = append(deposits, e)
			continue
		default:
			continue
		}
		row.ref = ref + importDirSuffix(row.effect)
		if len(row.ref) > importRefMax {
			pl.addError("ref_too_long", "event reference %q is longer than %d characters", row.ref, importRefMax)
			continue
		}
		if len([]rune(row.desc)) > MaxDescriptionLen {
			row.desc = string([]rune(row.desc)[:MaxDescriptionLen])
		}
		if row.amount > 0 {
			pl.cashRows = append(pl.cashRows, row)
		}
	}

	if err := s.checkPriorRefs(db, pl, prefix); err != nil {
		return err
	}
	if err := s.matchDeposits(db, pl, deposits, prefix); err != nil {
		return err
	}
	pl.cashCheckpoints(start)
	return nil
}

// importDirSuffix is the balance direction of a cash row, kept in its import_ref:
// ":dr" when the row debited the account, ":cr" when it credited it. Opening
// balances credit. The reversal on re-import reads the direction only from here.
func importDirSuffix(effect models.TransactionType) string {
	if effect == models.TransactionTypeExpense {
		return ":dr"
	}
	return ":cr"
}

// importRefCredit parses the direction suffix of an import_ref.
func importRefCredit(ref *string) (credit, ok bool) {
	if ref == nil {
		return false, false
	}
	switch {
	case strings.HasSuffix(*ref, ":cr"):
		return true, true
	case strings.HasSuffix(*ref, ":dr"):
		return false, true
	}
	return false, false
}

// checkPriorRefs reports earlier import rows whose direction cannot be read
// from their import_ref; such an import is never applied (no guessing).
func (s *importService) checkPriorRefs(db *gorm.DB, pl *importPlan, prefix string) error {
	var rows []models.Transaction
	if err := db.Select("id", "import_ref").
		Where("user_id = ? AND type <> ? AND import_ref LIKE ?", pl.userID, models.TransactionTypeTransfer, prefix+"%").
		Find(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	bad := 0
	for i := range rows {
		if _, ok := importRefCredit(rows[i].ImportRef); !ok {
			bad++
		}
	}
	if bad > 0 {
		pl.addError("import_ref_invalid", "%d earlier import row(s) have an unreadable import_ref direction; fix or remove them before importing", bad)
	}
	return nil
}

func importRefPrefix(account string) string { return "ibkr:" + account + ":" }

// importDepositTolerance is the largest relative deviation of a transfer's
// implied exchange rate from the market rate for it to be matched to a deposit.
var importDepositTolerance = big.NewRat(3, 100)

// depositCandidate is a legacy transfer considered for a deposit.
type depositCandidate struct {
	idx       int
	dist      int
	deviation *big.Rat // nil when no market rate exists
	pct       float64
}

// matchDeposits pairs each deposit, in date order, with a transfer to the
// legacy account (or a transfer an earlier import already re-pointed, found
// through its import_ref). A candidate is within +-5 days and its implied rate
// (source amount / deposit amount) must be within 3% of the market rate on the
// deposit date; the smallest deviation wins, then the nearest date, then the
// earliest created_at. Without a market rate the nearest date wins.
func (s *importService) matchDeposits(db *gorm.DB, pl *importPlan, deposits []ibkr.Event, prefix string) error {
	prior := map[string]models.Transaction{}
	var cands []models.Transaction
	if len(deposits) > 0 {
		var rows []models.Transaction
		if err := db.Where("user_id = ? AND type = ? AND import_ref LIKE ?", pl.userID, models.TransactionTypeTransfer, prefix+"%").
			Find(&rows).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for i := range rows {
			if rows[i].ImportRef != nil {
				prior[*rows[i].ImportRef] = rows[i]
			}
		}
		if pl.legacy != nil {
			if err := db.Where("user_id = ? AND type = ? AND to_account_id = ? AND import_ref IS NULL",
				pl.userID, models.TransactionTypeTransfer, pl.legacy.ID).
				Order("date ASC, created_at ASC, id ASC").Find(&cands).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
	}
	currencies := map[string]string{}
	currencyOf := func(accountID string) (string, error) {
		if c, ok := currencies[accountID]; ok {
			return c, nil
		}
		var codes []string
		if err := db.Model(&models.Account{}).Where("id = ?", accountID).Limit(1).Pluck("currency", &codes).Error; err != nil {
			return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		c := ""
		if len(codes) > 0 {
			c = normalizeCode(codes[0])
		}
		currencies[accountID] = c
		return c, nil
	}

	matched := make([]*depositMatch, len(deposits))
	for i := range deposits {
		d := &deposits[i]
		if d.Kind != ibkr.KindDeposit {
			continue
		}
		if tr, ok := prior[prefix+d.Key]; ok && tr.ToAccountID != nil {
			m := &depositMatch{ev: *d, tr: tr, toAmount: d.TotalAmount}
			m.changed = *tr.ToAccountID != pl.cash.ID || tr.EffectiveToAmount() != d.TotalAmount
			matched[i] = m
		}
	}

	rejected := map[int][]IBKRImportCandidate{}
	usedCand := map[int]bool{}
	for i := range deposits {
		d := &deposits[i]
		if matched[i] != nil || d.Kind != ibkr.KindDeposit || d.TotalAmount <= 0 {
			continue
		}
		day, err := time.Parse(importDateLayout, d.Date)
		if err != nil {
			continue
		}
		var ok []depositCandidate
		var all []depositCandidate
		noMarket := false
		for j := range cands {
			if usedCand[j] {
				continue
			}
			dist := int(math.Abs(day.Sub(truncDay(cands[j].Date)).Hours() / 24))
			if dist > importDepositWindowDays {
				continue
			}
			c := depositCandidate{idx: j, dist: dist}
			srcCcy, err := currencyOf(cands[j].AccountID)
			if err != nil {
				return err
			}
			var market fx.Rate
			haveMarket := false
			if pl.fxs != nil && srcCcy != "" {
				q, err := pl.fxs.Rate(normalizeCode(d.Currency), srcCcy, d.Time)
				switch {
				case err == nil:
					market, haveMarket = q.Rate, true
				case errors.Is(err, apperrors.ErrFXRateUnavailable):
				default:
					return err
				}
			}
			if haveMarket {
				implied := new(big.Rat).SetFrac64(cands[j].Amount, d.TotalAmount)
				marketRat := new(big.Rat).SetFrac64(int64(market), fx.RateScale)
				dev := new(big.Rat).Quo(implied, marketRat)
				dev.Sub(dev, big.NewRat(1, 1))
				c.pct, _ = new(big.Rat).Mul(dev, big.NewRat(100, 1)).Float64()
				dev.Abs(dev)
				c.deviation = dev
			} else {
				noMarket = true
			}
			all = append(all, c)
			if c.deviation == nil || c.deviation.Cmp(importDepositTolerance) <= 0 {
				ok = append(ok, c)
			}
		}
		if noMarket {
			pl.warn("no market exchange rate for the %s deposit on %s; matched by nearest date", d.Currency, d.Date)
		}
		if len(ok) == 0 {
			for _, c := range all {
				rejected[i] = append(rejected[i], IBKRImportCandidate{
					TransactionID: cands[c.idx].ID, Date: truncDay(cands[c.idx].Date).Format(importDateLayout),
					Amount: cands[c.idx].Amount, DeviationPct: math.Round(c.pct*100) / 100,
				})
			}
			continue
		}
		sort.SliceStable(ok, func(a, b int) bool {
			x, y := ok[a], ok[b]
			if x.deviation != nil && y.deviation != nil {
				if c := x.deviation.Cmp(y.deviation); c != 0 {
					return c < 0
				}
			}
			if x.dist != y.dist {
				return x.dist < y.dist
			}
			return cands[x.idx].CreatedAt.Before(cands[y.idx].CreatedAt)
		})
		best := ok[0]
		usedCand[best.idx] = true
		matched[i] = &depositMatch{ev: *d, tr: cands[best.idx], toAmount: d.TotalAmount, changed: true}
	}

	var legacyAfter int64
	if pl.legacy != nil {
		legacyAfter = pl.legacy.Balance
	}
	for i := range deposits {
		d, m := &deposits[i], matched[i]
		if m == nil {
			detail := "no transfer to the legacy cash account within 5 days and 3% of the market rate; record it by hand"
			pl.unmatched = append(pl.unmatched, IBKRImportUnmatched{
				Date: d.Date, Kind: string(d.Kind), Amount: d.TotalAmount, Detail: detail, Candidates: rejected[i],
			})
			continue
		}
		pl.matches = append(pl.matches, *m)
		if m.changed && pl.legacy != nil && m.tr.ToAccountID != nil && *m.tr.ToAccountID == pl.legacy.ID {
			legacyAfter -= m.tr.EffectiveToAmount()
		}
	}
	if pl.legacy != nil {
		pl.legacyAfter = &legacyAfter
	}
	return nil
}

func truncDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// cashCheckpoints compares the ledger (what the account holds because of the
// import) with Ending Cash at every statement end on or after the start date.
func (pl *importPlan) cashCheckpoints(start string) {
	matchedEv := map[string]bool{}
	for i := range pl.matches {
		matchedEv[pl.matches[i].ev.Key] = true
	}
	for _, cp := range pl.set.Checkpoints {
		if cp.Date < start || !cp.HasEndingCash {
			continue
		}
		ledger, rows := pl.opening, 0
		for i := range pl.set.Events {
			e := pl.set.Events[i]
			if e.Date < start || e.Date > cp.Date || e.NoCash {
				continue
			}
			switch e.Kind {
			case ibkr.KindDeposit, ibkr.KindWithdrawal:
				if !matchedEv[e.Key] {
					continue
				}
			case ibkr.KindBuy, ibkr.KindSell, ibkr.KindDividend, ibkr.KindExpense:
			default:
				continue
			}
			ledger += e.CashDelta()
			rows++
		}
		diff := cp.EndingCashCents - ledger
		pl.cashChecks = append(pl.cashChecks, IBKRImportCashCheckpoint{Date: cp.Date, IBKR: cp.EndingCashCents, Ledger: ledger, Diff: diff})
		if absInt64(diff) > int64(rows) {
			pl.warn("cash at %s differs from IBKR by %d cents (more than 1 cent per row over %d rows)", cp.Date, diff, rows)
		}
	}
	if len(pl.unmatched) > 0 {
		pl.warn("%d deposit/withdrawal(s) have no matching transfer; the cash ledger excludes them until recorded by hand", len(pl.unmatched))
	}
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// report renders the plan.
func (pl *importPlan) report(applied bool) *IBKRImportReport {
	r := &IBKRImportReport{
		Applied: applied, Account: pl.set.Account, BaseCurrency: pl.set.BaseCurrency,
		Periods:         []IBKRImportPeriod{},
		IgnoredSections: append([]string{}, pl.set.IgnoredSections...),
		Securities:      []IBKRImportSecurity{},
		Errors:          append([]IBKRImportIssue{}, pl.errs...),
		Warnings:        append([]string{}, pl.warnings...),
	}
	for _, p := range pl.set.Periods {
		r.Periods = append(r.Periods, IBKRImportPeriod{Start: p.Start.Format(importDateLayout), End: p.End.Format(importDateLayout)})
	}
	for _, sp := range pl.symbols {
		var invID *string
		if sp.inv != nil {
			id := sp.inv.ID
			invID = &id
		}
		r.Securities = append(r.Securities, IBKRImportSecurity{
			Symbol: sp.symbol, SecurityID: sp.sec.ID, InvestmentID: invID,
			ExistingTransactions: sp.existing, NewTransactions: len(sp.rows),
			Final: IBKRImportFinal{
				Quantity: sp.final.Quantity, CostBasis: sp.final.CostBasis, CostBasisBase: sp.final.CostBasisBase,
				RealizedGainLoss: sp.final.RealizedGainLoss, RealizedGainLossBase: sp.final.RealizedGainLossBase,
			},
			Checkpoints: sp.checkpoints,
		})
	}
	r.Cash = IBKRImportCash{
		OpeningBalance: pl.opening, Legs: pl.legs, Dividends: pl.dividends, Fees: pl.fees,
		DepositsMatched: []IBKRImportDeposit{}, UnmatchedDeposits: append([]IBKRImportUnmatched{}, pl.unmatched...),
		Checkpoints: append([]IBKRImportCashCheckpoint{}, pl.cashChecks...), LegacyAccountBalanceAfter: pl.legacyAfter,
	}
	for i := range pl.matches {
		m := &pl.matches[i]
		r.Cash.DepositsMatched = append(r.Cash.DepositsMatched, IBKRImportDeposit{
			Date: m.ev.Date, Amount: m.toAmount, TransactionID: m.tr.ID, SourceAmount: m.tr.Amount,
		})
	}
	return r
}
