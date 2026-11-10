package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
)

const (
	// csvReportRows bounds the per-row results in the report.
	csvReportRows = 200
	// csvReportIssues bounds the listed row problems.
	csvReportIssues = 100
	// csvInsertBatch is the insert batch size on apply.
	csvInsertBatch = 200
	// csvFuzzyDays is how far a manual entry's date may be from a statement row.
	csvFuzzyDays = 1
)

// Row outcomes in the report.
const (
	// CSVRowCreate is a row the import creates.
	CSVRowCreate = "create"
	// CSVRowSkipDuplicate is a row already imported earlier (same import_ref).
	CSVRowSkipDuplicate = "skip_duplicate"
	// CSVRowSkipManual is a row matching an entry the user typed by hand.
	CSVRowSkipManual = "skip_manual"
	// CSVRowInvalid is a row that could not be read.
	CSVRowInvalid = "invalid"
)

// CSVMapping says which column holds what. Columns are header names. Give
// either AmountColumn (signed) or both DebitColumn and CreditColumn.
type CSVMapping struct {
	DateColumn        string `json:"date_column"`
	DescriptionColumn string `json:"description_column"`
	AmountColumn      string `json:"amount_column,omitempty"`
	DebitColumn       string `json:"debit_column,omitempty"`
	CreditColumn      string `json:"credit_column,omitempty"`
	CategoryColumn    string `json:"category_column,omitempty"`
	NotesColumn       string `json:"notes_column,omitempty"`
	// DateFormat is one of CSVDateFormatNames, e.g. "DD/MM/YYYY".
	DateFormat string `json:"date_format"`
	// DecimalSeparator is "." or ",".
	DecimalSeparator string `json:"decimal_separator"`
	// AmountSign is "negative_is_expense" (default) or "positive_is_expense".
	// It only applies to AmountColumn.
	AmountSign string `json:"amount_sign,omitempty"`
}

// CSVImportParams are the inputs of a CSV import.
type CSVImportParams struct {
	// AccountID is the cash, credit-card or debt account the rows go into; the
	// amounts are in its currency.
	AccountID string
	// Mapping is nil for the first dry run, which only inspects the file.
	Mapping *CSVMapping
	// Apply writes the rows; false is a dry run.
	Apply bool
}

// CSVImportAccount identifies the target account in the report.
type CSVImportAccount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// CSVImportRow is the outcome of one file row.
type CSVImportRow struct {
	Line        int    `json:"line"`
	Status      string `json:"status"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Amount      int64  `json:"amount,omitempty"`
	Category    string `json:"category,omitempty"`
	// CategorySource is "column" or "rule" when a category was found.
	CategorySource string `json:"category_source,omitempty"`
	// Reason explains a skipped or invalid row.
	Reason string `json:"reason,omitempty"`
}

// CSVImportIssue is a problem with one row.
type CSVImportIssue struct {
	Line   int    `json:"line"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// CSVImportSummary counts the rows by outcome.
type CSVImportSummary struct {
	Rows            int `json:"rows"`
	WouldCreate     int `json:"would_create"`
	SkipDuplicate   int `json:"skip_duplicate"`
	SkipManual      int `json:"skip_manual"`
	Invalid         int `json:"invalid"`
	Categorized     int `json:"categorized"`
	ByRule          int `json:"categorized_by_rule"`
	UnknownCategory int `json:"unknown_category"`
	// IncomeTotal and ExpenseTotal are the totals of the rows to create, in cents of the account's currency.
	IncomeTotal  int64  `json:"income_total"`
	ExpenseTotal int64  `json:"expense_total"`
	FirstDate    string `json:"first_date,omitempty"`
	LastDate     string `json:"last_date,omitempty"`
}

// CSVImportReport is the result of a dry run or an applied import.
type CSVImportReport struct {
	Applied   bool             `json:"applied"`
	Account   CSVImportAccount `json:"account"`
	Delimiter string           `json:"delimiter"`
	// Columns are the file's header names.
	Columns []string `json:"columns"`
	// Sample is the first rows of the file.
	Sample [][]string `json:"sample"`
	// SuggestedMapping is the mapping guessed from the headers and sample.
	SuggestedMapping CSVMapping `json:"suggested_mapping"`
	// Mapping is the mapping used; nil when the request carried none.
	Mapping *CSVMapping `json:"mapping,omitempty"`
	// Summary and Rows are present when a mapping was given.
	Summary       *CSVImportSummary `json:"summary,omitempty"`
	Rows          []CSVImportRow    `json:"rows,omitempty"`
	RowsTruncated bool              `json:"rows_truncated,omitempty"`
	Issues        []CSVImportIssue  `json:"issues,omitempty"`
	// Created is the number of transactions written (apply only).
	Created int `json:"created"`
	// DateFormats lists the accepted date_format values.
	DateFormats []string `json:"date_formats"`
}

type csvImportService struct {
	db *gorm.DB
}

// NewCSVImportService creates a CSVImportServicer.
func NewCSVImportService(db *gorm.DB) CSVImportServicer {
	return &csvImportService{db: db}
}

// csvRow is a parsed, not yet classified file row.
type csvRow struct {
	line     int
	date     time.Time
	typ      models.TransactionType
	amount   int64
	desc     string
	notes    string
	category string // raw category cell
	catID    *string
	catName  string
	catSrc   string
	ref      string
	status   string
	reason   string
}

// csvPlan is everything a dry run or an apply decides.
type csvPlan struct {
	account *models.Account
	file    *csvFile
	mapping *CSVMapping
	rows    []*csvRow
	issues  []CSVImportIssue
	summary CSVImportSummary
}

// ImportCSV parses the upload and, with a mapping, plans the import: which
// rows are new, which repeat an earlier import (same import_ref) and which
// match an entry typed by hand. A dry run writes nothing. With p.Apply the new
// rows are written in one DB transaction, categorised by the user's rules,
// with source "import". The upload is only held in memory.
func (s *csvImportService) ImportCSV(userID string, p CSVImportParams, file io.Reader) (*CSVImportReport, error) {
	if p.AccountID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account_id is required")
	}
	if p.Apply && p.Mapping == nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "a column mapping is required to apply an import")
	}
	f, err := parseCSVFile(file)
	if err != nil {
		return nil, err
	}

	if !p.Apply {
		pl, err := s.plan(s.db, userID, p, f)
		if err != nil {
			return nil, err
		}
		s.logCounts(pl, false)
		return pl.report(false, 0), nil
	}

	mu, _ := importLocks.LoadOrStore(userID, &sync.Mutex{})
	lock, ok := mu.(*sync.Mutex)
	if !ok || !lock.TryLock() {
		return nil, apperrors.ErrImportInProgress
	}
	defer lock.Unlock()

	var out *CSVImportReport
	var applied *csvPlan
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockAccounts(tx, userID, []string{p.AccountID}, true); err != nil {
			return err
		}
		pl, err := s.plan(tx, userID, p, f)
		if err != nil {
			return err
		}
		if len(pl.issues) > 0 && pl.summary.Invalid == pl.summary.Rows {
			return apperrors.WithDetails(apperrors.ErrImportInvalid, map[string]any{"report": pl.report(false, 0)})
		}
		n, err := s.write(tx, userID, pl)
		if err != nil {
			return err
		}
		out, applied = pl.report(true, n), pl
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logCounts(applied, true)
	return out, nil
}

func (s *csvImportService) logCounts(pl *csvPlan, applied bool) {
	logger.Get().Infow("csv import",
		"applied", applied, "rows", pl.summary.Rows, "create", pl.summary.WouldCreate,
		"skip_duplicate", pl.summary.SkipDuplicate, "skip_manual", pl.summary.SkipManual,
		"invalid", pl.summary.Invalid)
}

// plan builds the import plan reading through db; it writes nothing.
func (s *csvImportService) plan(db *gorm.DB, userID string, p CSVImportParams, f *csvFile) (*csvPlan, error) {
	acct, err := loadImportAccount(db, userID, p.AccountID, true)
	if err != nil {
		return nil, err
	}
	if !acct.Type.IsSpending() {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account_id must be a cash, credit card or debt account")
	}
	pl := &csvPlan{account: acct, file: f}
	if p.Mapping == nil {
		return pl, nil
	}
	m, err := resolveCSVMapping(f, *p.Mapping, f.suggestMapping(isLiabilityAccount(acct.Type)))
	if err != nil {
		return nil, err
	}
	pl.mapping = m

	pl.parseRows()
	if err := s.classify(db, userID, pl); err != nil {
		return nil, err
	}
	pl.tally()
	return pl, nil
}

// resolveCSVMapping fills the blanks a request may leave (date format, decimal
// separator, amount sign) from the suggestion and checks every named column.
func resolveCSVMapping(f *csvFile, in, suggested CSVMapping) (*CSVMapping, error) {
	m := in
	invalid := func(msg string) error { return apperrors.WithMessage(apperrors.ErrInvalidInput, msg) }
	for _, c := range []struct{ field, val string }{
		{"date_column", m.DateColumn}, {"description_column", m.DescriptionColumn},
		{"amount_column", m.AmountColumn}, {"debit_column", m.DebitColumn},
		{"credit_column", m.CreditColumn}, {"category_column", m.CategoryColumn},
		{"notes_column", m.NotesColumn},
	} {
		if c.val != "" && f.columnIndex(c.val) < 0 {
			return nil, invalid(fmt.Sprintf("%s %q is not a column of the file", c.field, c.val))
		}
	}
	if m.DateColumn == "" {
		return nil, invalid("date_column is required")
	}
	if m.DescriptionColumn == "" {
		return nil, invalid("description_column is required")
	}
	switch {
	case m.AmountColumn != "" && (m.DebitColumn != "" || m.CreditColumn != ""):
		return nil, invalid("use either amount_column or debit_column and credit_column, not both")
	case m.AmountColumn == "" && (m.DebitColumn == "" || m.CreditColumn == ""):
		return nil, invalid("amount_column, or both debit_column and credit_column, is required")
	}
	if m.DateFormat == "" {
		m.DateFormat = suggested.DateFormat
	}
	if _, ok := csvDateLayout(m.DateFormat); !ok {
		return nil, invalid("date_format must be one of: " + strings.Join(CSVDateFormatNames(), ", "))
	}
	if m.DecimalSeparator == "" {
		m.DecimalSeparator = suggested.DecimalSeparator
	}
	if m.DecimalSeparator != "." && m.DecimalSeparator != "," {
		return nil, invalid(`decimal_separator must be "." or ","`)
	}
	switch m.AmountSign {
	case "":
		m.AmountSign = CSVNegativeIsExpense
	case CSVNegativeIsExpense, CSVPositiveIsExpense:
	default:
		return nil, invalid("amount_sign must be negative_is_expense or positive_is_expense")
	}
	return &m, nil
}

// parseRows turns the file rows into csvRows, marking unreadable ones invalid.
func (pl *csvPlan) parseRows() {
	f, m := pl.file, pl.mapping
	layout, _ := csvDateLayout(m.DateFormat)
	iDate, iDesc := f.columnIndex(m.DateColumn), f.columnIndex(m.DescriptionColumn)
	iAmt, iDeb, iCred := f.columnIndex(m.AmountColumn), f.columnIndex(m.DebitColumn), f.columnIndex(m.CreditColumn)
	iCat, iNotes := f.columnIndex(m.CategoryColumn), f.columnIndex(m.NotesColumn)

	for i, rec := range f.rows {
		r := &csvRow{line: f.lines[i], status: CSVRowCreate}
		pl.rows = append(pl.rows, r)
		fail := func(code, detail string) {
			r.status, r.reason = CSVRowInvalid, detail
			pl.issues = append(pl.issues, CSVImportIssue{Line: r.line, Code: code, Detail: detail})
		}
		date, err := parseCSVDate(f.cell(rec, iDate), layout)
		if err != nil {
			fail("bad_date", fmt.Sprintf("the date %q does not match %s", truncateRunes(f.cell(rec, iDate), 40), m.DateFormat))
			continue
		}
		r.date = date
		r.desc = cleanCSVText(f.cell(rec, iDesc), MaxDescriptionLen)
		if r.desc == "" {
			fail("empty_description", "the description is empty")
			continue
		}
		var cents int64
		var marker string
		if iAmt >= 0 {
			cents, marker, err = parseCSVAmount(f.cell(rec, iAmt), m.DecimalSeparator)
			if err != nil {
				fail("bad_amount", fmt.Sprintf("the amount %q is not readable: %v", truncateRunes(f.cell(rec, iAmt), 40), err))
				continue
			}
			r.typ, r.amount = csvTypeFor(cents, m.AmountSign)
			switch marker {
			case "cr":
				r.typ = models.TransactionTypeIncome
			case "dr":
				r.typ = models.TransactionTypeExpense
			}
		} else {
			deb, cred := pl.splitDebitCredit(f.cell(rec, iDeb), f.cell(rec, iCred), m.DecimalSeparator)
			switch {
			case deb.err != nil:
				fail("bad_amount", "the debit is not readable: "+deb.err.Error())
				continue
			case cred.err != nil:
				fail("bad_amount", "the credit is not readable: "+cred.err.Error())
				continue
			case deb.v != 0 && cred.v != 0:
				fail("both_debit_credit", "both a debit and a credit are filled")
				continue
			case deb.v != 0:
				r.typ, r.amount = models.TransactionTypeExpense, absInt(deb.v)
			case cred.v != 0:
				r.typ, r.amount = models.TransactionTypeIncome, absInt(cred.v)
			}
		}
		if r.amount == 0 {
			fail("zero_amount", "the amount is zero")
			continue
		}
		r.notes = cleanCSVText(f.cell(rec, iNotes), MaxNotesLen)
		r.category = cleanCSVText(f.cell(rec, iCat), 100)
	}
}

type csvMoney struct {
	v   int64
	err error
}

// splitDebitCredit parses the two money cells; an empty one is zero.
func (pl *csvPlan) splitDebitCredit(debit, credit, dec string) (deb, cred csvMoney) {
	parse := func(s string) csvMoney {
		if strings.TrimSpace(s) == "" {
			return csvMoney{}
		}
		v, _, err := parseCSVAmount(s, dec)
		return csvMoney{v: v, err: err}
	}
	return parse(debit), parse(credit)
}

func absInt(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// classify decides each valid row's category, its import_ref and whether it is
// a repeat or matches an entry typed by hand.
func (s *csvImportService) classify(db *gorm.DB, userID string, pl *csvPlan) error {
	var valid []*csvRow
	for _, r := range pl.rows {
		if r.status == CSVRowCreate {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	if err := s.categorize(db, userID, pl, valid); err != nil {
		return err
	}
	assignRefs(pl.account.ID, valid)
	if err := s.markDuplicates(db, userID, valid); err != nil {
		return err
	}
	return s.markManualMatches(db, userID, pl.account.ID, valid)
}

// categorize resolves categories: a category cell that names one of the user's
// categories (of the matching type) wins, otherwise the user's rules decide.
// Rules are loaded once and run through the pure Match.
func (s *csvImportService) categorize(db *gorm.DB, userID string, pl *csvPlan, rows []*csvRow) error {
	var cats []models.Category
	if err := db.Where("user_id = ?", userID).Find(&cats).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	byName := map[string]*models.Category{}
	for i := range cats {
		c := &cats[i]
		byName[string(c.Type)+"|"+strings.ToLower(c.Name)] = c
	}
	var rules []models.TransactionRule
	if err := rulePreloads(db).
		Where("user_id = ? AND is_active = ?", userID, true).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	catTypeFor := map[models.TransactionType]models.CategoryType{
		models.TransactionTypeExpense: models.CategoryTypeExpense,
		models.TransactionTypeIncome:  models.CategoryTypeIncome,
	}
	byID := map[string]*models.Category{}
	for i := range cats {
		byID[cats[i].ID] = &cats[i]
	}
	for _, r := range rows {
		if r.category != "" {
			if c := byName[string(catTypeFor[r.typ])+"|"+strings.ToLower(r.category)]; c != nil {
				id := c.ID
				r.catID, r.catName, r.catSrc = &id, c.Name, "column"
				continue
			}
		}
		res := Match(rules, RuleInput{Description: r.desc, Amount: r.amount, AccountID: pl.account.ID, Type: r.typ})
		if res.CategoryID != nil {
			r.catID, r.catSrc = res.CategoryID, "rule"
			if c := byID[*res.CategoryID]; c != nil {
				r.catName = c.Name
			}
		}
	}
	return nil
}

// assignRefs sets import_ref = "csv:<account>:<row hash>". The hash covers the
// date, the signed amount and the normalised description plus how many
// identical rows came before in the file, so two identical purchases on one day
// stay distinct and a re-import of the same file gets the same refs.
func assignRefs(accountID string, rows []*csvRow) {
	seen := map[string]int{}
	for _, r := range rows {
		signed := r.amount
		if r.typ == models.TransactionTypeExpense {
			signed = -signed
		}
		key := fmt.Sprintf("%s|%d|%s", r.date.Format(importDateLayout), signed, normalizeDescriptionKey(r.desc))
		n := seen[key]
		seen[key] = n + 1
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", key, n)))
		r.ref = "csv:" + accountID + ":" + hex.EncodeToString(sum[:])[:32]
	}
}

// markDuplicates skips rows whose import_ref already exists for the user.
func (s *csvImportService) markDuplicates(db *gorm.DB, userID string, rows []*csvRow) error {
	refs := make([]string, len(rows))
	for i, r := range rows {
		refs[i] = r.ref
	}
	existing := map[string]bool{}
	for start := 0; start < len(refs); start += 500 {
		end := min(start+500, len(refs))
		var got []string
		if err := db.Model(&models.Transaction{}).
			Where("user_id = ? AND import_ref IN ?", userID, refs[start:end]).
			Pluck("import_ref", &got).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for _, g := range got {
			existing[g] = true
		}
	}
	for _, r := range rows {
		if existing[r.ref] {
			r.status, r.reason = CSVRowSkipDuplicate, "already imported"
		}
	}
	return nil
}

type manualEntry struct {
	day  time.Time
	desc string
	used bool
}

// markManualMatches skips rows that match an entry the user typed by hand: the
// same amount and type, a date within a day, and the same normalised
// description (or one containing the other). Each manual entry absorbs at most
// one row, so two real purchases on one day are not both lost to one entry.
func (s *csvImportService) markManualMatches(db *gorm.DB, userID, accountID string, rows []*csvRow) error {
	var open []*csvRow
	lo, hi := rows[0].date, rows[0].date
	for _, r := range rows {
		if r.status != CSVRowCreate {
			continue
		}
		open = append(open, r)
		if r.date.Before(lo) {
			lo = r.date
		}
		if r.date.After(hi) {
			hi = r.date
		}
	}
	if len(open) == 0 {
		return nil
	}
	var manual []models.Transaction
	if err := db.Select("id", "type", "amount", "description", "date").
		Where("user_id = ? AND account_id = ? AND import_ref IS NULL AND type IN ? AND date >= ? AND date < ?",
			userID, accountID, []models.TransactionType{models.TransactionTypeIncome, models.TransactionTypeExpense},
			lo.AddDate(0, 0, -csvFuzzyDays-1), hi.AddDate(0, 0, csvFuzzyDays+2)).
		Find(&manual).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	byKey := map[string][]*manualEntry{}
	for i := range manual {
		t := &manual[i]
		k := fmt.Sprintf("%s|%d", t.Type, t.Amount)
		d := t.Date.UTC()
		byKey[k] = append(byKey[k], &manualEntry{
			day:  time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC),
			desc: normalizeDescriptionKey(t.Description),
		})
	}
	for _, r := range open {
		want := normalizeDescriptionKey(r.desc)
		for _, e := range byKey[fmt.Sprintf("%s|%d", r.typ, r.amount)] {
			gap := r.date.Sub(e.day)
			if e.used || gap > csvFuzzyDays*24*time.Hour || gap < -csvFuzzyDays*24*time.Hour {
				continue
			}
			if descriptionsMatch(want, e.desc) {
				e.used = true
				r.status, r.reason = CSVRowSkipManual, "matches an entry you added by hand"
				break
			}
		}
	}
	return nil
}

// tally counts the outcomes.
func (pl *csvPlan) tally() {
	sm := &pl.summary
	sm.Rows = len(pl.rows)
	var firstDate, lastDate time.Time
	for _, r := range pl.rows {
		switch r.status {
		case CSVRowCreate:
			sm.WouldCreate++
			if r.typ == models.TransactionTypeIncome {
				sm.IncomeTotal += r.amount
			} else {
				sm.ExpenseTotal += r.amount
			}
			if r.catID != nil {
				sm.Categorized++
				if r.catSrc == "rule" {
					sm.ByRule++
				}
			} else if r.category != "" {
				sm.UnknownCategory++
			}
			if firstDate.IsZero() || r.date.Before(firstDate) {
				firstDate = r.date
			}
			if r.date.After(lastDate) {
				lastDate = r.date
			}
		case CSVRowSkipDuplicate:
			sm.SkipDuplicate++
		case CSVRowSkipManual:
			sm.SkipManual++
		case CSVRowInvalid:
			sm.Invalid++
		}
	}
	if !firstDate.IsZero() {
		sm.FirstDate, sm.LastDate = firstDate.Format(importDateLayout), lastDate.Format(importDateLayout)
	}
}

// write inserts the rows to create and moves the account balance once, by the
// sum of their effects, inside tx.
func (s *csvImportService) write(tx *gorm.DB, userID string, pl *csvPlan) (int, error) {
	var toCreate []models.Transaction
	var delta int64
	for _, r := range pl.rows {
		if r.status != CSVRowCreate {
			continue
		}
		ref := r.ref
		toCreate = append(toCreate, models.Transaction{
			UserID: userID, AccountID: pl.account.ID, CategoryID: r.catID, Type: r.typ,
			Amount: r.amount, Description: r.desc, Date: r.date, Notes: r.notes,
			Source: models.TransactionSourceImport, ImportRef: &ref,
		})
		d, _ := balanceDelta(pl.account.Type, r.typ, r.amount)
		delta += d
	}
	if len(toCreate) == 0 {
		return 0, nil
	}
	if err := tx.CreateInBatches(&toCreate, csvInsertBatch).Error; err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if err := tx.Model(&models.Account{}).Where("id = ? AND user_id = ?", pl.account.ID, userID).
		UpdateColumn("balance", gorm.Expr("balance + ?", delta)).Error; err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return len(toCreate), nil
}

// report renders the plan.
func (pl *csvPlan) report(applied bool, created int) *CSVImportReport {
	f := pl.file
	rep := &CSVImportReport{
		Applied:     applied,
		Account:     CSVImportAccount{ID: pl.account.ID, Name: pl.account.Name, Currency: pl.account.Currency},
		Delimiter:   string(f.delimiter),
		Columns:     f.header,
		DateFormats: CSVDateFormatNames(),
		Created:     created,
		Mapping:     pl.mapping,
	}
	for i := 0; i < len(f.rows) && i < csvSampleRows; i++ {
		rep.Sample = append(rep.Sample, f.rows[i])
	}
	rep.SuggestedMapping = f.suggestMapping(isLiabilityAccount(pl.account.Type))
	if pl.mapping == nil {
		return rep
	}
	sm := pl.summary
	rep.Summary = &sm
	issues := pl.issues
	if len(issues) > csvReportIssues {
		issues = issues[:csvReportIssues]
	}
	rep.Issues = issues
	// Show the problems first, then the rest in file order.
	order := make([]*csvRow, len(pl.rows))
	copy(order, pl.rows)
	sort.SliceStable(order, func(i, j int) bool {
		return (order[i].status == CSVRowInvalid) && (order[j].status != CSVRowInvalid)
	})
	for _, r := range order {
		if len(rep.Rows) == csvReportRows {
			rep.RowsTruncated = true
			break
		}
		row := CSVImportRow{Line: r.line, Status: r.status, Reason: r.reason}
		if !r.date.IsZero() {
			row.Date = r.date.Format(importDateLayout)
		}
		row.Description, row.Amount = r.desc, r.amount
		if r.typ != "" {
			row.Type = string(r.typ)
		}
		row.Category, row.CategorySource = r.catName, r.catSrc
		rep.Rows = append(rep.Rows, row)
	}
	return rep
}
