package services

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// daysInCurve is the length of the typical cumulative spending curve (the
// longest month).
const daysInCurve = 31

// Typical is what a "normal" month looks like for a user, averaged over the
// complete months before the current one (plan 023, 5.4). Every amount is in
// the user's base currency (cents).
type Typical struct {
	// Months is the number of months asked for.
	Months int `json:"months"`
	// MonthsUsed is how many of them the averages cover: the months from the
	// user's first income or expense in the window, so a new user is not
	// averaged against empty months. 0 means there is no history yet and
	// every figure is zero.
	MonthsUsed int    `json:"months_used"`
	Currency   string `json:"currency"`
	// AvgIncome and AvgSpending are per-month averages.
	AvgIncome   int64 `json:"avg_income"`
	AvgSpending int64 `json:"avg_spending"`
	// SavedPercent is (income - spending) / income over the window, in
	// percent with one decimal; 0 when there was no income.
	SavedPercent float64 `json:"saved_percent"`
	// DailyCumulative is the average cumulative spending after day 1..31 of a
	// month (always 31 points; a shorter month holds its total after its last
	// day).
	DailyCumulative []int64 `json:"daily_cumulative"`
	// ByCategory is the average monthly spending per category, largest first.
	ByCategory []SpendingByCategoryItem `json:"by_category"`
	// FXMissing is true when some amounts had no exchange rate and were left out.
	FXMissing   bool                `json:"fx_missing"`
	Unconverted []UnconvertedAmount `json:"unconverted"`
}

// TypicalServicer computes the "typical month" figures.
type TypicalServicer interface {
	// GetTypical averages the last `months` (1..24) complete months.
	GetTypical(userID string, months int) (*Typical, error)
}

type typicalService struct {
	db  *gorm.DB
	fxs FXServicer
}

// NewTypicalService creates the TypicalServicer. fxs converts amounts into the
// user's base currency; nil only converts same-currency amounts.
func NewTypicalService(db *gorm.DB, fxs FXServicer) TypicalServicer {
	return &typicalService{db: db, fxs: fxs}
}

// GetTypical implements TypicalServicer. The window is the `months` complete
// UTC months before the current one, shortened to start at the month of the
// user's first income or expense inside it. Totals come from periodTotals (so
// opening balances are excluded and every group converts at its own day's
// rate); categories come from totalsByCategory.
func (s *typicalService) GetTypical(userID string, months int) (*Typical, error) {
	if months < minSummaryMonths || months > maxSummaryMonths {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("months must be between %d and %d", minSummaryMonths, maxSummaryMonths))
	}
	base, err := userBaseCurrency(s.db, userID)
	if err != nil {
		return nil, err
	}
	out := &Typical{
		Months:          months,
		Currency:        base,
		DailyCumulative: make([]int64, daysInCurve),
		ByCategory:      []SpendingByCategoryItem{},
		Unconverted:     []UnconvertedAmount{},
	}

	now := time.Now().UTC()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	windowStart := thisMonth.AddDate(0, -months, 0)
	windowEnd := thisMonth.Add(-time.Nanosecond)

	var first []time.Time
	if err := s.db.Table("transactions").
		Where("user_id = ? AND deleted_at IS NULL AND type IN ? AND date >= ? AND date <= ?",
			userID, []models.TransactionType{models.TransactionTypeExpense, models.TransactionTypeIncome}, windowStart, windowEnd).
		Limit(1).Order("date ASC").Pluck("date", &first).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(first) == 0 {
		return out, nil
	}
	start := time.Date(first[0].UTC().Year(), first[0].UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	used := 0
	for m := start; m.Before(thisMonth); m = m.AddDate(0, 1, 0) {
		used++
	}
	out.MonthsUsed = used

	monthly, err := periodTotals(s.db, s.fxs, userID, base, start, windowEnd, periodMonth, true)
	if err != nil {
		return nil, err
	}
	daily, err := periodTotals(s.db, s.fxs, userID, base, start, windowEnd, periodDay, false)
	if err != nil {
		return nil, err
	}
	var income, spending int64
	for _, t := range monthly {
		income += t.Income
		spending += t.Expenses
		out.FXMissing = out.FXMissing || t.FXMissing
	}
	out.AvgIncome = divRound(income, used)
	out.AvgSpending = divRound(spending, used)
	if income > 0 {
		out.SavedPercent = math.Round(float64(income-spending)/float64(income)*1000) / 10
	}

	sums := make([]int64, daysInCurve)
	for m := start; m.Before(thisMonth); m = m.AddDate(0, 1, 0) {
		length := m.AddDate(0, 1, -1).Day()
		var running int64
		for d := 1; d <= daysInCurve; d++ {
			if d <= length {
				running += daily[m.AddDate(0, 0, d-1).Format(dayLayout)].Expenses
			}
			sums[d-1] += running
		}
	}
	for i, v := range sums {
		out.DailyCumulative[i] = divRound(v, used)
	}

	cats, err := (&transactionService{db: s.db, fxService: s.fxs}).totalsByCategory(userID, models.TransactionTypeExpense, start, windowEnd)
	if err != nil {
		return nil, err
	}
	out.FXMissing = out.FXMissing || cats.FXMissing
	out.Unconverted = cats.Unconverted
	for _, item := range cats.Items {
		item.Total = divRound(item.Total, used)
		if item.Total > 0 {
			out.ByCategory = append(out.ByCategory, item)
		}
	}
	return out, nil
}

// divRound divides by n rounding half away from zero (n > 0).
func divRound(v int64, n int) int64 {
	d := int64(n)
	if v >= 0 {
		return (v + d/2) / d
	}
	return -((-v + d/2) / d)
}
