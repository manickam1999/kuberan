package ibkr

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	// Embed the tz database so America/New_York resolves in minimal containers.
	_ "time/tzdata"
)

// NYLocation is the time zone IBKR prints trade and corporate action times in.
var NYLocation = mustLoadNY()

func mustLoadNY() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(fmt.Sprintf("ibkr: loading America/New_York: %v", err))
	}
	return loc
}

const (
	kindHeader   = "Header"
	kindData     = "Data"
	kindSubTotal = "SubTotal"
	kindTotal    = "Total"
)

// ignoredExact and ignoredPrefix list the recognised, deliberately unused sections.
var (
	ignoredExact = map[string]bool{
		"Net Asset Value":             true,
		"Change in NAV":               true,
		"Change in Dividend Accruals": true,
		"Codes":                       true,
		"Notes/Legal Notes":           true,
	}
	ignoredPrefix = []string{"Mark-to-Market", "Realized & Unrealized", "Total P/L"}
)

func isIgnoredSection(name string) bool {
	if ignoredExact[name] {
		return true
	}
	for _, p := range ignoredPrefix {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

var (
	splitRe      = regexp.MustCompile(`Split (\d+) for (\d+)`)
	periodLayout = "January 2, 2006"
)

// row is one CSV data row addressed by header name.
type row struct {
	section string
	line    int
	header  map[string]int
	fields  []string
}

func (r row) get(name string) string {
	i, ok := r.header[name]
	if !ok || i >= len(r.fields) {
		return ""
	}
	return strings.TrimSpace(r.fields[i])
}

func (r row) has(name string) bool {
	_, ok := r.header[name]
	return ok
}

func (r row) errf(format string, args ...any) error {
	return &ParseError{Line: r.line, Section: r.section, Msg: fmt.Sprintf(format, args...)}
}

// req returns a column that must exist in the header.
func (r row) req(names ...string) ([]string, error) {
	out := make([]string, len(names))
	for i, n := range names {
		if !r.has(n) {
			return nil, r.errf("missing column %q", n)
		}
		out[i] = r.get(n)
	}
	return out, nil
}

func (r row) isTotal(col string) bool {
	return strings.HasPrefix(r.get(col), kindTotal)
}

// Parse reads one Activity Statement CSV. It never touches the network or disk
// and does not log. The returned error is a *ParseError when it concerns a
// specific row.
func Parse(r io.Reader) (*Statement, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	st := &Statement{
		Positions:   map[string]Position{},
		Instruments: map[string]Instrument{},
	}
	headers := map[string]map[string]int{}
	seenIgnored := map[string]bool{}
	var periodRaw string
	first := true

	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ibkr: reading csv: %w", err)
		}
		line, _ := cr.FieldPos(0)
		if first {
			rec[0] = strings.TrimPrefix(rec[0], "\ufeff")
			first = false
		}
		if len(rec) < 2 {
			continue
		}
		section := strings.TrimSpace(rec[0])
		kind := strings.TrimSpace(rec[1])
		if section == "" {
			continue
		}

		if kind == kindHeader {
			h := make(map[string]int, len(rec))
			for i, name := range rec[2:] {
				name = strings.TrimSpace(name)
				if _, dup := h[name]; !dup && name != "" {
					h[name] = i + 2
				}
			}
			headers[section] = h // re-key on every Header row
			continue
		}
		if kind != kindData {
			// SubTotal / Total / free-form rows (e.g. "Total P/L ...") carry no new data.
			if isIgnoredSection(section) && !seenIgnored[section] {
				seenIgnored[section] = true
				st.IgnoredSections = append(st.IgnoredSections, section)
			}
			continue
		}

		rw := row{section: section, line: line, header: headers[section], fields: rec}
		if isIgnoredSection(section) {
			if !seenIgnored[section] {
				seenIgnored[section] = true
				st.IgnoredSections = append(st.IgnoredSections, section)
			}
			continue
		}
		if rw.header == nil {
			return nil, rw.errf("data row before any header row")
		}

		var perr error
		switch section {
		case "Statement":
			markIgnored(st, seenIgnored, section)
			if rw.get("Field Name") == "Period" {
				periodRaw = rw.get("Field Value")
			}
		case "Account Information":
			markIgnored(st, seenIgnored, section)
			switch rw.get("Field Name") {
			case "Account":
				st.AccountID = rw.get("Field Value")
			case "Base Currency":
				st.BaseCurrency = rw.get("Field Value")
			}
		case "Trades":
			perr = parseTrade(st, rw)
		case "Dividends":
			perr = parseCashRow(&st.Dividends, rw, "Date", false)
		case "Withholding Tax":
			perr = parseCashRow(&st.Withholdings, rw, "Date", false)
		case "Fees":
			perr = parseCashRow(&st.Fees, rw, "Date", true)
		case "Deposits & Withdrawals":
			perr = parseCashRow(&st.Deposits, rw, "Settle Date", false)
		case "Grant Activity":
			perr = parseGrant(st, rw)
		case "Corporate Actions":
			perr = parseCorporateAction(st, rw)
		case "Open Positions":
			perr = parsePosition(st, rw)
		case "Cash Report":
			perr = parseCashReport(st, rw)
		case "Financial Instrument Information":
			perr = parseInstrument(st, rw)
		default:
			return nil, rw.errf("unsupported section %q", section)
		}
		if perr != nil {
			return nil, perr
		}
	}

	if st.AccountID == "" {
		return nil, &ParseError{Section: "Account Information", Msg: "account id not found"}
	}
	if st.BaseCurrency == "" {
		return nil, &ParseError{Section: "Account Information", Msg: "base currency not found"}
	}
	if err := parsePeriod(st, periodRaw); err != nil {
		return nil, err
	}
	return st, nil
}

func markIgnored(st *Statement, seen map[string]bool, name string) {
	if !seen[name] {
		seen[name] = true
		st.IgnoredSections = append(st.IgnoredSections, name)
	}
}

func parsePeriod(st *Statement, raw string) error {
	if raw == "" {
		return &ParseError{Section: "Statement", Msg: "statement period not found"}
	}
	parts := strings.Split(raw, " - ")
	if len(parts) > 2 {
		return &ParseError{Section: "Statement", Msg: "unrecognised statement period"}
	}
	start, err := time.Parse(periodLayout, strings.TrimSpace(parts[0]))
	if err != nil {
		return &ParseError{Section: "Statement", Msg: "unrecognised statement period start"}
	}
	end := start
	if len(parts) == 2 {
		end, err = time.Parse(periodLayout, strings.TrimSpace(parts[1]))
		if err != nil {
			return &ParseError{Section: "Statement", Msg: "unrecognised statement period end"}
		}
	}
	if end.Before(start) {
		return &ParseError{Section: "Statement", Msg: "statement period ends before it starts"}
	}
	st.PeriodStart, st.PeriodEnd = start, end
	return nil
}

// symbolOf returns the text before "(" in an IBKR description.
func symbolOf(desc string) string {
	if i := strings.Index(desc, "("); i >= 0 {
		desc = desc[:i]
	}
	return strings.TrimSpace(desc)
}

func parseDateTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02, 15:04:05", "2006-01-02 15:04:05", "2006-01-02;15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, NYLocation); err == nil {
			return t.UTC(), nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, NYLocation); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid date/time %q", s)
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q", s)
	}
	return t, nil
}

func parseTrade(st *Statement, r row) error {
	vals, err := r.req("DataDiscriminator", "Asset Category", "Currency", "Symbol", "Date/Time", "Quantity", "T. Price", "Proceeds", "Comm/Fee")
	if err != nil {
		return err
	}
	switch vals[0] {
	case "Order":
	case "ClosedLot", "Lot":
		return nil
	default:
		return r.errf("unsupported trade row type %q", vals[0])
	}
	if vals[1] != "Stocks" {
		return r.errf("unsupported asset category %q", vals[1])
	}
	t, err := parseDateTime(vals[4])
	if err != nil {
		return r.errf("%v", err)
	}
	qty, err := parseQty(vals[5])
	if err != nil {
		return r.errf("%v", err)
	}
	price, err := parseRat(vals[6])
	if err != nil {
		return r.errf("T. Price: %v", err)
	}
	proceeds, err := parseRat(vals[7])
	if err != nil {
		return r.errf("Proceeds: %v", err)
	}
	fee, err := parseRat(vals[8])
	if err != nil {
		return r.errf("Comm/Fee: %v", err)
	}
	if qty == 0 || vals[3] == "" {
		return r.errf("trade without symbol or quantity")
	}
	st.Trades = append(st.Trades, TradeRow{Line: r.line, Symbol: vals[3], Currency: vals[2], Time: t,
		Quantity: qty, Price: price, Proceeds: proceeds, Fee: fee})
	return nil
}

// parseCashRow handles Dividends, Withholding Tax, Fees and Deposits & Withdrawals.
func parseCashRow(dst *[]CashRow, r row, dateCol string, hasSubtitle bool) error {
	vals, err := r.req("Currency", dateCol, "Description", "Amount")
	if err != nil {
		return err
	}
	if r.isTotal("Currency") || r.isTotal("Subtitle") {
		return nil
	}
	d, err := parseDate(vals[1])
	if err != nil {
		return r.errf("%v", err)
	}
	amt, err := parseRat(vals[3])
	if err != nil {
		return r.errf("Amount: %v", err)
	}
	cr := CashRow{Line: r.line, Currency: vals[0], Date: d, Description: vals[2], Amount: amt}
	if r.section != "Deposits & Withdrawals" {
		cr.Symbol = symbolOf(vals[2])
		if cr.Symbol == "" {
			return r.errf("cannot determine symbol from description")
		}
	}
	if hasSubtitle {
		cr.Subtitle = r.get("Subtitle")
	}
	*dst = append(*dst, cr)
	return nil
}

func parseGrant(st *Statement, r row) error {
	vals, err := r.req("Symbol", "Report Date", "Description", "Quantity", "Price", "Value")
	if err != nil {
		return err
	}
	if r.isTotal("Symbol") {
		return nil
	}
	d, err := parseDate(vals[1])
	if err != nil {
		return r.errf("%v", err)
	}
	qty, err := parseQty(vals[3])
	if err != nil {
		return r.errf("%v", err)
	}
	price, err := parseRat(vals[4])
	if err != nil {
		return r.errf("Price: %v", err)
	}
	value, err := parseRat(vals[5])
	if err != nil {
		return r.errf("Value: %v", err)
	}
	st.Grants = append(st.Grants, GrantRow{Line: r.line, Symbol: vals[0], ReportDate: d,
		Description: vals[2], Quantity: qty, Price: price, Value: value})
	return nil
}

func parseCorporateAction(st *Statement, r row) error {
	vals, err := r.req("Asset Category", "Currency", "Date/Time", "Description")
	if err != nil {
		return err
	}
	if r.isTotal("Asset Category") {
		return nil
	}
	m := splitRe.FindStringSubmatch(vals[3])
	if m == nil {
		return r.errf("unsupported corporate action %q", vals[3])
	}
	num, _ := strconv.Atoi(m[1])
	den, _ := strconv.Atoi(m[2])
	if num <= 0 || den <= 0 {
		return r.errf("invalid split ratio in %q", vals[3])
	}
	t, err := parseDateTime(vals[2])
	if err != nil {
		return r.errf("%v", err)
	}
	st.CorporateActions = append(st.CorporateActions, SplitRow{Line: r.line, Symbol: symbolOf(vals[3]),
		Currency: vals[1], Time: t, Description: vals[3], Numerator: num, Denominator: den})
	return nil
}

func parsePosition(st *Statement, r row) error {
	vals, err := r.req("DataDiscriminator", "Asset Category", "Currency", "Symbol", "Quantity", "Cost Basis")
	if err != nil {
		return err
	}
	switch vals[0] {
	case "Summary":
	case "Lot":
		return nil
	default:
		return r.errf("unsupported open position row type %q", vals[0])
	}
	if vals[1] != "Stocks" {
		return r.errf("unsupported asset category %q", vals[1])
	}
	qty, err := parseQty(vals[4])
	if err != nil {
		return r.errf("%v", err)
	}
	cb, err := parseRat(vals[5])
	if err != nil {
		return r.errf("Cost Basis: %v", err)
	}
	cbc, err := cents(cb)
	if err != nil {
		return r.errf("Cost Basis: %v", err)
	}
	st.Positions[vals[3]] = Position{Line: r.line, Currency: vals[2], Quantity: qty, CostBasisCents: cbc}
	return nil
}

func parseCashReport(st *Statement, r row) error {
	vals, err := r.req("Currency Summary", "Currency", "Total")
	if err != nil {
		return err
	}
	if vals[0] != "Ending Cash" || vals[1] != "Base Currency Summary" {
		return nil
	}
	v, err := parseRat(vals[2])
	if err != nil {
		return r.errf("Total: %v", err)
	}
	c, err := cents(v)
	if err != nil {
		return r.errf("Total: %v", err)
	}
	st.EndingCashCents, st.HasEndingCash = c, true
	return nil
}

func parseInstrument(st *Statement, r row) error {
	vals, err := r.req("Symbol", "Listing Exch")
	if err != nil {
		return err
	}
	if vals[0] == "" {
		return nil
	}
	st.Instruments[vals[0]] = Instrument{Symbol: vals[0], Description: r.get("Description"),
		ISIN: r.get("Security ID"), ListingExchange: vals[1], Type: r.get("Type")}
	return nil
}
