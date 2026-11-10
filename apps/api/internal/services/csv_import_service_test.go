package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestParseCSVAmount(t *testing.T) {
	cases := []struct {
		in, dec string
		want    int64
		marker  string
		wantErr bool
	}{
		{"12.50", ".", 1250, "", false},
		{"-12.5", ".", -1250, "", false},
		{"(12.50)", ".", -1250, "", false},
		{"1,234.56", ".", 123456, "", false},
		{"1.234,56", ",", 123456, "", false},
		{"12,5", ",", 1250, "", false},
		{"RM 1,000.00", ".", 100000, "", false},
		{"$-3.10", ".", -310, "", false},
		{"3.10-", ".", -310, "", false},
		{"45.00 CR", ".", 4500, "cr", false},
		{"45.00DR", ".", 4500, "dr", false},
		{"1 234,00", ",", 123400, "", false},
		{"12.345", ".", 0, "", true},
		{"12.500", ".", 1250, "", false},
		{"abc", ".", 0, "", true},
		{"", ".", 0, "", true},
		{"1.2.3", ".", 0, "", true},
	}
	for _, c := range cases {
		got, marker, err := parseCSVAmount(c.in, c.dec)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got %d", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want || marker != c.marker {
			t.Errorf("%q (%s): got %d %q %v, want %d %q", c.in, c.dec, got, marker, err, c.want, c.marker)
		}
	}
}

func TestParseCSVFileDelimiterBOMAndLimits(t *testing.T) {
	f, err := parseCSVFile(strings.NewReader("\ufeffDate;Description;Amount\n\n05/03/2024;Coffee;-3,50\n06/03/2024;\"Lunch; with team\";-12,00\n"))
	testutil.AssertNoError(t, err)
	if f.delimiter != ';' || len(f.rows) != 2 || f.header[0] != "Date" {
		t.Fatalf("delimiter %q header %v rows %d", f.delimiter, f.header, len(f.rows))
	}
	if f.rows[1][1] != "Lunch; with team" {
		t.Errorf("quoted field = %q", f.rows[1][1])
	}

	var b strings.Builder
	b.WriteString("date,description,amount\n")
	for i := 0; i <= CSVImportMaxRows; i++ {
		fmt.Fprintf(&b, "2024-01-01,row %d,-1.00\n", i)
	}
	_, err = parseCSVFile(strings.NewReader(b.String()))
	testutil.AssertAppError(t, err, "CSV_TOO_MANY_ROWS")

	_, err = parseCSVFile(strings.NewReader("date,description,amount\n"))
	testutil.AssertAppError(t, err, "INVALID_INPUT")
	_, err = parseCSVFile(strings.NewReader(""))
	testutil.AssertAppError(t, err, "INVALID_INPUT")
	_, err = parseCSVFile(strings.NewReader("a,b\n\xff\xfe,1\n"))
	testutil.AssertAppError(t, err, "INVALID_INPUT")
}

func TestSuggestMapping(t *testing.T) {
	f, err := parseCSVFile(strings.NewReader("Transaction Date,Details,Withdrawal,Deposit,Remarks\n25/12/2024,Coffee,3.50,,\n26/12/2024,Salary,,2500.00,\n"))
	testutil.AssertNoError(t, err)
	m := f.suggestMapping(false)
	if m.DateColumn != "Transaction Date" || m.DescriptionColumn != "Details" || m.DebitColumn != "Withdrawal" ||
		m.CreditColumn != "Deposit" || m.NotesColumn != "Remarks" || m.AmountColumn != "" {
		t.Fatalf("mapping = %+v", m)
	}
	if m.DateFormat != "DD/MM/YYYY" || m.DecimalSeparator != "." {
		t.Errorf("format = %q sep = %q", m.DateFormat, m.DecimalSeparator)
	}

	f, _ = parseCSVFile(strings.NewReader("Date,Description,Amount\n05/03/2024,Coffee,\"3,50\"\n06/03/2024,Lunch,\"12,00\"\n"))
	m = f.suggestMapping(false)
	if m.DecimalSeparator != "," {
		t.Errorf("decimal comma not detected: %+v", m)
	}
	// 05/03 and 06/03 are ambiguous; the first listed day-first format wins.
	if m.DateFormat != "DD/MM/YYYY" {
		t.Errorf("date format = %q", m.DateFormat)
	}
	f, _ = parseCSVFile(strings.NewReader("Date,Description,Amount\n03/25/2024,Coffee,3.50\n"))
	if got := f.suggestMapping(false).DateFormat; got != "MM/DD/YYYY" {
		t.Errorf("month-first date format = %q", got)
	}

	// A stray footer line does not defeat the guess; garbage does.
	f, _ = parseCSVFile(strings.NewReader("Date,Description,Amount\n05/03/2024,A,1\n06/03/2024,B,2\n07/03/2024,C,3\nTotal,,6\n"))
	if got := f.suggestMapping(false).DateFormat; got != "DD/MM/YYYY" {
		t.Errorf("date format with a footer = %q", got)
	}
	f, _ = parseCSVFile(strings.NewReader("Date,Description,Amount\nfoo,A,1\nbar,B,2\n"))
	if got := f.suggestMapping(false).DateFormat; got != "" {
		t.Errorf("date format for garbage = %q", got)
	}

	// A card statement listing purchases as positive amounts.
	f, _ = parseCSVFile(strings.NewReader("Date,Description,Amount\n2024-03-05,Coffee,3.50\n2024-03-06,Lunch,12.00\n2024-03-07,Refund,-4.00\n"))
	if got := f.suggestMapping(true).AmountSign; got != CSVPositiveIsExpense {
		t.Errorf("liability sign = %q", got)
	}
	if got := f.suggestMapping(false).AmountSign; got != CSVNegativeIsExpense {
		t.Errorf("cash sign = %q", got)
	}
}

func csvTestSetup(t *testing.T) (*csvImportService, *models.User, *models.Account) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	user := testutil.CreateTestUser(t, db)
	acct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	svc, ok := NewCSVImportService(db).(*csvImportService)
	if !ok {
		t.Fatal("unexpected service type")
	}
	return svc, user, acct
}

const csvBank = `Date,Description,Debit,Credit,Category
2024-03-01,ACME PAYROLL,,2500.00,Salary
2024-03-02,Whole Foods Market,45.10,,
2024-03-02,Whole Foods Market,45.10,,
2024-03-03,Mystery,10.00,,Nope
not a date,Broken,1.00,,
2024-03-04,Zero,0,,
`

func csvBankMapping() *CSVMapping {
	return &CSVMapping{
		DateColumn: "Date", DescriptionColumn: "Description", DebitColumn: "Debit", CreditColumn: "Credit",
		CategoryColumn: "Category", DateFormat: "YYYY-MM-DD", DecimalSeparator: ".",
	}
}

func TestCSVImportDryRunWithoutMappingInspectsOnly(t *testing.T) {
	svc, user, acct := csvTestSetup(t)
	rep, err := svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID}, strings.NewReader(csvBank))
	testutil.AssertNoError(t, err)
	if rep.Applied || rep.Summary != nil || len(rep.Columns) != 5 || len(rep.Sample) != csvSampleRows {
		t.Fatalf("report = %+v", rep)
	}
	if rep.SuggestedMapping.DateColumn != "Date" || rep.SuggestedMapping.DebitColumn != "Debit" {
		t.Errorf("suggested = %+v", rep.SuggestedMapping)
	}
	if n := countRows(t, svc.db, &models.Transaction{}, "import_ref IS NOT NULL"); n != 0 {
		t.Errorf("dry run wrote %d rows", n)
	}
}

func TestCSVImportApplyRulesDedupeAndBalance(t *testing.T) {
	svc, user, acct := csvTestSetup(t)
	db := svc.db

	groceries := &models.Category{UserID: user.ID, Name: "Groceries", Type: models.CategoryTypeExpense}
	salary := &models.Category{UserID: user.ID, Name: "Salary", Type: models.CategoryTypeIncome}
	db.Create(groceries)
	db.Create(salary)
	rule := &models.TransactionRule{UserID: user.ID, Name: "WF", IsActive: true}
	db.Omit("Conditions", "Actions").Create(rule)
	db.Create(&models.TransactionRuleCondition{RuleID: rule.ID, Field: models.RuleFieldDescription, Operator: models.RuleOpContains, ValueText: "whole foods"})
	db.Omit("Category").Create(&models.TransactionRuleAction{RuleID: rule.ID, ActionType: models.RuleActionSetCategory, CategoryID: &groceries.ID})

	p := CSVImportParams{AccountID: acct.ID, Mapping: csvBankMapping()}
	dry, err := svc.ImportCSV(user.ID, p, strings.NewReader(csvBank))
	testutil.AssertNoError(t, err)
	sm := dry.Summary
	if sm.Rows != 6 || sm.WouldCreate != 4 || sm.Invalid != 2 || sm.ByRule != 2 || sm.Categorized != 3 || sm.UnknownCategory != 1 {
		t.Fatalf("summary = %+v", sm)
	}
	if sm.IncomeTotal != 250000 || sm.ExpenseTotal != 4510*2+1000 {
		t.Errorf("totals = %d / %d", sm.IncomeTotal, sm.ExpenseTotal)
	}
	if len(dry.Issues) != 2 || dry.Issues[0].Code != "bad_date" {
		t.Errorf("issues = %+v", dry.Issues)
	}
	if n := countRows(t, db, &models.Transaction{}, "import_ref IS NOT NULL"); n != 0 {
		t.Fatalf("dry run wrote %d rows", n)
	}

	p.Apply = true
	rep, err := svc.ImportCSV(user.ID, p, strings.NewReader(csvBank))
	testutil.AssertNoError(t, err)
	if !rep.Applied || rep.Created != 4 {
		t.Fatalf("created %d applied %v", rep.Created, rep.Applied)
	}
	var rows []models.Transaction
	db.Where("import_ref IS NOT NULL").Order("date, amount").Find(&rows)
	if len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	refs := map[string]bool{}
	for _, r := range rows {
		if r.Source != models.TransactionSourceImport || r.AccountID != acct.ID || !strings.HasPrefix(*r.ImportRef, "csv:"+acct.ID+":") {
			t.Errorf("row = %+v", r)
		}
		if len(*r.ImportRef) > importRefMax {
			t.Errorf("import_ref too long: %d", len(*r.ImportRef))
		}
		refs[*r.ImportRef] = true
	}
	if len(refs) != 4 {
		t.Errorf("identical purchases must keep distinct refs, got %d", len(refs))
	}
	var bal models.Account
	db.First(&bal, "id = ?", acct.ID)
	if want := int64(100000 + 250000 - 4510*2 - 1000); bal.Balance != want {
		t.Errorf("balance = %d, want %d", bal.Balance, want)
	}
	var wf models.Transaction
	db.Where("description = ? AND import_ref IS NOT NULL", "Whole Foods Market").First(&wf)
	if wf.CategoryID == nil || *wf.CategoryID != groceries.ID {
		t.Errorf("rule did not categorise: %+v", wf.CategoryID)
	}
	var pay models.Transaction
	db.Where("description = ?", "ACME PAYROLL").First(&pay)
	if pay.CategoryID == nil || *pay.CategoryID != salary.ID || pay.Type != models.TransactionTypeIncome {
		t.Errorf("category column not applied: %+v", pay)
	}

	// Re-running the same file creates nothing.
	again, err := svc.ImportCSV(user.ID, p, strings.NewReader(csvBank))
	testutil.AssertNoError(t, err)
	if again.Created != 0 || again.Summary.SkipDuplicate != 4 {
		t.Errorf("re-import: created %d summary %+v", again.Created, again.Summary)
	}
	if n := countRows(t, db, &models.Transaction{}, "import_ref IS NOT NULL"); n != 4 {
		t.Errorf("%d rows after re-import", n)
	}
}

func TestCSVImportSkipsRowsMatchingManualEntries(t *testing.T) {
	svc, user, acct := csvTestSetup(t)
	db := svc.db
	day := func(d int) time.Time { return time.Date(2024, 3, d, 18, 30, 0, 0, time.UTC) }
	// Typed by hand: one coffee a day late, one lunch with a shorter description.
	db.Create(&models.Transaction{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 350, Description: "coffee", Date: day(6)})
	db.Create(&models.Transaction{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 1200, Description: "Lunch", Date: day(7)})
	// A different amount does not match.
	db.Create(&models.Transaction{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 999, Description: "Taxi", Date: day(7)})

	file := "date,description,amount\n" +
		"2024-03-05,COFFEE,-3.50\n" + // within a day of the typed coffee
		"2024-03-05,COFFEE,-3.50\n" + // a second coffee: the typed one is already used
		"2024-03-07,Lunch - Cafe 1234 SG,-12.00\n" +
		"2024-03-07,Taxi,-10.00\n"
	p := CSVImportParams{AccountID: acct.ID, Mapping: &CSVMapping{
		DateColumn: "date", DescriptionColumn: "description", AmountColumn: "amount", DateFormat: "YYYY-MM-DD", DecimalSeparator: ".",
	}}
	rep, err := svc.ImportCSV(user.ID, p, strings.NewReader(file))
	testutil.AssertNoError(t, err)
	if rep.Summary.SkipManual != 2 || rep.Summary.WouldCreate != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	for _, r := range rep.Rows {
		if r.Status == CSVRowSkipManual && r.Reason == "" {
			t.Error("skipped row without a reason")
		}
	}
}

func TestCSVImportCardSignAndTarget(t *testing.T) {
	svc, user, _ := csvTestSetup(t)
	card := testutil.CreateTestCreditCardAccount(t, svc.db, user.ID, 0)
	invest := testutil.CreateTestInvestmentAccount(t, svc.db, user.ID)
	file := "Date,Description,Amount\n2024-03-05,Coffee,3.50\n2024-03-06,Payment received,-100.00\n"
	m := &CSVMapping{DateColumn: "Date", DescriptionColumn: "Description", AmountColumn: "Amount", DateFormat: "YYYY-MM-DD", DecimalSeparator: ".", AmountSign: CSVPositiveIsExpense}
	rep, err := svc.ImportCSV(user.ID, CSVImportParams{AccountID: card.ID, Mapping: m, Apply: true}, strings.NewReader(file))
	testutil.AssertNoError(t, err)
	if rep.Created != 2 {
		t.Fatalf("created %d", rep.Created)
	}
	var got models.Account
	svc.db.First(&got, "id = ?", card.ID)
	if got.Balance != 350-10000 { // a purchase raises the debt, a payment lowers it
		t.Errorf("card balance = %d", got.Balance)
	}

	_, err = svc.ImportCSV(user.ID, CSVImportParams{AccountID: invest.ID}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "INVALID_INPUT")
}

func TestCSVImportValidationAndScoping(t *testing.T) {
	svc, user, acct := csvTestSetup(t)
	other := testutil.CreateTestUser(t, svc.db)
	file := "Date,Description,Amount\n2024-03-05,Coffee,-3.50\n"
	good := CSVMapping{DateColumn: "Date", DescriptionColumn: "Description", AmountColumn: "Amount", DateFormat: "YYYY-MM-DD", DecimalSeparator: "."}

	bad := good
	bad.AmountColumn = "Nope"
	_, err := svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID, Mapping: &bad}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "INVALID_INPUT")

	bad = good
	bad.DebitColumn, bad.CreditColumn = "Amount", "Amount"
	_, err = svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID, Mapping: &bad}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "INVALID_INPUT")

	bad = good
	bad.DateFormat = "WHENEVER"
	_, err = svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID, Mapping: &bad}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "INVALID_INPUT")

	_, err = svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID, Apply: true}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "INVALID_INPUT")

	// Another user's account is invisible.
	_, err = svc.ImportCSV(other.ID, CSVImportParams{AccountID: acct.ID, Mapping: &good}, strings.NewReader(file))
	testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")

	// Nothing readable: apply is refused with the report attached.
	allBad := "Date,Description,Amount\nxx,Coffee,-3.50\n"
	_, err = svc.ImportCSV(user.ID, CSVImportParams{AccountID: acct.ID, Mapping: &good, Apply: true}, strings.NewReader(allBad))
	testutil.AssertAppError(t, err, "IMPORT_INVALID")
}
