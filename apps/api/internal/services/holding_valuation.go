package services

import (
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
)

// nativePrice is a security's latest price in its own currency.
type nativePrice struct {
	Price      int64
	Currency   string
	RecordedAt time.Time
}

// latestNativePrices returns the latest price row per security (securities with
// no price are absent) together with the security's currency
// (UPPER(securities.currency)).
//
// Both dialects read one row per security with one backward probe of the unique
// (security_id, recorded_at) index: Postgres through a LATERAL ... ORDER BY
// recorded_at DESC LIMIT 1 per security (DISTINCT ON sorts every price row of
// the ids instead), SQLite (tests) through a per-security subselect.
func latestNativePrices(db *gorm.DB, securityIDs []string) (map[string]nativePrice, error) {
	if len(securityIDs) == 0 {
		return map[string]nativePrice{}, nil
	}
	ids := uniqueStrings(securityIDs)

	type priceRow struct {
		SecurityID string
		Price      int64
		RecordedAt time.Time
		Currency   string
	}
	var rows []priceRow

	var err error
	if db.Name() == "postgres" {
		err = db.Raw(
			"SELECT s.id AS security_id, p.price AS price, p.recorded_at AS recorded_at, UPPER(s.currency) AS currency "+
				"FROM securities s CROSS JOIN LATERAL ("+
				"SELECT sp.price, sp.recorded_at FROM security_prices sp WHERE sp.security_id = s.id "+
				"ORDER BY sp.recorded_at DESC LIMIT 1) p WHERE s.id IN ?", ids,
		).Scan(&rows).Error
	} else {
		err = db.Table("security_prices sp").
			Select("sp.security_id AS security_id, sp.price AS price, sp.recorded_at AS recorded_at, UPPER(s.currency) AS currency").
			Joins("JOIN securities s ON s.id = sp.security_id").
			Where("sp.security_id IN ?", ids).
			Where("sp.id = (SELECT x.id FROM security_prices x WHERE x.security_id = sp.security_id ORDER BY x.recorded_at DESC LIMIT 1)").
			Scan(&rows).Error
	}
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := make(map[string]nativePrice, len(rows))
	for _, r := range rows {
		result[r.SecurityID] = nativePrice{Price: r.Price, Currency: strings.TrimSpace(r.Currency), RecordedAt: r.RecordedAt}
	}
	return result, nil
}

// valuer converts native amounts into one target currency at one instant, with
// all the FX rates it needs fetched in ONE FXServicer.Rates call (identity
// needs no query).
type valuer struct {
	target string
	at     time.Time
	quotes map[CurrencyPair]*FXQuote
}

// newValuer builds a valuer for target at the given instant, fetching a quote
// for every distinct non-target currency in currencies. A nil fxs is allowed
// (tests only): then only identity conversion succeeds.
func newValuer(fxs FXServicer, target string, at time.Time, currencies []string) (*valuer, error) {
	target = normalizeCode(target)
	v := &valuer{target: target, at: at, quotes: map[CurrencyPair]*FXQuote{}}
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

	quotes, err := fxs.Rates(pairs, at)
	if err != nil {
		return nil, err
	}
	if quotes != nil {
		v.quotes = quotes
	}
	return v, nil
}

// Convert returns amount (in currency) in the target currency. ok is false when
// no rate exists or the result overflows. The quote is nil for an identity
// conversion.
func (v *valuer) Convert(amount int64, currency string) (converted int64, quote *FXQuote, ok bool) {
	currency = normalizeCode(currency)
	if currency == v.target || currency == "" {
		return amount, nil, true
	}
	q := v.quotes[CurrencyPair{Base: currency, Quote: v.target}]
	if q == nil {
		return 0, nil, false
	}
	out, err := fx.Convert(amount, q.Rate)
	if err != nil {
		return 0, nil, false
	}
	return out, q, true
}

// defaultBaseCurrency mirrors the users.base_currency column default; it is the
// fallback when a user row cannot be read.
const defaultBaseCurrency = "MYR"

// userBaseCurrency returns the user's base currency (upper-case).
func userBaseCurrency(db *gorm.DB, userID string) (string, error) {
	var codes []string
	if err := db.Table("users").Where("id = ?", userID).Limit(1).
		Pluck("base_currency", &codes).Error; err != nil {
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(codes) == 0 || strings.TrimSpace(codes[0]) == "" {
		return defaultBaseCurrency, nil
	}
	return normalizeCode(codes[0]), nil
}

// distinctPriceCurrencies returns the distinct currencies of the given prices.
func distinctPriceCurrencies(prices map[string]nativePrice) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	for _, p := range prices {
		if _, ok := seen[p.Currency]; ok {
			continue
		}
		seen[p.Currency] = struct{}{}
		out = append(out, p.Currency)
	}
	return out
}
