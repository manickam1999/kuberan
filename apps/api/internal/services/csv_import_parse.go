package services

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

const (
	// CSVImportMaxRows bounds the data rows of one CSV import.
	CSVImportMaxRows = 5000
	// csvMaxColumns bounds the columns of a CSV file.
	csvMaxColumns = 100
	// csvSampleRows is how many data rows the dry run echoes back.
	csvSampleRows = 5
)

// CSV amount sign conventions.
const (
	// CSVNegativeIsExpense treats negative amounts as money out (bank statements).
	CSVNegativeIsExpense = "negative_is_expense"
	// CSVPositiveIsExpense treats positive amounts as money out (card statements).
	CSVPositiveIsExpense = "positive_is_expense"
)

// csvDateFormats maps the friendly format names the API accepts to Go layouts.
// The layouts use the unpadded day/month so both "5/3/2024" and "05/03/2024"
// parse. Order matters for guessing: the unambiguous formats come first.
var csvDateFormats = []struct{ name, layout string }{
	{"YYYY-MM-DD", "2006-1-2"},
	{"YYYY/MM/DD", "2006/1/2"},
	{"YYYYMMDD", "20060102"},
	{"DD/MM/YYYY", "2/1/2006"},
	{"MM/DD/YYYY", "1/2/2006"},
	{"DD-MM-YYYY", "2-1-2006"},
	{"DD.MM.YYYY", "2.1.2006"},
	{"DD/MM/YY", "2/1/06"},
	{"MM/DD/YY", "1/2/06"},
	{"DD MMM YYYY", "2 Jan 2006"},
	{"DD-MMM-YYYY", "2-Jan-2006"},
	{"MMM DD, YYYY", "Jan 2, 2006"},
}

// CSVDateFormatNames lists the accepted date_format values.
func CSVDateFormatNames() []string {
	out := make([]string, len(csvDateFormats))
	for i, f := range csvDateFormats {
		out[i] = f.name
	}
	return out
}

func csvDateLayout(name string) (string, bool) {
	for _, f := range csvDateFormats {
		if strings.EqualFold(f.name, strings.TrimSpace(name)) {
			return f.layout, true
		}
	}
	return "", false
}

// csvFile is a parsed upload: the header and the data rows.
type csvFile struct {
	delimiter rune
	header    []string
	rows      [][]string
	lines     []int // 1-based file line of each data row
}

// parseCSVFile reads the upload. It strips a UTF-8 BOM, detects the delimiter
// (comma, semicolon, tab or pipe), uses the first non-empty record as the
// header, skips blank records and refuses files over CSVImportMaxRows.
func parseCSVFile(r io.Reader) (*csvFile, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "could not read the file")
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the file is not valid UTF-8; export it as UTF-8 CSV")
	}
	f := &csvFile{delimiter: detectDelimiter(raw)}
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.Comma = f.delimiter
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the file is not a valid CSV: "+err.Error())
		}
		if csvBlank(rec) {
			continue
		}
		line, _ := cr.FieldPos(0)
		if f.header == nil {
			if len(rec) > csvMaxColumns {
				return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("the file has more than %d columns", csvMaxColumns))
			}
			f.header = make([]string, len(rec))
			for i, h := range rec {
				f.header[i] = strings.TrimSpace(h)
			}
			continue
		}
		if len(f.rows) >= CSVImportMaxRows {
			return nil, apperrors.ErrCSVTooManyRows
		}
		f.rows = append(f.rows, rec)
		f.lines = append(f.lines, line)
	}
	if f.header == nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the file is empty")
	}
	if len(f.rows) == 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the file has a header but no data rows")
	}
	return f, nil
}

func csvBlank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// detectDelimiter picks the delimiter that appears most in the first lines.
func detectDelimiter(raw []byte) rune {
	head := raw
	if len(head) > 4096 {
		head = head[:4096]
	}
	best, bestN := ',', 0
	for _, d := range []rune{',', ';', '\t', '|'} {
		if n := strings.Count(string(head), string(d)); n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

// columnIndex returns the index of the column called name (exact match, then
// case-insensitive), or -1.
func (f *csvFile) columnIndex(name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for i, h := range f.header {
		if h == name {
			return i
		}
	}
	for i, h := range f.header {
		if strings.EqualFold(h, name) {
			return i
		}
	}
	return -1
}

func (f *csvFile) cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// headerHints are the header names each field is suggested from (lowercase).
var headerHints = struct {
	date, desc, amount, debit, credit, category, notes []string
}{
	date:     []string{"date", "transaction date", "txn date", "posting date", "posted date", "posted", "booking date", "value date", "trans date"},
	desc:     []string{"description", "details", "transaction details", "memo", "narrative", "payee", "merchant", "particulars", "transaction description", "name", "reference"},
	amount:   []string{"amount", "transaction amount", "value", "amt", "net amount"},
	debit:    []string{"debit", "debit amount", "withdrawal", "withdrawals", "money out", "paid out", "dr"},
	credit:   []string{"credit", "credit amount", "deposit", "deposits", "money in", "paid in", "cr"},
	category: []string{"category", "categories"},
	notes:    []string{"notes", "note", "remarks", "comment", "comments"},
}

func (f *csvFile) findHeader(hints []string) string {
	for _, hint := range hints {
		for _, h := range f.header {
			if strings.EqualFold(h, hint) {
				return h
			}
		}
	}
	for _, hint := range hints {
		for _, h := range f.header {
			if strings.Contains(strings.ToLower(h), hint) && len(hint) > 3 {
				return h
			}
		}
	}
	return ""
}

// suggestMapping guesses the mapping from the header names and the first rows.
// liability marks a credit-card or debt target, whose statements usually list
// purchases as positive amounts.
func (f *csvFile) suggestMapping(liability bool) CSVMapping {
	m := CSVMapping{
		DateColumn:        f.findHeader(headerHints.date),
		DescriptionColumn: f.findHeader(headerHints.desc),
		AmountColumn:      f.findHeader(headerHints.amount),
		DebitColumn:       f.findHeader(headerHints.debit),
		CreditColumn:      f.findHeader(headerHints.credit),
		CategoryColumn:    f.findHeader(headerHints.category),
		NotesColumn:       f.findHeader(headerHints.notes),
		AmountSign:        CSVNegativeIsExpense,
	}
	if m.AmountColumn != "" && m.DebitColumn != "" && m.CreditColumn != "" {
		// Both layouts present: separate debit/credit columns are the more explicit.
		m.AmountColumn = ""
	}
	if m.AmountColumn != "" {
		m.DebitColumn, m.CreditColumn = "", ""
	} else if m.DebitColumn == "" || m.CreditColumn == "" {
		m.DebitColumn, m.CreditColumn = "", ""
	}
	m.DateFormat = f.guessDateFormat(f.columnIndex(m.DateColumn))
	m.DecimalSeparator = f.guessDecimalSeparator(m)
	if liability && m.AmountColumn != "" && f.mostlyPositive(f.columnIndex(m.AmountColumn), m.DecimalSeparator) {
		m.AmountSign = CSVPositiveIsExpense
	}
	return m
}

// sampleCells returns up to 25 non-empty cells of a column.
func (f *csvFile) sampleCells(idx int) []string {
	var out []string
	for _, r := range f.rows {
		if c := f.cell(r, idx); c != "" {
			out = append(out, c)
			if len(out) == 25 {
				break
			}
		}
	}
	return out
}

// guessDateFormat returns the format that parses the most sample dates, the
// earlier one in the list on a tie, provided it parses at least half of them
// (a stray footer line must not defeat the guess). "" when none does.
func (f *csvFile) guessDateFormat(idx int) string {
	samples := f.sampleCells(idx)
	if idx < 0 || len(samples) == 0 {
		return ""
	}
	best, bestN := "", 0
	for _, df := range csvDateFormats {
		n := 0
		for _, s := range samples {
			if _, err := parseCSVDate(s, df.layout); err == nil {
				n++
			}
		}
		if n > bestN {
			best, bestN = df.name, n
		}
	}
	if bestN*2 < len(samples) {
		return ""
	}
	return best
}

var (
	csvCommaDecimal = regexp.MustCompile(`\d,\d{1,2}\s*[A-Za-z)]*$`)
	csvDotDecimal   = regexp.MustCompile(`\d\.\d{1,2}\s*[A-Za-z)]*$`)
)

// guessDecimalSeparator returns "," when the amount samples use a decimal comma.
func (f *csvFile) guessDecimalSeparator(m CSVMapping) string {
	comma, dot := 0, 0
	for _, col := range []string{m.AmountColumn, m.DebitColumn, m.CreditColumn} {
		for _, s := range f.sampleCells(f.columnIndex(col)) {
			switch {
			case csvCommaDecimal.MatchString(s):
				comma++
			case csvDotDecimal.MatchString(s):
				dot++
			}
		}
	}
	if comma > dot {
		return ","
	}
	return "."
}

func (f *csvFile) mostlyPositive(idx int, dec string) bool {
	pos, neg := 0, 0
	for _, s := range f.sampleCells(idx) {
		v, _, err := parseCSVAmount(s, dec)
		if err != nil || v == 0 {
			continue
		}
		if v > 0 {
			pos++
		} else {
			neg++
		}
	}
	return pos > neg
}

// parseCSVDate parses a date cell with layout. A time of day after the date is
// ignored ("2024-03-05 14:30:00", "2024-03-05T14:30:00Z").
func parseCSVDate(s, layout string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.Parse(layout, s)
	if err != nil && !strings.Contains(layout, " ") {
		if i := strings.IndexAny(s, " T"); i > 0 {
			t, err = time.Parse(layout, s[:i])
		}
	}
	if err != nil {
		return time.Time{}, errors.New("unreadable date")
	}
	t = normalizeCSVDate(t)
	if t.IsZero() {
		return time.Time{}, errors.New("date out of range")
	}
	return t, nil
}

// normalizeCSVDate pins a civil date at noon UTC, so every timezone reads the
// same calendar day.
func normalizeCSVDate(t time.Time) time.Time {
	y, m, d := t.Date()
	if y < 1970 || y > time.Now().Year()+1 {
		return time.Time{}
	}
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// parseCSVAmount parses a money cell into signed cents. dec is "." or ",": the
// other of the two is the thousands separator. Currency symbols and codes,
// spaces, parentheses (negative) and a trailing minus or CR/DR are accepted. A
// third decimal digit is refused unless it is zero. The string result is the
// trailing "CR" or "DR" marker in lower case, or "".
func parseCSVAmount(raw, dec string) (cents int64, marker string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, "", errors.New("empty amount")
	}
	neg := false
	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "CR"):
		marker = "cr"
		s = strings.TrimSpace(s[:len(s)-2])
	case strings.HasSuffix(upper, "DR"):
		marker = "dr"
		s = strings.TrimSpace(s[:len(s)-2])
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}
	var b strings.Builder
	seenDec := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
		case string(r) == dec:
			if seenDec {
				return 0, "", errors.New("unreadable amount")
			}
			seenDec = true
			b.WriteByte('.')
		case r == '-' || r == '−':
			neg = !neg
		case r == '+' || r == ' ' || r == ' ' || r == '\'' || r == ',' || r == '.':
			// thousands separators and signs
		case unicode.IsLetter(r) || unicode.Is(unicode.Sc, r):
			// currency code or symbol
		default:
			return 0, "", errors.New("unreadable amount")
		}
	}
	num := b.String()
	if num == "" || num == "." {
		return 0, "", errors.New("unreadable amount")
	}
	whole, frac, _ := strings.Cut(num, ".")
	frac = strings.TrimRight(frac, "0")
	if len(frac) > 2 {
		return 0, "", errors.New("more than two decimals")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if len(whole) > 13 {
		return 0, "", errors.New("amount too large")
	}
	var v int64
	for _, c := range whole + frac {
		v = v*10 + int64(c-'0')
	}
	if neg {
		v = -v
	}
	return v, marker, nil
}

// cleanCSVText turns control characters into spaces, collapses whitespace and
// cuts the text to limit characters.
func cleanCSVText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, limit)
}

// normalizeDescriptionKey reduces a description for fuzzy comparison: lower
// case, letters and digits only, single spaces.
func normalizeDescriptionKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// descriptionsMatch reports whether two normalised descriptions are the same
// or one contains the other (bank statements append references and dates).
func descriptionsMatch(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	if a == b {
		return true
	}
	if len(a) >= 4 && strings.Contains(b, a) {
		return true
	}
	return len(b) >= 4 && strings.Contains(a, b)
}

// csvTypeFor maps a signed amount to a transaction type and magnitude under
// the sign convention.
func csvTypeFor(cents int64, sign string) (typ models.TransactionType, magnitude int64) {
	expense := cents < 0
	if sign == CSVPositiveIsExpense {
		expense = cents > 0
	}
	if cents < 0 {
		cents = -cents
	}
	if expense {
		return models.TransactionTypeExpense, cents
	}
	return models.TransactionTypeIncome, cents
}
