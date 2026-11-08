// Package fx implements exact fixed-point foreign-exchange rate math. It is
// pure (no database, no floats in the arithmetic): a Rate is an int64 scaled by
// 1e10, and money (int64 minor units) is converted with math/big using round
// half to even.
package fx

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// RateScale is the fixed-point scale of a Rate: 1.0 == RateScale.
const RateScale int64 = 10_000_000_000

// rateDecimals is the number of fractional digits a Rate carries.
const rateDecimals = 10

// MaxRate is the largest rate that fits NUMERIC(20,10): 99999999.9999999999.
const MaxRate Rate = 99_999_999*Rate(RateScale) + (Rate(RateScale) - 1)

// Rate is an exchange rate as a fixed-point integer scaled by RateScale, so
// 4.4712 is 44712000000.
type Rate int64

// Errors returned by the package.
var (
	// ErrInvalidRate is returned when a rate string or float cannot be used as a Rate.
	ErrInvalidRate = errors.New("fx: invalid rate")
	// ErrOverflow is returned when a computation does not fit in an int64.
	ErrOverflow = errors.New("fx: arithmetic overflow")
)

var (
	bigScale = big.NewInt(RateScale)
	bigTwo   = big.NewInt(2)
)

// ParseRate parses an exact decimal such as "4", "4.4712" or "0.2236". It
// accepts at most 10 fractional digits, requires a value greater than zero and
// at most 99999999.9999999999. Signs, exponents, spaces and a missing integer
// part are rejected.
func ParseRate(s string) (Rate, error) {
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidRate)
	}
	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if !allDigits(intPart) || (hasDot && !allDigits(fracPart)) {
		return 0, fmt.Errorf("%w: %q is not a plain decimal", ErrInvalidRate, s)
	}
	if len(fracPart) > rateDecimals {
		return 0, fmt.Errorf("%w: %q has more than %d fractional digits", ErrInvalidRate, s, rateDecimals)
	}
	intPart = strings.TrimLeft(intPart, "0")
	if len(intPart) > 8 {
		return 0, fmt.Errorf("%w: %q is too large", ErrInvalidRate, s)
	}
	fracPart += strings.Repeat("0", rateDecimals-len(fracPart))

	// intPart has at most 8 digits and fracPart exactly 10, so 18 digits fit in an int64.
	digits := intPart + fracPart
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %w", ErrInvalidRate, s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%w: %q must be greater than zero", ErrInvalidRate, s)
	}
	return Rate(v), nil
}

// allDigits reports whether s is non-empty and made only of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// String returns the canonical decimal form with trailing zeros trimmed
// ("4.4712", "4").
func (r Rate) String() string {
	v := int64(r)
	sign := ""
	// Work in uint64 so math.MinInt64 does not overflow on negation.
	u := uint64(v)
	if v < 0 {
		sign = "-"
		u = -u
	}
	scale := uint64(RateScale)
	whole := u / scale
	frac := u % scale
	if frac == 0 {
		return sign + strconv.FormatUint(whole, 10)
	}
	fs := strconv.FormatUint(frac, 10)
	fs = strings.Repeat("0", rateDecimals-len(fs)) + fs
	return sign + strconv.FormatUint(whole, 10) + "." + strings.TrimRight(fs, "0")
}

// RateFromFloat converts a provider float to a Rate, rounding to 10 decimal
// places. It rejects NaN, Inf and values that are not positive after rounding.
// It is meant for provider input and tests, never for money math.
func RateFromFloat(f float64) (Rate, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%w: %v is not finite", ErrInvalidRate, f)
	}
	if f <= 0 {
		return 0, fmt.Errorf("%w: %v must be greater than zero", ErrInvalidRate, f)
	}
	return ParseRate(strconv.FormatFloat(f, 'f', rateDecimals, 64))
}

// Convert returns amount × r, rounded half to even, where amount is in minor
// units and r converts between the two currencies. Negative amounts round
// symmetrically. It errors when the result does not fit in an int64.
func Convert(amount int64, r Rate) (int64, error) {
	num := new(big.Int).Mul(big.NewInt(amount), big.NewInt(int64(r)))
	q := divRoundHalfEven(num, bigScale)
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: %d × %s", ErrOverflow, amount, r)
	}
	return q.Int64(), nil
}

// Invert returns 1/r rounded half to even to 10 decimal places. It errors when
// r is not positive, or the result is zero or does not fit in an int64.
func Invert(r Rate) (Rate, error) {
	if r <= 0 {
		return 0, fmt.Errorf("%w: cannot invert %s", ErrInvalidRate, r)
	}
	num := new(big.Int).Mul(bigScale, bigScale)
	q := divRoundHalfEven(num, big.NewInt(int64(r)))
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: 1/%s", ErrOverflow, r)
	}
	if q.Sign() == 0 {
		return 0, fmt.Errorf("%w: 1/%s rounds to zero", ErrInvalidRate, r)
	}
	return Rate(q.Int64()), nil
}

// Mul returns a×b rounded half to even, for chaining rates through a pivot
// currency. It errors when either rate is not positive, or the product is zero
// or does not fit in an int64.
func Mul(a, b Rate) (Rate, error) {
	if a <= 0 || b <= 0 {
		return 0, fmt.Errorf("%w: cannot multiply %s and %s", ErrInvalidRate, a, b)
	}
	num := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
	q := divRoundHalfEven(num, bigScale)
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: %s × %s", ErrOverflow, a, b)
	}
	if q.Sign() == 0 {
		return 0, fmt.Errorf("%w: %s × %s rounds to zero", ErrInvalidRate, a, b)
	}
	return Rate(q.Int64()), nil
}

// RateFromAmounts returns the rate implied by exchanging from units of one
// currency for to units of another, i.e. to/from rounded half to even to 10
// decimal places. Both amounts must be positive; the result must be non-zero
// and fit a Rate.
func RateFromAmounts(from, to int64) (Rate, error) {
	if from <= 0 || to <= 0 {
		return 0, fmt.Errorf("%w: amounts must be positive (%d, %d)", ErrInvalidRate, from, to)
	}
	num := new(big.Int).Mul(big.NewInt(to), bigScale)
	q := divRoundHalfEven(num, big.NewInt(from))
	if !q.IsInt64() || q.Int64() > int64(MaxRate) {
		return 0, fmt.Errorf("%w: %d/%d", ErrOverflow, to, from)
	}
	if q.Sign() == 0 {
		return 0, fmt.Errorf("%w: %d/%d rounds to zero", ErrInvalidRate, to, from)
	}
	return Rate(q.Int64()), nil
}

// divRoundHalfEven returns num/den (den > 0) rounded to the nearest integer,
// with exact ties going to the even neighbour.
func divRoundHalfEven(num, den *big.Int) *big.Int {
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() == 0 {
		return q
	}
	twice := new(big.Int).Mul(new(big.Int).Abs(rem), bigTwo)
	switch twice.Cmp(den) {
	case 1:
		q.Add(q, big.NewInt(int64(num.Sign())))
	case 0:
		if q.Bit(0) == 1 {
			q.Add(q, big.NewInt(int64(num.Sign())))
		}
	}
	return q
}

// Value implements driver.Valuer, writing the canonical decimal string so the
// database NUMERIC column receives the exact value.
func (r Rate) Value() (driver.Value, error) {
	return r.String(), nil
}

// Scan implements sql.Scanner. It accepts a decimal string or []byte
// (Postgres NUMERIC), a float64 (SQLite REAL, rounded to 10 places) and an
// int64 (SQLite INTEGER holding a whole-number rate such as 4).
func (r *Rate) Scan(src any) error {
	var (
		v   Rate
		err error
	)
	switch x := src.(type) {
	case string:
		v, err = ParseRate(x)
	case []byte:
		v, err = ParseRate(string(x))
	case float64:
		v, err = RateFromFloat(x)
	case int64:
		if x <= 0 || x > int64(MaxRate/Rate(RateScale)) {
			return fmt.Errorf("%w: %d", ErrInvalidRate, x)
		}
		v = Rate(x * RateScale)
	default:
		return fmt.Errorf("fx: cannot scan %T into Rate", src)
	}
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// MarshalJSON writes the rate as a JSON string ("4.4712"), never a number, so
// clients cannot lose precision.
func (r Rate) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON reads a rate from a JSON decimal string.
func (r *Rate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: rate must be a decimal string: %w", ErrInvalidRate, err)
	}
	v, err := ParseRate(s)
	if err != nil {
		return err
	}
	*r = v
	return nil
}
