package services

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// readExportZip unzips an export into file name -> CSV records.
func readExportZip(t *testing.T, data []byte) map[string][][]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	out := map[string][][]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		recs, err := csv.NewReader(rc).ReadAll()
		_ = rc.Close()
		if err != nil {
			t.Fatalf("parse %s: %v", f.Name, err)
		}
		out[f.Name] = recs
	}
	return out
}

// col returns the value of column name in a CSV record.
func col(t *testing.T, recs [][]string, row int, name string) string {
	t.Helper()
	for i, h := range recs[0] {
		if h == name {
			return recs[row][i]
		}
	}
	t.Fatalf("no column %q in %v", name, recs[0])
	return ""
}

// seedExportUser builds a small but complete data set for one user.
func seedExportUser(t *testing.T, db *gorm.DB, email, secSymbol string) (*models.User, *models.Account) {
	t.Helper()
	user := testutil.CreateTestUserWithEmail(t, db, email)
	if err := db.Model(user).Update("base_currency", "MYR").Error; err != nil {
		t.Fatal(err)
	}
	cash := &models.Account{UserID: user.ID, Name: "Checking", Type: models.AccountTypeCash, Balance: 123456, Currency: "MYR", IsActive: true}
	usd := &models.Account{UserID: user.ID, Name: "USD wallet", Type: models.AccountTypeCash, Balance: 5000, Currency: "USD", IsActive: true}
	brokerage := &models.Account{UserID: user.ID, Name: "Brokerage", Type: models.AccountTypeInvestment, Currency: "USD", IsActive: true}
	for _, a := range []*models.Account{cash, usd, brokerage} {
		if err := db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
	}
	cat := &models.Category{UserID: user.ID, Name: "Food", Type: models.CategoryTypeExpense, Icon: "utensils", Color: "#fff"}
	if err := db.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	personal := int64(500)
	tx1 := &models.Transaction{UserID: user.ID, AccountID: cash.ID, CategoryID: &cat.ID, Type: models.TransactionTypeExpense, Amount: 1250, Description: "=SUM(A1)", Date: day, PersonalAmount: &personal}
	toAmount := int64(280)
	tx2 := &models.Transaction{UserID: user.ID, AccountID: cash.ID, ToAccountID: &usd.ID, ToAmount: &toAmount, Type: models.TransactionTypeTransfer, Amount: 1250, Description: "to usd, \"quoted\", comma", Date: day}
	tx3 := &models.Transaction{UserID: user.ID, AccountID: cash.ID, Type: models.TransactionTypeIncome, Amount: 99, Description: "deleted one", Date: day}
	for _, tx := range []*models.Transaction{tx1, tx2, tx3} {
		if err := db.Create(tx).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(tx3).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Budget{UserID: user.ID, CategoryID: cat.ID, Name: "Food budget", Amount: 50000, Period: models.BudgetPeriodMonthly, IsActive: true, Currency: "MYR"}).Error; err != nil {
		t.Fatal(err)
	}

	sec := &models.Security{Symbol: secSymbol, Name: secSymbol + " Inc", AssetType: models.AssetTypeStock, Currency: "USD"}
	other := &models.Security{Symbol: "UNHELD" + secSymbol, Name: "Not held", AssetType: models.AssetTypeStock, Currency: "USD"}
	for _, s := range []*models.Security{sec, other} {
		if err := db.Create(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	inv := &models.Investment{AccountID: brokerage.ID, SecurityID: sec.ID, Quantity: 2.5, CostBasis: 30000, CostBasisBase: 135000, RealizedGainLoss: 100, RealizedGainLossBase: 450}
	if err := db.Create(inv).Error; err != nil {
		t.Fatal(err)
	}
	rate, err := fx.ParseRate("4.5")
	if err != nil {
		t.Fatal(err)
	}
	base := int64(135000)
	itx := &models.InvestmentTransaction{InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: day, Quantity: 2.5, PricePerUnit: 12000, TotalAmount: 30000, Fee: 100, FXRateToBase: &rate, BaseAmount: &base}
	if err := db.Create(itx).Error; err != nil {
		t.Fatal(err)
	}

	rec := &models.Receivable{UserID: user.ID, TransactionID: tx1.ID, CounterpartyName: "Ali", CounterpartyKey: "ali", Kind: models.ReceivableKindSplit, Amount: 750, PaidAmount: 200, Status: models.ReceivableStatusPartiallyPaid}
	if err := db.Create(rec).Error; err != nil {
		t.Fatal(err)
	}
	acct := int64(45)
	st := &models.ReceivableSettlement{UserID: user.ID, ReceivableID: rec.ID, TransactionID: tx1.ID, AccountID: usd.ID, Amount: 200, AccountAmount: &acct, Date: day}
	if err := db.Create(st).Error; err != nil {
		t.Fatal(err)
	}

	rule := &models.TransactionRule{UserID: user.ID, Name: "Groceries", Priority: 1, IsActive: true}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TransactionRuleCondition{RuleID: rule.ID, Field: models.RuleFieldDescription, Operator: models.RuleOpContains, ValueText: "tesco"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TransactionRuleAction{RuleID: rule.ID, ActionType: models.RuleActionSetCategory, CategoryID: &cat.ID}).Error; err != nil {
		t.Fatal(err)
	}
	return user, cash
}

func TestExportService_ZipContentsAndScoping(t *testing.T) {
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	alice, aliceCash := seedExportUser(t, db, "alice-export@test.com", "AAA")
	bob, _ := seedExportUser(t, db, "bob-export@test.com", "BBB")

	svc := NewExportService(db)
	var buf bytes.Buffer
	if err := svc.WriteExport(alice.ID, &buf); err != nil {
		t.Fatalf("WriteExport: %v", err)
	}
	files := readExportZip(t, buf.Bytes())

	wantFiles := map[string]int{ // file -> data rows
		"accounts.csv": 3, "categories.csv": 1, "transactions.csv": 3, "budgets.csv": 1,
		"investments.csv": 1, "investment_transactions.csv": 1, "securities.csv": 1,
		"receivables.csv": 1, "receivable_settlements.csv": 1, "rules.csv": 1,
	}
	for name, rows := range wantFiles {
		recs, ok := files[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if len(recs) != rows+1 {
			t.Errorf("%s: got %d data rows, want %d", name, len(recs)-1, rows)
		}
	}
	if len(files) != len(wantFiles) {
		t.Errorf("got %d files, want %d", len(files), len(wantFiles))
	}

	// Accounts: money, formatted money and currency.
	accts := files["accounts.csv"]
	for i := 1; i < len(accts); i++ {
		if col(t, accts, i, "id") == aliceCash.ID {
			if got := col(t, accts, i, "balance"); got != "123456" {
				t.Errorf("balance = %q", got)
			}
			if got := col(t, accts, i, "balance_formatted"); got != "1234.56" {
				t.Errorf("balance_formatted = %q", got)
			}
			if got := col(t, accts, i, "currency"); got != "MYR" {
				t.Errorf("currency = %q", got)
			}
		}
	}

	// Transactions: soft-deleted row present with deleted_at, formula guarded,
	// transfer carries destination amount and currency.
	txs := files["transactions.csv"]
	var sawDeleted, sawFormula, sawTransfer bool
	for i := 1; i < len(txs); i++ {
		switch col(t, txs, i, "type") {
		case "income":
			sawDeleted = col(t, txs, i, "deleted_at") != ""
		case "expense":
			sawFormula = col(t, txs, i, "description") == "'=SUM(A1)" &&
				col(t, txs, i, "amount_formatted") == "12.50" &&
				col(t, txs, i, "personal_amount") == "500" && col(t, txs, i, "deleted_at") == ""
		case "transfer":
			sawTransfer = col(t, txs, i, "to_amount") == "280" && col(t, txs, i, "to_currency") == "USD" &&
				col(t, txs, i, "currency") == "MYR" && col(t, txs, i, "description") == "to usd, \"quoted\", comma"
		}
	}
	if !sawDeleted || !sawFormula || !sawTransfer {
		t.Errorf("transactions.csv content wrong: deleted=%v formula=%v transfer=%v\n%v", sawDeleted, sawFormula, sawTransfer, txs)
	}

	// Investments: security currency plus the base-currency twins.
	inv := files["investments.csv"]
	for name, want := range map[string]string{
		"currency": "USD", "cost_basis": "30000", "cost_basis_formatted": "300.00",
		"base_currency": "MYR", "cost_basis_base": "135000", "realized_gain_loss_base": "450", "quantity": "2.5",
	} {
		if got := col(t, inv, 1, name); got != want {
			t.Errorf("investments.%s = %q, want %q", name, got, want)
		}
	}
	itx := files["investment_transactions.csv"]
	for name, want := range map[string]string{
		"type": "buy", "currency": "USD", "total_amount_formatted": "300.00",
		"fx_rate_to_base": "4.5", "base_amount": "135000", "base_currency": "MYR", "fee_formatted": "1.00",
	} {
		if got := col(t, itx, 1, name); got != want {
			t.Errorf("investment_transactions.%s = %q, want %q", name, got, want)
		}
	}
	if got := col(t, files["securities.csv"], 1, "symbol"); got != "AAA" {
		t.Errorf("securities.symbol = %q, want only the held security AAA", got)
	}

	// Receivables and settlements use the expense account's currency; the
	// settlement also reports the credited account's currency and amount.
	rc := files["receivables.csv"]
	if col(t, rc, 1, "currency") != "MYR" || col(t, rc, 1, "outstanding_formatted") != "5.50" {
		t.Errorf("receivables row wrong: %v", rc[1])
	}
	se := files["receivable_settlements.csv"]
	if col(t, se, 1, "currency") != "MYR" || col(t, se, 1, "account_currency") != "USD" ||
		col(t, se, 1, "account_amount") != "45" || col(t, se, 1, "amount") != "200" {
		t.Errorf("settlement row wrong: %v", se[1])
	}

	rules := files["rules.csv"]
	if got := col(t, rules, 1, "conditions"); got != `[{"field":"description","operator":"contains","value_text":"tesco"}]` {
		t.Errorf("rule conditions = %s", got)
	}

	// Scoping: nothing of Bob's leaks into Alice's export.
	for name, recs := range files {
		for _, row := range recs[1:] {
			for _, cell := range row {
				if cell == bob.ID || cell == "BBB" || cell == "UNHELDBBB" || cell == "Bob" {
					t.Errorf("%s leaks other user's data: %v", name, row)
				}
			}
		}
	}
}

func TestExportService_EmptyUserHasHeadersOnly(t *testing.T) {
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	user := testutil.CreateTestUserWithEmail(t, db, "empty-export@test.com")

	var buf bytes.Buffer
	if err := NewExportService(db).WriteExport(user.ID, &buf); err != nil {
		t.Fatal(err)
	}
	files := readExportZip(t, buf.Bytes())
	if len(files) != 10 {
		t.Fatalf("got %d files, want 10", len(files))
	}
	for name, recs := range files {
		if len(recs) != 1 {
			t.Errorf("%s: want header only, got %d rows", name, len(recs))
		}
	}
	if err := NewExportService(db).WriteExport("00000000-0000-0000-0000-000000000000", &bytes.Buffer{}); err == nil {
		t.Error("unknown user should fail before writing")
	}
}

func TestExportCents(t *testing.T) {
	cases := map[int64]string{0: "0.00", 5: "0.05", 100: "1.00", 123456: "1234.56", -1250: "-12.50", -5: "-0.05"}
	for in, want := range cases {
		if got := exportCents(in); got != want {
			t.Errorf("exportCents(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemStatusService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	user := testutil.CreateTestUserWithEmail(t, db, "status@test.com")
	other := testutil.CreateTestUserWithEmail(t, db, "status-other@test.com")
	svc := NewSystemStatusService(db)

	st, err := svc.GetStatus(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LatestPriceAt != nil || st.LatestFXRateAt != nil || st.LatestSnapshotAt != nil || st.LastBackupAt != nil {
		t.Fatalf("empty database should give all nil, got %+v", st)
	}

	sec := testutil.CreateTestSecurity(t, db)
	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(48 * time.Hour)
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, t1)
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 110, t2)
	rate, _ := fx.ParseRate("4.5")
	if err := db.Create(&models.FXRate{Base: "USD", Quote: "MYR", Rate: rate, RecordedAt: t1}).Error; err != nil {
		t.Fatal(err)
	}
	snap := func(uid string, at time.Time) {
		if err := db.Create(&models.PortfolioSnapshot{UserID: uid, RecordedAt: at, Currency: "MYR"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	snap(user.ID, t1)
	snap(other.ID, t2.Add(time.Hour)) // another user's newer snapshot must not show
	size := int64(1024)
	runs := []models.BackupRun{
		{StartedAt: t1, FinishedAt: t1.Add(time.Minute), Status: "ok", SizeBytes: &size},
		{StartedAt: t2, FinishedAt: t2.Add(time.Minute), Status: "failed", Error: "boom"},
	}
	for i := range runs {
		if err := db.Create(&runs[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	st, err = svc.GetStatus(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	eq := func(name string, got *time.Time, want time.Time) {
		t.Helper()
		if got == nil || !got.Equal(want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	eq("latest_price_at", st.LatestPriceAt, t2)
	eq("latest_fx_rate_at", st.LatestFXRateAt, t1)
	eq("latest_snapshot_at", st.LatestSnapshotAt, t1)
	eq("last_backup_at", st.LastBackupAt, t1.Add(time.Minute)) // the failed run does not count
}
