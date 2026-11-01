package services

import (
	"fmt"
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

// pgFormat is the Postgres to_char format matching goLayout.
func (g periodGranularity) pgFormat() string {
	if g == periodMonth {
		return "YYYY-MM"
	}
	return "YYYY-MM-DD"
}

// periodTotal holds the income and expense totals of one bucket.
type periodTotal struct {
	Income   int64
	Expenses int64
}

// periodTotals sums a user's income and effective expenses between start and
// end (inclusive) in one grouped query and returns them keyed by UTC bucket
// (goLayout format). Semantics match the per-bucket queries it replaces:
// income excludes "Initial balance" rows and sums the raw amount, expenses sum
// effectiveAmountSQL. When includeIncome is false only expenses are read.
//
// On Postgres the bucketing happens in SQL; on other dialects (SQLite in tests)
// rows are grouped by their exact timestamp and bucketed in Go.
func periodTotals(db *gorm.DB, userID string, start, end time.Time, g periodGranularity, includeIncome bool) (map[string]periodTotal, error) {
	type row struct {
		Bucket string
		At     time.Time
		Type   models.TransactionType
		Total  int64
	}

	isPostgres := db.Name() == "postgres"
	bucketExpr := "date AS at"
	if isPostgres {
		bucketExpr = fmt.Sprintf("to_char(date AT TIME ZONE 'UTC', '%s') AS bucket", g.pgFormat())
	}
	groupBy := "date, type"
	if isPostgres {
		groupBy = "bucket, type"
	}

	q := db.Model(&models.Transaction{}).
		Select(bucketExpr+", type, COALESCE(SUM(CASE WHEN type = ? THEN "+effectiveAmountSQL+" ELSE amount END), 0) AS total",
			models.TransactionTypeExpense).
		Where("user_id = ? AND deleted_at IS NULL AND date BETWEEN ? AND ?", userID, start, end)
	if includeIncome {
		q = q.Where("(type = ? OR (type = ? AND description != ?))",
			models.TransactionTypeExpense, models.TransactionTypeIncome, "Initial balance")
	} else {
		q = q.Where("type = ?", models.TransactionTypeExpense)
	}

	var rows []row
	if err := q.Group(groupBy).Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	out := make(map[string]periodTotal)
	for _, r := range rows {
		key := r.Bucket
		if !isPostgres {
			key = r.At.UTC().Format(g.goLayout())
		}
		t := out[key]
		switch r.Type {
		case models.TransactionTypeIncome:
			t.Income += r.Total
		case models.TransactionTypeExpense:
			t.Expenses += r.Total
		}
		out[key] = t
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
