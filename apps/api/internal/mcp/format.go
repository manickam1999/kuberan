package mcp

import (
	"fmt"
	"math"
)

// formatCents converts int64 cents to a dollar string like "$1234.56".
func formatCents(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	dollars := cents / 100
	remainder := cents % 100
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s$%d.%02d", sign, dollars, remainder)
}

// dollarsToCents converts a float64 dollar amount to int64 cents.
func dollarsToCents(dollars float64) int64 {
	return int64(math.Round(dollars * 100))
}
