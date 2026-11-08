package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
)

const (
	// fxPivotCurrency is the pivot for pairs with no direct or inverse rate.
	fxPivotCurrency = "USD"
	// fxDefaultSource is the source recorded when an input names none.
	fxDefaultSource = "yahoo"
	// fxMaxSourceLen mirrors the fx_rates.source column.
	fxMaxSourceLen = 32
	// fxInsertBatchSize bounds the rows per INSERT (6 columns each).
	fxInsertBatchSize = 500
	// fxLegChunkSize bounds the (base, quote) legs per lookup query (2 params each).
	fxLegChunkSize = 200
)

// fxNonTwoDecimalCurrencies lists ISO 4217 currencies whose minor unit is not
// 1/100; the app stores every amount in cents, so they are not supported (Q8).
var fxNonTwoDecimalCurrencies = map[string]struct{}{
	"BHD": {}, "BIF": {}, "CLP": {}, "DJF": {}, "GNF": {}, "IQD": {}, "ISK": {}, "JOD": {},
	"JPY": {}, "KMF": {}, "KRW": {}, "KWD": {}, "LYD": {}, "OMR": {}, "PYG": {}, "RWF": {},
	"TND": {}, "UGX": {}, "UYI": {}, "VND": {}, "VUV": {}, "XAF": {}, "XOF": {}, "XPF": {},
}

// fxService looks up and records exchange rates.
type fxService struct {
	db           *gorm.DB
	maxStaleness time.Duration
}

// NewFXService creates a new FXServicer. Quotes older than maxStaleness (as of
// the requested time) are flagged Stale.
func NewFXService(db *gorm.DB, maxStaleness time.Duration) FXServicer {
	return &fxService{db: db, maxStaleness: maxStaleness}
}

// isCurrencyCode reports whether code is three upper-case ASCII letters.
func isCurrencyCode(code string) bool {
	if len(code) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}
	return true
}

// normalizeCode trims and upper-cases a currency code.
func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// Rate resolves from→to as of at. See FXServicer.
func (s *fxService) Rate(from, to string, at time.Time) (*FXQuote, error) {
	pair := CurrencyPair{Base: normalizeCode(from), Quote: normalizeCode(to)}
	quotes, err := s.Rates([]CurrencyPair{pair}, at)
	if err != nil {
		return nil, err
	}
	q, ok := quotes[pair]
	if !ok {
		return nil, apperrors.ErrFXRateUnavailable
	}
	return q, nil
}

// fxLeg is a stored observation of one directed pair.
type fxLeg struct {
	rate       fx.Rate
	recordedAt time.Time
}

// Rates resolves many pairs with a bounded number of queries: it collects every
// directed leg any pair could need (the pair, its inverse and the USD legs),
// fetches the latest observation <= at for each, then resolves in Go. The
// returned map is keyed by the upper-cased pair.
func (s *fxService) Rates(pairs []CurrencyPair, at time.Time) (map[CurrencyPair]*FXQuote, error) {
	at = at.UTC()
	out := make(map[CurrencyPair]*FXQuote, len(pairs))

	wanted := make([]CurrencyPair, 0, len(pairs))
	need := map[CurrencyPair]struct{}{}
	for _, p := range pairs {
		p = CurrencyPair{Base: normalizeCode(p.Base), Quote: normalizeCode(p.Quote)}
		if _, done := out[p]; done {
			continue
		}
		if p.Base == "" || p.Quote == "" {
			continue
		}
		if p.Base == p.Quote {
			out[p] = &FXQuote{Base: p.Base, Quote: p.Quote, Rate: fx.Rate(fx.RateScale), RecordedAt: at, Path: FXPathIdentity}
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

	legs, err := s.loadLegs(need, at)
	if err != nil {
		return nil, err
	}
	for _, p := range wanted {
		if _, done := out[p]; done {
			continue
		}
		if q := s.resolve(p, legs, at); q != nil {
			out[p] = q
		}
	}
	return out, nil
}

// loadLegs fetches the latest row <= at for every directed pair in need, with
// one index probe per leg on (base, quote, recorded_at DESC), in chunks so the
// parameter count stays bounded. Postgres uses a LATERAL ... ORDER BY
// recorded_at DESC LIMIT 1 per leg; SQLite (tests) a UNION ALL of the same
// per-leg subselects.
func (s *fxService) loadLegs(need map[CurrencyPair]struct{}, at time.Time) (map[CurrencyPair]fxLeg, error) {
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

	legs := make(map[CurrencyPair]fxLeg, len(keys))
	postgres := s.db.Name() == "postgres"
	for start := 0; start < len(keys); start += fxLegChunkSize {
		chunk := keys[start:min(start+fxLegChunkSize, len(keys))]
		query, args := latestLegsQuery(chunk, at, postgres)

		var rows []models.FXRate
		if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for _, r := range rows {
			legs[CurrencyPair{r.Base, r.Quote}] = fxLeg{rate: r.Rate, recordedAt: r.RecordedAt.UTC()}
		}
	}
	return legs, nil
}

// latestLegsQuery builds the "latest row <= at per leg" query for one chunk.
func latestLegsQuery(chunk []CurrencyPair, at time.Time, postgres bool) (query string, args []interface{}) {
	args = make([]interface{}, 0, len(chunk)*3)
	if postgres {
		holders := make([]string, len(chunk))
		for i, k := range chunk {
			holders[i] = "(?::varchar, ?::varchar)"
			args = append(args, k.Base, k.Quote)
		}
		args = append(args, at)
		return "SELECT f.id, f.base, f.quote, f.rate, f.recorded_at, f.source " +
			"FROM (VALUES " + strings.Join(holders, ", ") + ") AS k(base, quote) " +
			"CROSS JOIN LATERAL (SELECT x.id, x.base, x.quote, x.rate, x.recorded_at, x.source FROM fx_rates x " +
			"WHERE x.base = k.base AND x.quote = k.quote AND x.recorded_at <= ? " +
			"ORDER BY x.recorded_at DESC LIMIT 1) f", args
	}
	parts := make([]string, len(chunk))
	for i, k := range chunk {
		parts[i] = "SELECT * FROM (SELECT id, base, quote, rate, recorded_at, source FROM fx_rates " +
			"WHERE base = ? AND quote = ? AND recorded_at <= ? ORDER BY recorded_at DESC LIMIT 1)"
		args = append(args, k.Base, k.Quote, at)
	}
	return strings.Join(parts, " UNION ALL "), args
}

// resolve applies the lookup order: direct, inverse, then the USD pivot. A
// direct row wins over a newer inverse one. It returns nil when no path exists.
func (s *fxService) resolve(p CurrencyPair, legs map[CurrencyPair]fxLeg, at time.Time) *FXQuote {
	if rate, recordedAt, path, ok := oneLeg(p.Base, p.Quote, legs); ok {
		return s.quote(p, rate, recordedAt, path, at)
	}
	if p.Base == fxPivotCurrency || p.Quote == fxPivotCurrency {
		return nil
	}
	r1, t1, _, ok1 := oneLeg(p.Base, fxPivotCurrency, legs)
	r2, t2, _, ok2 := oneLeg(fxPivotCurrency, p.Quote, legs)
	if !ok1 || !ok2 {
		return nil
	}
	rate, err := fx.Mul(r1, r2)
	if err != nil {
		return nil
	}
	older := t1
	if t2.Before(t1) {
		older = t2
	}
	return s.quote(p, rate, older, FXPathPivotUSD, at)
}

// oneLeg resolves from→to from a stored direct row, else the inverse of a
// stored to→from row.
func oneLeg(from, to string, legs map[CurrencyPair]fxLeg) (rate fx.Rate, recordedAt time.Time, path string, ok bool) {
	if l, found := legs[CurrencyPair{from, to}]; found {
		return l.rate, l.recordedAt, FXPathDirect, true
	}
	if l, found := legs[CurrencyPair{to, from}]; found {
		inv, err := fx.Invert(l.rate)
		if err != nil {
			return 0, time.Time{}, "", false
		}
		return inv, l.recordedAt, FXPathInverse, true
	}
	return 0, time.Time{}, "", false
}

// quote builds an FXQuote and sets its staleness.
func (s *fxService) quote(p CurrencyPair, rate fx.Rate, recordedAt time.Time, path string, at time.Time) *FXQuote {
	return &FXQuote{
		Base:       p.Base,
		Quote:      p.Quote,
		Rate:       rate,
		RecordedAt: recordedAt,
		Stale:      at.Sub(recordedAt) > s.maxStaleness,
		Path:       path,
	}
}

// RecordRates validates and upserts rates on (base, quote, recorded_at),
// updating rate and source on conflict. If a batch repeats a key the last
// entry wins. It returns the number of rows written.
func (s *fxService) RecordRates(rates []FXRateInput) (int, error) {
	if len(rates) == 0 {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "Rates array is empty")
	}

	type key struct {
		base, quote string
		at          time.Time
	}
	index := make(map[key]int, len(rates))
	rows := make([]models.FXRate, 0, len(rates))
	for i, in := range rates {
		source := strings.TrimSpace(in.Source)
		if source == "" {
			source = fxDefaultSource
		}
		var problem string
		switch {
		case !isCurrencyCode(in.Base):
			problem = "base must be a 3-letter upper-case currency code"
		case !isCurrencyCode(in.Quote):
			problem = "quote must be a 3-letter upper-case currency code"
		case in.Base == in.Quote:
			problem = "base and quote must differ"
		case in.Rate <= 0 || in.Rate > fx.MaxRate:
			problem = "rate must be greater than zero and fit NUMERIC(20,10)"
		case in.RecordedAt.IsZero():
			problem = "recorded_at is required"
		case len(source) > fxMaxSourceLen:
			problem = fmt.Sprintf("source must be at most %d characters", fxMaxSourceLen)
		}
		if problem != "" {
			return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("rates[%d]: %s", i, problem))
		}

		row := models.FXRate{Base: in.Base, Quote: in.Quote, Rate: in.Rate, RecordedAt: in.RecordedAt.UTC(), Source: source}
		k := key{in.Base, in.Quote, row.RecordedAt}
		if at, dup := index[k]; dup {
			rows[at] = row
			continue
		}
		index[k] = len(rows)
		rows = append(rows, row)
	}

	var written int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(rows); start += fxInsertBatchSize {
			batch := rows[start:min(start+fxInsertBatchSize, len(rows))]
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "base"}, {Name: "quote"}, {Name: "recorded_at"}},
				DoUpdates: clause.AssignmentColumns([]string{"rate", "source"}),
			}).Create(&batch)
			if result.Error != nil {
				return result.Error
			}
			written += result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return int(written), nil
}

// ListRates returns the stored observations of the direct pair with
// from <= recorded_at <= to, oldest first. It never derives inverse or pivot rates.
func (s *fxService) ListRates(base, quote string, from, to time.Time) ([]models.FXRate, error) {
	rates := []models.FXRate{}
	err := s.db.
		Where("base = ? AND quote = ? AND recorded_at >= ? AND recorded_at <= ?",
			normalizeCode(base), normalizeCode(quote), from.UTC(), to.UTC()).
		Order("recorded_at ASC").
		Find(&rates).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return rates, nil
}

// NeededPairs returns the pairs worth publishing: every currency in use by an
// active account or a security against every user base currency, plus
// (base, USD) for each non-USD base so pivots always work.
func (s *fxService) NeededPairs() (pairs []CurrencyPair, baseCurrencies []string, err error) {
	var accountCurrencies, securityCurrencies []string
	if err := s.db.Model(&models.Account{}).Where("is_active = ?", true).
		Distinct().Pluck("UPPER(currency)", &accountCurrencies).Error; err != nil {
		return nil, nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if err := s.db.Model(&models.Security{}).
		Distinct().Pluck("UPPER(currency)", &securityCurrencies).Error; err != nil {
		return nil, nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	var bases []string
	if err := s.db.Model(&models.User{}).Where("is_active = ?", true).
		Distinct().Pluck("UPPER(base_currency)", &bases).Error; err != nil {
		return nil, nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	currencies := sortedUnique(append(accountCurrencies, securityCurrencies...))
	baseCurrencies = sortedUnique(bases)

	set := map[CurrencyPair]struct{}{}
	for _, b := range baseCurrencies {
		for _, c := range currencies {
			if c != b {
				set[CurrencyPair{Base: c, Quote: b}] = struct{}{}
			}
		}
		if b != fxPivotCurrency {
			set[CurrencyPair{Base: b, Quote: fxPivotCurrency}] = struct{}{}
		}
	}
	pairs = make([]CurrencyPair, 0, len(set))
	for p := range set {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Base != pairs[j].Base {
			return pairs[i].Base < pairs[j].Base
		}
		return pairs[i].Quote < pairs[j].Quote
	})
	return pairs, baseCurrencies, nil
}

// sortedUnique returns the non-blank, trimmed values sorted without duplicates.
// It never returns nil.
func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// IsSupportedCurrency reports whether code can be a user's base currency: a
// 2-decimal ISO-shaped code that is USD or has some rate to USD (staleness
// ignored).
func (s *fxService) IsSupportedCurrency(code string, at time.Time) bool {
	code = normalizeCode(code)
	if !isCurrencyCode(code) {
		return false
	}
	if !isTwoDecimalCurrency(code) {
		return false
	}
	if code == fxPivotCurrency {
		return true
	}
	_, err := s.Rate(code, fxPivotCurrency, at)
	return err == nil
}
