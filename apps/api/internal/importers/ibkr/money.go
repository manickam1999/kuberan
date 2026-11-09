package ibkr

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// parseRat parses an exact decimal such as "-0.35691725". An empty string is
// zero. Thousands separators are tolerated.
func parseRat(s string) (*big.Rat, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return new(big.Rat), nil
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid number %q", s)
	}
	return r, nil
}

// parseQty parses a quantity as float64 rounded to 10 decimal places.
func parseQty(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid quantity %q", s)
	}
	return roundQty(f), nil
}

// roundQty rounds a quantity to 10 decimal places.
func roundQty(f float64) float64 {
	return math.Round(f*1e10) / 1e10
}

var (
	ratHundred = big.NewRat(100, 1)
	bigTwo     = big.NewInt(2)
)

// cents converts an exact decimal amount to int64 cents with a single
// round-half-away-from-zero.
func cents(r *big.Rat) (int64, error) {
	x := new(big.Rat).Mul(r, ratHundred)
	neg := x.Sign() < 0
	if neg {
		x.Neg(x)
	}
	// floor((2n + d) / 2d) == round half up for non-negative n/d.
	num := new(big.Int).Mul(x.Num(), bigTwo)
	num.Add(num, x.Denom())
	den := new(big.Int).Mul(x.Denom(), bigTwo)
	q := new(big.Int).Quo(num, den)
	if !q.IsInt64() {
		return 0, fmt.Errorf("amount out of range")
	}
	v := q.Int64()
	if neg {
		v = -v
	}
	return v, nil
}

// absRat returns |r| as a new value.
func absRat(r *big.Rat) *big.Rat {
	return new(big.Rat).Abs(r)
}
