package services

import (
	"sort"
	"strings"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
)

// dayLayout is the layout of the UTC day keys used by RatesByDay and datedValuer.
const dayLayout = "2006-01-02"

// utcDay truncates t to the start of its UTC day.
func utcDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// RatesByDay resolves many pairs for every UTC day in [start, end]. See
// FXServicer. It runs a bounded number of queries regardless of the range: the
// carry-in (latest row before start, via loadLegs) plus one range query per
// chunk of legs, then resolves every day in Go with the same resolve as Rates.
func (s *fxService) RatesByDay(pairs []CurrencyPair, start, end time.Time) (map[CurrencyPair]map[string]*FXQuote, error) {
	first, last := utcDay(start), utcDay(end)
	out := make(map[CurrencyPair]map[string]*FXQuote, len(pairs))
	if last.Before(first) {
		return out, nil
	}
	days := make([]time.Time, 0, int(last.Sub(first).Hours()/24)+1)
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}

	wanted := make([]CurrencyPair, 0, len(pairs))
	need := map[CurrencyPair]struct{}{}
	for _, p := range pairs {
		p = CurrencyPair{Base: normalizeCode(p.Base), Quote: normalizeCode(p.Quote)}
		if _, done := out[p]; done || p.Base == "" || p.Quote == "" {
			continue
		}
		byDay := make(map[string]*FXQuote, len(days))
		out[p] = byDay
		if p.Base == p.Quote {
			for _, d := range days {
				byDay[d.Format(dayLayout)] = &FXQuote{Base: p.Base, Quote: p.Quote, Rate: fx.Rate(fx.RateScale), RecordedAt: endOfDay(d), Path: FXPathIdentity}
			}
			continue
		}
		wanted = append(wanted, p)
		need[CurrencyPair{p.Base, p.Quote}] = struct{}{}
		need[CurrencyPair{p.Quote, p.Base}] = struct{}{}
		if p.Base != fxPivotCurrency && p.Quote != fxPivotCurrency {
			for _, c := range []string{p.Base, p.Quote} {
				need[CurrencyPair{c, fxPivotCurrency}] = struct{}{}
				need[CurrencyPair{fxPivotCurrency, c}] = struct{}{}
			}
		}
	}
	if len(wanted) == 0 {
		return out, nil
	}

	// Carry-in: the latest row strictly before the first day.
	legs, err := s.loadLegs(need, first.Add(-time.Nanosecond))
	if err != nil {
		return nil, err
	}
	rows, err := s.loadLegRows(need, first, endOfDay(last))
	if err != nil {
		return nil, err
	}

	next := 0
	for _, d := range days {
		eod := endOfDay(d)
		for next < len(rows) && !rows[next].RecordedAt.After(eod) {
			r := rows[next]
			legs[CurrencyPair{r.Base, r.Quote}] = fxLeg{rate: r.Rate, recordedAt: r.RecordedAt.UTC()}
			next++
		}
		key := d.Format(dayLayout)
		for _, p := range wanted {
			if q := s.resolve(p, legs, eod); q != nil {
				out[p][key] = q
			}
		}
	}
	return out, nil
}

// loadLegRows fetches the stored rows of the given directed pairs with
// from <= recorded_at <= to, oldest first (on Postgres only the last row of each
// UTC day per leg; SQLite returns every row, which resolves identically), in chunks of fxLegChunkSize legs.
func (s *fxService) loadLegRows(need map[CurrencyPair]struct{}, from, to time.Time) ([]models.FXRate, error) {
	keys := make([]CurrencyPair, 0, len(need))
	for k := range need {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Base != keys[j].Base {
			return keys[i].Base < keys[j].Base
		}
		return keys[i].Quote < keys[j].Quote
	})

	var all []models.FXRate
	for start := 0; start < len(keys); start += fxLegChunkSize {
		chunk := keys[start:min(start+fxLegChunkSize, len(keys))]
		holders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)*2+2)
		for i, k := range chunk {
			holders[i] = "(?, ?)"
			args = append(args, k.Base, k.Quote)
		}
		args = append(args, from.UTC(), to.UTC())
		var rows []models.FXRate
		// Only the last row per (leg, UTC day) matters to RatesByDay (it takes
		// the latest row <= end of day), so Postgres collapses intraday rows.
		selectCols := "SELECT id, base, quote, rate, recorded_at, source FROM fx_rates "
		orderBy := ""
		if s.db.Name() == "postgres" {
			selectCols = "SELECT DISTINCT ON (base, quote, (recorded_at AT TIME ZONE 'UTC')::date) " +
				"id, base, quote, rate, recorded_at, source FROM fx_rates "
			orderBy = " ORDER BY base, quote, (recorded_at AT TIME ZONE 'UTC')::date, recorded_at DESC"
		}
		err := s.db.Raw(
			selectCols+
				"WHERE (base, quote) IN ("+strings.Join(holders, ", ")+") "+
				"AND recorded_at >= ? AND recorded_at <= ?"+orderBy,
			args...,
		).Scan(&rows).Error
		if err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		all = append(all, rows...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].RecordedAt.Before(all[j].RecordedAt) })
	return all, nil
}

// datedValuer converts native amounts into one target currency at the rate of a
// given UTC day, with all rates fetched by one RatesByDay call (identity needs
// no query).
type datedValuer struct {
	target string
	quotes map[CurrencyPair]map[string]*FXQuote
}

// newDatedValuer builds a datedValuer for target over [start, end], fetching
// day rates for every distinct non-target currency in currencies. A nil fxs is
// allowed: then only identity conversion succeeds. No query is made when every
// currency equals the target.
func newDatedValuer(fxs FXServicer, target string, start, end time.Time, currencies []string) (*datedValuer, error) {
	target = normalizeCode(target)
	v := &datedValuer{target: target, quotes: map[CurrencyPair]map[string]*FXQuote{}}
	if fxs == nil {
		return v, nil
	}
	seen := map[string]struct{}{}
	pairs := make([]CurrencyPair, 0, len(currencies))
	for _, c := range currencies {
		c = normalizeCode(c)
		if c == "" || c == target {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		pairs = append(pairs, CurrencyPair{Base: c, Quote: target})
	}
	if len(pairs) == 0 {
		return v, nil
	}
	quotes, err := fxs.RatesByDay(pairs, start, end)
	if err != nil {
		return nil, err
	}
	if quotes != nil {
		v.quotes = quotes
	}
	return v, nil
}

// Convert returns amount (in currency) in the target currency at the rate of
// day ("YYYY-MM-DD", UTC). ok is false when no rate exists for that day or the
// result overflows; it never falls back to 1:1.
func (v *datedValuer) Convert(amount int64, currency, day string) (int64, bool) {
	currency = normalizeCode(currency)
	if currency == v.target || currency == "" {
		return amount, true
	}
	q := v.quotes[CurrencyPair{Base: currency, Quote: v.target}][day]
	if q == nil {
		return 0, false
	}
	out, err := fx.Convert(amount, q.Rate)
	if err != nil {
		return 0, false
	}
	return out, true
}
