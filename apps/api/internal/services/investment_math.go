package services

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
)

// quantityEpsilon is the tolerance under which two share quantities are the same
// quantity. Broker quantities carry at most about 8 decimal places, so any
// difference within 1e-9 is float64 rounding dust left by summing and splitting
// fractional lots, not a real difference in holdings. Comparisons between
// quantities use it so that a position the broker shows as closed is closed
// exactly, and a full sell is not rejected over a rounding hair.
const quantityEpsilon = 1e-9

// holdingState is the part of an investment that trades change: the position
// and its cost basis and realized P&L, in the security's currency and in the
// user's base currency (plan 024, phase 3a).
//
// apply is the single implementation of the per-transaction math. The
// incremental write paths (AddInvestment, RecordBuy, RecordSell, RecordSplit)
// and rebuildInvestment both call it, so a replay can never drift from what the
// incremental path stored.
type holdingState struct {
	Quantity             float64
	CostBasis            int64
	CostBasisBase        int64
	RealizedGainLoss     int64
	RealizedGainLossBase int64
}

// tradeEvent carries the inputs of one investment transaction to apply.
type tradeEvent struct {
	Type        models.InvestmentTransactionType
	Quantity    float64
	TotalAmount int64   // security currency; sells: net proceeds
	BaseAmount  int64   // TotalAmount in the user's base currency, frozen at the trade's rate
	SplitRatio  float64 // splits only
}

// tradeResult holds the realized figures a sell produces.
type tradeResult struct {
	RealizedGainLoss     int64
	RealizedGainLossBase int64
}

// stateOf returns the trade-driven state of a stored holding.
func stateOf(inv *models.Investment) holdingState {
	return holdingState{
		Quantity:             inv.Quantity,
		CostBasis:            inv.CostBasis,
		CostBasisBase:        inv.CostBasisBase,
		RealizedGainLoss:     inv.RealizedGainLoss,
		RealizedGainLossBase: inv.RealizedGainLossBase,
	}
}

// columns returns the investments columns this state owns, for an UPDATE.
func (h holdingState) columns() map[string]interface{} {
	return map[string]interface{}{
		"quantity":                h.Quantity,
		"cost_basis":              h.CostBasis,
		"cost_basis_base":         h.CostBasisBase,
		"realized_gain_loss":      h.RealizedGainLoss,
		"realized_gain_loss_base": h.RealizedGainLossBase,
	}
}

// proportionalCost returns the part of cost removed when quantity of a
// position is sold: round(cost × quantity / position). Selling the whole
// position (within quantityEpsilon) removes exactly the remaining cost. The same
// helper serves the native and the base cost so both round identically.
func proportionalCost(cost int64, quantity, position float64) int64 {
	if quantity >= position-quantityEpsilon {
		return cost
	}
	return int64(math.Round(float64(cost) * (quantity / position)))
}

// apply folds one transaction into the state. Buys add to the position and to
// both cost bases; sells remove a proportional share of both and realize the
// difference to the proceeds; splits multiply the quantity; dividends (and any
// other type) change nothing. A sell larger than the position (beyond
// quantityEpsilon) is rejected with ErrInsufficientShares and leaves the state
// untouched. A sell within quantityEpsilon of the position is a sell of the
// whole position: all cost is removed and the quantity becomes exactly 0.
func (h *holdingState) apply(e tradeEvent) (tradeResult, error) {
	switch e.Type {
	case models.InvestmentTransactionBuy:
		h.Quantity += e.Quantity
		h.CostBasis += e.TotalAmount
		h.CostBasisBase += e.BaseAmount
		h.normaliseQuantity()
	case models.InvestmentTransactionSell:
		if h.Quantity <= quantityEpsilon || e.Quantity > h.Quantity+quantityEpsilon {
			return tradeResult{}, apperrors.ErrInsufficientShares
		}
		removed, removedBase := h.CostBasis, h.CostBasisBase
		if math.Abs(e.Quantity-h.Quantity) <= quantityEpsilon {
			h.Quantity = 0
		} else {
			removed = proportionalCost(h.CostBasis, e.Quantity, h.Quantity)
			removedBase = proportionalCost(h.CostBasisBase, e.Quantity, h.Quantity)
			h.Quantity -= e.Quantity
		}
		res := tradeResult{
			RealizedGainLoss:     e.TotalAmount - removed,
			RealizedGainLossBase: e.BaseAmount - removedBase,
		}
		h.CostBasis -= removed
		h.CostBasisBase -= removedBase
		h.RealizedGainLoss += res.RealizedGainLoss
		h.RealizedGainLossBase += res.RealizedGainLossBase
		h.normaliseQuantity()
		return res, nil
	case models.InvestmentTransactionSplit:
		h.Quantity *= e.SplitRatio
		h.normaliseQuantity()
	}
	return tradeResult{}, nil
}

// normaliseQuantity snaps a quantity within quantityEpsilon of zero to exactly
// zero, so float dust never leaves a closed position looking open.
func (h *holdingState) normaliseQuantity() {
	if math.Abs(h.Quantity) <= quantityEpsilon {
		h.Quantity = 0
	}
}

// eventOf converts a stored row into the inputs of apply. Legacy rows without
// a frozen base amount were entered in the base currency, so their base amount
// is their total.
func eventOf(t *models.InvestmentTransaction) tradeEvent {
	base := t.TotalAmount
	if t.BaseAmount != nil {
		base = *t.BaseAmount
	}
	return tradeEvent{
		Type:        t.Type,
		Quantity:    t.Quantity,
		TotalAmount: t.TotalAmount,
		BaseAmount:  base,
		SplitRatio:  t.SplitRatio,
	}
}

// rebuildInvestment recomputes an investment's quantity, cost bases and realized
// P&L from zero by replaying its non-deleted transactions in (date, created_at,
// id) order, and writes the results to the holding and to every sell row. It
// uses holdingState.apply, the same math as the incremental write paths. A sell
// that exceeds the replayed position returns ErrInsufficientShares naming the
// transaction; the caller's DB transaction should roll back.
func rebuildInvestment(tx *gorm.DB, investmentID string) error {
	if _, err := lockInvestment(tx, investmentID); err != nil {
		return err
	}

	var rows []models.InvestmentTransaction
	if err := tx.Where("investment_id = ?", investmentID).
		Order("date ASC, created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var st holdingState
	for i := range rows {
		row := &rows[i]
		res, err := st.apply(eventOf(row))
		if err != nil {
			return apperrors.WithMessage(apperrors.ErrInsufficientShares,
				fmt.Sprintf("sell transaction %s exceeds the position held at that point", row.ID))
		}
		if row.Type != models.InvestmentTransactionSell {
			continue
		}
		if row.RealizedGainLoss == res.RealizedGainLoss && row.RealizedGainLossBase == res.RealizedGainLossBase {
			continue
		}
		if err := tx.Model(&models.InvestmentTransaction{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"realized_gain_loss":      res.RealizedGainLoss,
			"realized_gain_loss_base": res.RealizedGainLossBase,
		}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	if err := tx.Model(&models.Investment{}).Where("id = ?", investmentID).Updates(st.columns()).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// baseAmountFor converts a security-currency amount to base at rate.
func baseAmountFor(amount int64, rate fx.Rate) (int64, error) {
	out, err := fx.Convert(amount, rate)
	if err != nil {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "amount is too large to convert")
	}
	return out, nil
}

// identityRate is the rate 1.
const identityRate = fx.Rate(fx.RateScale)

// tradeCashLeg describes the cash movement that settles part of a trade; it
// feeds the trade FX rate precedence.
type tradeCashLeg struct {
	currency string // the cash account's currency
	amount   int64  // the amount moved, in that currency
	native   int64  // the part of the trade it settles, in the security's currency
}

// tradeRate resolves a trade's security-currency to base rate. Precedence:
//  1. an explicit rate;
//  2. when a cash leg is used and its currency is the base, the rate that leg
//     implies (what was actually paid beats a market rate);
//  3. 1 when the security is in the base currency;
//  4. the market rate as of the trade date (staleness allowed);
//  5. otherwise FX_RATE_UNAVAILABLE asking the caller to pass fx_rate.
func (s *investmentService) tradeRate(base, secCcy string, date time.Time, explicit *fx.Rate, leg *tradeCashLeg) (fx.Rate, error) {
	if explicit != nil {
		return *explicit, nil
	}
	if leg != nil && leg.currency == base && leg.amount > 0 && leg.native > 0 {
		r, err := fx.RateFromAmounts(leg.native, leg.amount)
		if err != nil {
			return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "the amounts imply an unusable exchange rate; pass fx_rate")
		}
		return r, nil
	}
	if secCcy == base {
		return identityRate, nil
	}
	if s.fxService != nil {
		q, err := s.fxService.Rate(secCcy, base, date)
		if err == nil {
			return q.Rate, nil
		}
		if !errors.Is(err, apperrors.ErrFXRateUnavailable) {
			return 0, err
		}
	}
	return 0, apperrors.WithMessage(apperrors.ErrFXRateUnavailable,
		fmt.Sprintf("No %s/%s exchange rate is available for %s; pass fx_rate", secCcy, base, date.UTC().Format("2006-01-02")))
}

// parseOptionalRate parses a request's fx_rate; "" means not given.
func parseOptionalRate(raw string) (*fx.Rate, error) {
	if raw == "" {
		return nil, nil
	}
	r, err := fx.ParseRate(raw)
	if err != nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "fx_rate must be a positive decimal string with at most 10 decimals")
	}
	return &r, nil
}
