package services

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// Range caps for the bucketed aggregates. They live in the service layer so the
// REST handlers and the MCP tools are protected alike.
const (
	// maxDailyRangeDays is the longest span (to - from, in whole days) accepted
	// by the daily aggregates; at most maxDailyRangeDays+1 buckets are returned.
	maxDailyRangeDays = 366
	// minSummaryMonths and maxSummaryMonths bound GetMonthlySummary's months.
	minSummaryMonths = 1
	maxSummaryMonths = 24
)

// periodGranularity selects the bucket size for periodTotals.
type periodGranularity int

const (
	periodDay periodGranularity = iota
	periodMonth
)

// goLayout is the Go time layout of a bucket key ("2006-01-02" / "2006-01").
func (g periodGranularity) goLayout() string {
	if g == periodMonth {
		return "2006-01"
	}
	return "2006-01-02"
}

// periodTotal holds the income and expense totals of one bucket, in the target
// (base) currency.
type periodTotal struct {
	Income   int64
	Expenses int64
	// FXMissing is true when part of the bucket had no exchange rate and was
	// left out of the totals.
	FXMissing bool
}

// dayExprs returns the SELECT expression that yields a transaction's UTC day
// and the matching GROUP BY expression. On Postgres the day is bucketed in SQL
// (alias "bucket"); on other dialects (SQLite in tests) rows are grouped by
// their exact timestamp and bucketed in Go by dayKey.
func dayExprs(db *gorm.DB) (selectExpr, groupExpr string, isPostgres bool) {
	if db.Name() == "postgres" {
		return "to_char(t.date AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS bucket", "bucket", true
	}
	return "t.date AS at", "t.date", false
}

// dayKey returns the UTC day key ("YYYY-MM-DD") of a grouped row.
func dayKey(isPostgres bool, bucket string, at time.Time) string {
	if isPostgres {
		return bucket
	}
	return at.UTC().Format(dayLayout)
}

// dayBounds returns the first and last day (as UTC instants) of a set of day keys.
func dayBounds(days map[string]struct{}) (first, last time.Time, ok bool) {
	for d := range days {
		t, err := time.Parse(dayLayout, d)
		if err != nil {
			continue
		}
		if !ok || t.Before(first) {
			first = t
		}
		if !ok || t.After(last) {
			last = t
		}
		ok = true
	}
	return first, last, ok
}

// currencyList returns the keys of a set.
func currencyList(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// unconvertedList turns a currency->amount map into a sorted slice (never nil).
func unconvertedList(m map[string]int64) []UnconvertedAmount {
	out := make([]UnconvertedAmount, 0, len(m))
	for c, amt := range m {
		out = append(out, UnconvertedAmount{Currency: c, Amount: amt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

// periodTotals sums a user's income and effective expenses between start and
// end (inclusive), converted to the user's base currency, and returns them
// keyed by UTC bucket (goLayout format).
//
// One grouped query joins accounts and groups by (UTC day, account currency,
// type). Each group is then converted at its own day's rate (datedValuer; the
// fast path makes no FX query when every group is in the base currency) and
// rolled up to the day or month bucket in Go, so a monthly figure is the sum of
// daily conversions. Income sums the raw amount and expenses sum
// effectiveAmountSQL; opening balances have their own type, so they are never
// counted as income. When includeIncome is false only expenses are read. A
// group with no rate is left out and flags its bucket FXMissing.
func periodTotals(db *gorm.DB, fxs FXServicer, userID, base string, start, end time.Time, g periodGranularity, includeIncome bool) (map[string]periodTotal, error) {
	type row struct {
		Bucket   string
		At       time.Time
		Currency string
		Type     models.TransactionType
		Total    int64
	}

	dayExpr, dayGroup, isPostgres := dayExprs(db)
	q := db.Table("transactions t").
		Joins("JOIN accounts a ON a.id = t.account_id").
		Select(dayExpr+", a.currency AS currency, t.type AS type, "+
			"COALESCE(SUM(CASE WHEN t.type = ? THEN "+effectiveAmountSQL+" ELSE t.amount END), 0) AS total",
			models.TransactionTypeExpense).
		Where("t.user_id = ? AND t.deleted_at IS NULL AND t.date BETWEEN ? AND ?", userID, start, end)
	if includeIncome {
		q = q.Where("t.type IN ?",
			[]models.TransactionType{models.TransactionTypeExpense, models.TransactionTypeIncome})
	} else {
		q = q.Where("t.type = ?", models.TransactionTypeExpense)
	}

	var rows []row
	if err := q.Group(dayGroup + ", a.currency, t.type").Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	days := map[string]struct{}{}
	currencies := map[string]struct{}{}
	for _, r := range rows {
		days[dayKey(isPostgres, r.Bucket, r.At)] = struct{}{}
		currencies[normalizeCode(r.Currency)] = struct{}{}
	}
	var dv *datedValuer
	if first, last, ok := dayBounds(days); ok {
		var err error
		if dv, err = newDatedValuer(fxs, base, first, last, currencyList(currencies)); err != nil {
			return nil, err
		}
	}

	out := make(map[string]periodTotal)
	for _, r := range rows {
		day := dayKey(isPostgres, r.Bucket, r.At)
		t, err := time.Parse(dayLayout, day)
		if err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		key := t.Format(g.goLayout())
		pt := out[key]
		converted, ok := dv.Convert(r.Total, r.Currency, day)
		if !ok {
			pt.FXMissing = true
			out[key] = pt
			continue
		}
		switch r.Type {
		case models.TransactionTypeIncome:
			pt.Income += converted
		case models.TransactionTypeExpense:
			pt.Expenses += converted
		}
		out[key] = pt
	}
	return out, nil
}

// dailyRange normalizes from/to to UTC day boundaries and enforces the daily
// range cap. ok is false when to is before from (an empty result).
func dailyRange(from, to time.Time) (start, end time.Time, ok bool, err error) {
	start = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if last.Before(start) {
		return start, last, false, nil
	}
	if last.Sub(start) > maxDailyRangeDays*24*time.Hour {
		return start, last, false, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("date range cannot exceed %d days", maxDailyRangeDays))
	}
	return start, last, true, nil
}

// endOfDay returns the last representable instant of the UTC day starting at day.
func endOfDay(day time.Time) time.Time {
	return day.Add(24*time.Hour - time.Nanosecond)
}
