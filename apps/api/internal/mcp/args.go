package mcp

import (
	"fmt"
	"math"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/pagination"
)

// Bounds on read-tool arguments. Tool arguments come from a language model
// (and, through it, from whatever text it has read), so every numeric or date
// argument is bounded before it reaches the service layer.
const (
	// minArgYear and maxArgYear bound any date a tool accepts.
	minArgYear = 1970
	maxArgYear = 2100

	// maxDailyRangeDays caps tools that return one row per day.
	maxDailyRangeDays = 366
	// maxAggregateRangeDays caps tools that aggregate a range into a bounded
	// number of rows (by category, top N, grouped snapshots).
	maxAggregateRangeDays = 10 * 366

	// minMonths and maxMonths bound get_monthly_summary's months argument.
	minMonths     = 1
	maxMonths     = 24
	defaultMonths = 6

	// maxPageSize caps page_size and limit arguments.
	maxPageSize = 100
	// maxPage caps the page argument; with maxPageSize it reaches 100k rows.
	maxPage = 1000

	// maxFilterDollars bounds amount filters so the conversion to int64 cents
	// cannot overflow.
	maxFilterDollars = 1e12

	// maxFilterTextLen caps free-text filter arguments.
	maxFilterTextLen = 255
)

// parseArgDate parses a YYYY-MM-DD tool argument within minArgYear..maxArgYear.
func parseArgDate(name, v string) (time.Time, *mcp.CallToolResult) {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, mcp.NewToolResultError(fmt.Sprintf("invalid %s format, expected YYYY-MM-DD", name))
	}
	if t.Year() < minArgYear || t.Year() > maxArgYear {
		return time.Time{}, mcp.NewToolResultError(fmt.Sprintf("%s must be between %d and %d", name, minArgYear, maxArgYear))
	}
	return t, nil
}

// optionalArgDate parses an optional date argument; it returns nil when absent.
func optionalArgDate(args map[string]interface{}, name string) (*time.Time, *mcp.CallToolResult) {
	v, _ := args[name].(string)
	if v == "" {
		return nil, nil
	}
	t, res := parseArgDate(name, v)
	if res != nil {
		return nil, res
	}
	return &t, nil
}

// dateRangeArgs reads the required from_date and to_date arguments. The range
// must be ordered and span at most maxDays days (inclusive).
func dateRangeArgs(args map[string]interface{}, maxDays int) (from, to time.Time, res *mcp.CallToolResult) {
	fromStr, _ := args["from_date"].(string)
	toStr, _ := args["to_date"].(string)
	if from, res = parseArgDate("from_date", fromStr); res != nil {
		return
	}
	if to, res = parseArgDate("to_date", toStr); res != nil {
		return
	}
	if res = checkRange(from, to, maxDays); res != nil {
		return
	}
	return from, to, nil
}

// checkRange rejects a range whose end precedes its start or that spans more
// than maxDays days (inclusive). maxDays <= 0 checks only the order.
func checkRange(from, to time.Time, maxDays int) *mcp.CallToolResult {
	if to.Before(from) {
		return mcp.NewToolResultError("to_date must not be before from_date")
	}
	if maxDays > 0 && int(to.Sub(from).Hours()/24)+1 > maxDays {
		return mcp.NewToolResultError(fmt.Sprintf(
			"date range is too long: at most %d days; split it into smaller ranges", maxDays))
	}
	return nil
}

// monthsArg reads get_monthly_summary's months argument, clamped to
// minMonths..maxMonths.
func monthsArg(args map[string]interface{}) int {
	v, ok := args["months"].(float64)
	if !ok || math.IsNaN(v) {
		return defaultMonths
	}
	return int(math.Max(minMonths, math.Min(maxMonths, math.Floor(v))))
}

// limitArg reads a positive integer argument with a default, clamped to
// 1..maxPageSize.
func limitArg(args map[string]interface{}, name string, def int) int {
	v, ok := args[name].(float64)
	if !ok || math.IsNaN(v) || v < 1 {
		return def
	}
	return int(math.Min(maxPageSize, math.Floor(v)))
}

// parsePage reads the optional page (default 1, at most maxPage) and
// page_size (default 20, at most maxPageSize) tool arguments.
func parsePage(args map[string]interface{}) pagination.PageRequest {
	page := pagination.PageRequest{Page: 1, PageSize: limitArg(args, "page_size", 20)}
	if v, ok := args["page"].(float64); ok && !math.IsNaN(v) && v >= 1 {
		page.Page = int(math.Min(maxPage, math.Floor(v)))
	}
	return page
}

// amountFilterArg reads an optional dollar amount filter as cents. It returns
// nil when absent.
func amountFilterArg(args map[string]interface{}, name string) (*int64, *mcp.CallToolResult) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, nil
	}
	v, ok := raw.(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > maxFilterDollars {
		return nil, mcp.NewToolResultError(name + " must be a number of dollars")
	}
	cents := dollarsToCents(v)
	return &cents, nil
}

// filterTextArg reads an optional free-text filter, rejecting one longer than
// maxFilterTextLen bytes.
func filterTextArg(args map[string]interface{}, name string) (string, *mcp.CallToolResult) {
	v, _ := args[name].(string)
	if len(v) > maxFilterTextLen {
		return "", mcp.NewToolResultError(fmt.Sprintf("%s is too long (max %d characters)", name, maxFilterTextLen))
	}
	return v, nil
}
