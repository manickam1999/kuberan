package services

import (
	"errors"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/services/fdmath"
)

// demoLocks holds one mutex per user so a load or removal runs one at a time.
var demoLocks sync.Map

// Tables recorded in demo_rows.
const (
	demoTableAccounts     = "accounts"
	demoTableCategories   = "categories"
	demoTableTransactions = "transactions"
	demoTableBudgets      = "budgets"
	demoTableSecurities   = "securities"
	demoTableInvestments  = "investments"
	demoTableInvTx        = "investment_transactions"
	demoTableReceivables  = "receivables"
	demoTableSettlements  = "receivable_settlements"
	demoTableRules        = "transaction_rules"
	demoTableConditions   = "transaction_rule_conditions"
	demoTableActions      = "transaction_rule_actions"
	demoTableFixedDeposit = "fixed_deposit_details"
)

// demoDeleteOrder lists the demo tables children first, so a removal never
// violates a foreign key.
var demoDeleteOrder = []string{
	demoTableSettlements, demoTableReceivables, demoTableTransactions,
	demoTableInvTx, demoTableInvestments, demoTableBudgets,
	demoTableActions, demoTableConditions, demoTableRules,
	demoTableFixedDeposit, demoTableAccounts, demoTableCategories, demoTableSecurities,
}

// demoDeleteChunk bounds the ids in one DELETE ... IN (...) statement.
const demoDeleteChunk = 400

// DemoResult reports what a load or removal touched, by table.
type DemoResult struct {
	// Rows counts the rows by table name.
	Rows map[string]int `json:"rows"`
	// Total is the sum of Rows.
	Total int `json:"total"`
}

type demoService struct {
	db  *gorm.DB
	fxs FXServicer
	now func() time.Time
}

// DemoOption configures a DemoServicer.
type DemoOption func(*demoService)

// WithDemoFXService lets the sample household include a foreign-currency
// account when a rate for it exists.
func WithDemoFXService(fxs FXServicer) DemoOption {
	return func(s *demoService) { s.fxs = fxs }
}

// WithDemoClock overrides the clock (tests).
func WithDemoClock(now func() time.Time) DemoOption {
	return func(s *demoService) { s.now = now }
}

// NewDemoService creates a DemoServicer.
func NewDemoService(db *gorm.DB, opts ...DemoOption) DemoServicer {
	s := &demoService{db: db, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// demoRecorder collects the rows to list in demo_rows.
type demoRecorder struct {
	userID string
	rows   []models.DemoRow
}

func (r *demoRecorder) add(table, id string) {
	r.rows = append(r.rows, models.DemoRow{UserID: r.userID, Table: table, RowID: id})
}

func (r *demoRecorder) counts() DemoResult {
	res := DemoResult{Rows: map[string]int{}}
	for _, row := range r.rows {
		res.Rows[row.Table]++
		res.Total++
	}
	return res
}

func demoLock(userID string) (*sync.Mutex, bool) {
	mu, _ := demoLocks.LoadOrStore(userID, &sync.Mutex{})
	lock, ok := mu.(*sync.Mutex)
	if !ok || !lock.TryLock() {
		return nil, false
	}
	return lock, true
}

// IsLoaded reports whether the user has sample rows.
func (s *demoService) IsLoaded(userID string) (bool, error) {
	var n int64
	if err := s.db.Model(&models.DemoRow{}).Where("user_id = ?", userID).Limit(1).Count(&n).Error; err != nil {
		return false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return n > 0, nil
}

// Load creates the sample household. It refuses when it is already loaded or
// when the user has transactions of their own (opening balances and sample
// rows do not count). Everything is written, and listed in demo_rows, in one
// database transaction.
func (s *demoService) Load(userID string) (*DemoResult, error) {
	lock, ok := demoLock(userID)
	if !ok {
		return nil, apperrors.ErrDemoBusy
	}
	defer lock.Unlock()

	rec := &demoRecorder{userID: userID}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&models.DemoRow{}).Where("user_id = ?", userID).Count(&existing).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if existing > 0 {
			return apperrors.ErrDemoAlreadyLoaded
		}
		var own int64
		if err := tx.Model(&models.Transaction{}).
			Where("user_id = ? AND type <> ?", userID, models.TransactionTypeOpeningBalance).
			Count(&own).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if own > 0 {
			return apperrors.ErrDemoHasData
		}
		base, err := userBaseCurrency(tx, userID)
		if err != nil {
			return err
		}
		b := &demoBuilder{db: tx, userID: userID, base: base, rec: rec, now: s.now(), fxs: s.fxs}
		if err := b.build(); err != nil {
			return err
		}
		if len(rec.rows) > 0 {
			if err := tx.CreateInBatches(&rec.rows, 100).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	res := rec.counts()
	logger.Get().Infow("demo data loaded", "rows", res.Total)
	return &res, nil
}

// Remove hard-deletes the rows listed in demo_rows, children first, in one
// database transaction, together with whatever the user added inside the sample
// accounts and categories since (see adoptDemoDependents). It refuses with
// DEMO_IN_USE when removing would damage data outside the sample household. A
// user with no sample rows gets an empty result.
func (s *demoService) Remove(userID string) (*DemoResult, error) {
	lock, ok := demoLock(userID)
	if !ok {
		return nil, apperrors.ErrDemoBusy
	}
	defer lock.Unlock()

	res := DemoResult{Rows: map[string]int{}}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var loaded int64
		if err := tx.Model(&models.DemoRow{}).Where("user_id = ?", userID).Count(&loaded).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if loaded == 0 {
			return nil
		}
		if err := adoptDemoDependents(tx, userID); err != nil {
			return err
		}
		if err := s.assertNotInUse(tx, userID); err != nil {
			return err
		}
		var rows []models.DemoRow
		if err := tx.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		ids := map[string][]string{}
		for _, r := range rows {
			ids[r.Table] = append(ids[r.Table], r.RowID)
		}
		if err := s.detachRealRows(tx, userID); err != nil {
			return err
		}
		for _, table := range demoDeleteOrder {
			n, err := deleteDemoTable(tx, userID, table, ids[table])
			if err != nil {
				return err
			}
			if n > 0 {
				res.Rows[table] = n
				res.Total += n
			}
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.DemoRow{}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if err := sweepDemoSecurities(tx); err != nil {
			return err
		}
		if err := clearAccountOrphans(tx, userID); err != nil {
			return err
		}
		return clearEmptySnapshots(tx, userID)
	})
	if err != nil {
		return nil, err
	}
	logger.Get().Infow("demo data removed", "rows", res.Total)
	return &res, nil
}

// demoSubquery selects the ids the demo recorded for a table.
const demoSubquery = "(SELECT row_id FROM demo_rows WHERE user_id = ? AND table_name = ?)"

// adoptDemoDependents lists in demo_rows what hangs off the sample accounts and
// categories but was not created by the load: the user's own transactions on a
// sample account, the receivables and repayments of those, holdings bought in a
// sample brokerage account and budgets set on a sample category. They cannot
// outlive the account or category they belong to, so the removal takes them
// along. Rows are only ever adopted through a sample account or category, so
// real accounts are never touched.
func adoptDemoDependents(tx *gorm.DB, userID string) error {
	steps := []struct {
		table string
		query string
		args  []any
	}{
		{demoTableTransactions,
			"SELECT id FROM transactions WHERE user_id = ? AND (account_id IN " + demoSubquery + " OR to_account_id IN " + demoSubquery + ") AND id NOT IN " + demoSubquery,
			[]any{userID, userID, demoTableAccounts, userID, demoTableAccounts, userID, demoTableTransactions}},
		{demoTableReceivables,
			"SELECT id FROM receivables WHERE user_id = ? AND transaction_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, userID, demoTableTransactions, userID, demoTableReceivables}},
		{demoTableSettlements,
			"SELECT id FROM receivable_settlements WHERE user_id = ? AND receivable_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, userID, demoTableReceivables, userID, demoTableSettlements}},
		{demoTableInvestments,
			"SELECT id FROM investments WHERE account_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, demoTableAccounts, userID, demoTableInvestments}},
		{demoTableInvTx,
			"SELECT id FROM investment_transactions WHERE investment_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, demoTableInvestments, userID, demoTableInvTx}},
		{demoTableBudgets,
			"SELECT id FROM budgets WHERE user_id = ? AND category_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, userID, demoTableCategories, userID, demoTableBudgets}},
	}
	for _, st := range steps {
		var ids []string
		if err := tx.Raw(st.query, st.args...).Scan(&ids).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if len(ids) == 0 {
			continue
		}
		rows := make([]models.DemoRow, len(ids))
		for i, id := range ids {
			rows[i] = models.DemoRow{UserID: userID, Table: st.table, RowID: id}
		}
		if err := tx.CreateInBatches(&rows, 100).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return nil
}

// assertNotInUse refuses a removal that would damage data outside the sample
// household: a transfer with one real leg, a receipt (its file lives in object
// storage), a repayment that credited a real account or a real receivable's
// repayment that credited a sample account, and a real fixed deposit paying
// out to a sample account. Run after adoptDemoDependents.
func (s *demoService) assertNotInUse(tx *gorm.DB, userID string) error {
	checks := []struct {
		sql  string
		args []any
	}{
		{"SELECT COUNT(*) FROM transactions WHERE user_id = ? AND id IN " + demoSubquery +
			" AND ((account_id IN " + demoSubquery + " AND to_account_id IS NOT NULL AND to_account_id NOT IN " + demoSubquery + ")" +
			" OR (account_id NOT IN " + demoSubquery + " AND to_account_id IN " + demoSubquery + "))",
			[]any{userID, userID, demoTableTransactions, userID, demoTableAccounts, userID, demoTableAccounts,
				userID, demoTableAccounts, userID, demoTableAccounts}},
		{"SELECT COUNT(*) FROM transaction_attachments WHERE transaction_id IN " + demoSubquery,
			[]any{userID, demoTableTransactions}},
		{"SELECT COUNT(*) FROM receivable_settlements WHERE user_id = ? AND ((receivable_id IN " + demoSubquery + " AND account_id NOT IN " + demoSubquery + ")" +
			" OR (receivable_id NOT IN " + demoSubquery + " AND account_id IN " + demoSubquery + "))",
			[]any{userID, userID, demoTableReceivables, userID, demoTableAccounts, userID, demoTableReceivables, userID, demoTableAccounts}},
		{"SELECT COUNT(*) FROM fixed_deposit_details WHERE linked_account_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, demoTableAccounts, userID, demoTableFixedDeposit}},
	}
	for _, c := range checks {
		var n int64
		if err := tx.Raw(c.sql, c.args...).Scan(&n).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if n > 0 {
			return apperrors.ErrDemoInUse
		}
	}
	return nil
}

// detachRealRows clears references from real rows to sample rows that can be
// cleared without losing data: a category on the user's own transaction or rule
// action, and a default account.
func (s *demoService) detachRealRows(tx *gorm.DB, userID string) error {
	stmts := []struct {
		sql  string
		args []any
	}{
		{"UPDATE transactions SET category_id = NULL WHERE user_id = ? AND category_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, userID, demoTableCategories, userID, demoTableTransactions}},
		{"UPDATE transaction_rule_actions SET category_id = NULL WHERE category_id IN " + demoSubquery + " AND id NOT IN " + demoSubquery,
			[]any{userID, demoTableCategories, userID, demoTableActions}},
		{"UPDATE users SET default_account_id = NULL WHERE id = ? AND default_account_id IN " + demoSubquery,
			[]any{userID, userID, demoTableAccounts}},
	}
	for _, st := range stmts {
		if err := tx.Exec(st.sql, st.args...).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return nil
}

// demoUserScoped lists the demo tables that carry a user_id column.
var demoUserScoped = map[string]bool{
	demoTableAccounts: true, demoTableCategories: true, demoTableTransactions: true,
	demoTableBudgets: true, demoTableReceivables: true, demoTableRules: true,
	demoTableSettlements: true,
}

// deleteDemoTable hard-deletes the given ids from table and returns how many
// rows went. A security is only deleted (with its prices) when no holding of
// anyone refers to it, because the sample securities are shared.
func deleteDemoTable(tx *gorm.DB, userID, table string, ids []string) (int, error) {
	total := 0
	for start := 0; start < len(ids); start += demoDeleteChunk {
		end := start + demoDeleteChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		if table == demoTableSecurities {
			free, err := unreferencedSecurities(tx, chunk)
			if err != nil {
				return total, err
			}
			if len(free) == 0 {
				continue
			}
			if err := tx.Exec("DELETE FROM security_prices WHERE security_id IN ?", free).Error; err != nil {
				return total, apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			chunk = free
		}
		// table comes from the fixed demoDeleteOrder list, never from input.
		q := "DELETE FROM " + table + " WHERE id IN ?"
		args := []any{chunk}
		if demoUserScoped[table] {
			q += " AND user_id = ?"
			args = append(args, userID)
		}
		res := tx.Exec(q, args...)
		if res.Error != nil {
			return total, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
		}
		total += int(res.RowsAffected)
	}
	return total, nil
}

// sweepDemoSecurities deletes sample securities (exchange "DEMO-<currency>")
// that no holding refers to any more. They are shared between users, so the
// last household to go takes them with it, whoever created them.
func sweepDemoSecurities(tx *gorm.DB) error {
	var ids []string
	if err := tx.Raw(`SELECT id FROM securities s WHERE s.exchange LIKE 'DEMO-%'
		AND NOT EXISTS (SELECT 1 FROM investments i WHERE i.security_id = s.id)`).Scan(&ids).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for start := 0; start < len(ids); start += demoDeleteChunk {
		chunk := ids[start:min(start+demoDeleteChunk, len(ids))]
		if err := tx.Exec("DELETE FROM security_prices WHERE security_id IN ?", chunk).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if err := tx.Exec("DELETE FROM securities WHERE id IN ?", chunk).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return nil
}

// unreferencedSecurities returns the ids no investment (even a deleted one) uses.
func unreferencedSecurities(tx *gorm.DB, ids []string) ([]string, error) {
	var used []string
	if err := tx.Raw("SELECT DISTINCT security_id FROM investments WHERE security_id IN ?", ids).Scan(&used).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	inUse := make(map[string]bool, len(used))
	for _, u := range used {
		inUse[u] = true
	}
	var free []string
	for _, id := range ids {
		if !inUse[id] {
			free = append(free, id)
		}
	}
	return free, nil
}

// clearAccountOrphans removes per-account rows left behind by the hard delete
// of sample accounts: reminders (ref_id has no foreign key) and daily balance
// snapshots (cascaded by the foreign key on Postgres, but not on SQLite).
func clearAccountOrphans(tx *gorm.DB, userID string) error {
	live := tx.Unscoped().Model(&models.Account{}).Select("id").Where("user_id = ?", userID)
	if err := tx.Unscoped().Where("user_id = ? AND ref_id NOT IN (?)", userID, live).
		Delete(&models.Reminder{}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if err := tx.Where("user_id = ? AND account_id NOT IN (?)", userID, live).
		Delete(&models.AccountBalanceSnapshot{}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// clearEmptySnapshots drops the user's portfolio history once they have no
// accounts left: it only described the sample household.
func clearEmptySnapshots(tx *gorm.DB, userID string) error {
	var n int64
	if err := tx.Unscoped().Model(&models.Account{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if n > 0 {
		return nil
	}
	if err := tx.Exec("DELETE FROM portfolio_snapshots WHERE user_id = ?", userID).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// demoAccountCount returns how many of the given accounts belong to the sample
// household.
func demoAccountCount(db *gorm.DB, userID string, accountIDs ...string) (int, error) {
	var n int64
	if err := db.Model(&models.DemoRow{}).
		Where("user_id = ? AND table_name = ? AND row_id IN ?", userID, demoTableAccounts, accountIDs).
		Count(&n).Error; err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return int(n), nil
}

// guardDemoTransfer refuses a transfer between a sample account and a real one,
// because removing the sample data would leave the real balance wrong.
func guardDemoTransfer(db *gorm.DB, userID, fromID, toID string) error {
	n, err := demoAccountCount(db, userID, fromID, toID)
	if err != nil {
		return err
	}
	if n == 1 {
		return apperrors.ErrDemoTransferBlocked
	}
	return nil
}

// demoBuilder writes the sample household through tx.
type demoBuilder struct {
	db     *gorm.DB
	userID string
	base   string
	rec    *demoRecorder
	now    time.Time
	fxs    FXServicer

	cats  map[string]*models.Category
	accts map[string]*models.Account
	bal   map[string]int64
	txns  map[string]*models.Transaction // by demoTxn.key
}

func (b *demoBuilder) fail(err error) error {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return err
	}
	return apperrors.Wrap(apperrors.ErrInternalServer, err)
}

func (b *demoBuilder) build() error {
	today := time.Date(b.now.Year(), b.now.Month(), b.now.Day(), 12, 0, 0, 0, time.UTC)
	withFX := b.hasForeignRate()
	if err := b.createCategories(); err != nil {
		return err
	}
	if err := b.createAccounts(today, withFX); err != nil {
		return err
	}
	if err := b.createTransactions(today, withFX); err != nil {
		return err
	}
	if err := b.createReceivables(); err != nil {
		return err
	}
	if err := b.createBudgets(); err != nil {
		return err
	}
	if err := b.createRules(); err != nil {
		return err
	}
	if err := b.createHoldings(today); err != nil {
		return err
	}
	return b.writeBalances()
}

// foreignCurrency is the currency of the optional second-currency account.
func (b *demoBuilder) foreignCurrency() string {
	if b.base == "USD" {
		return "EUR"
	}
	return "USD"
}

// hasForeignRate reports whether the foreign account can be valued, so the
// sample never shows an account without a base-currency figure.
func (b *demoBuilder) hasForeignRate() bool {
	if b.fxs == nil {
		return false
	}
	_, err := b.fxs.Rate(b.foreignCurrency(), b.base, b.now)
	return err == nil
}

func (b *demoBuilder) createCategories() error {
	b.cats = map[string]*models.Category{}
	for _, spec := range demoCategories {
		var found []models.Category
		if err := b.db.Where("user_id = ? AND name = ? AND type = ?", b.userID, spec.Name, spec.Type).
			Limit(1).Find(&found).Error; err != nil {
			return b.fail(err)
		}
		if len(found) > 0 {
			b.cats[spec.Name] = &found[0]
			continue
		}
		c := &models.Category{UserID: b.userID, Name: spec.Name, Type: spec.Type, Icon: spec.Icon, Color: spec.Color}
		if err := b.db.Create(c).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableCategories, c.ID)
		b.cats[spec.Name] = c
	}
	return nil
}

func (b *demoBuilder) createAccounts(today time.Time, withFX bool) error {
	b.accts = map[string]*models.Account{}
	b.bal = map[string]int64{}
	fdStart := today.AddDate(0, -7, 0)
	openDate := fdStart.AddDate(0, 0, -1)
	dueDay := time.Date(today.Year(), today.Month(), 12, 0, 0, 0, 0, time.UTC)
	if !dueDay.After(today) {
		dueDay = dueDay.AddDate(0, 1, 0)
	}

	specs := []struct {
		key     string
		acct    models.Account
		opening int64
	}{
		{demoAcctDBS, models.Account{Name: "DBS Everyday", Type: models.AccountTypeCash, Description: "Salary and daily spending", Currency: b.base}, cents(18500)},
		{demoAcctHYS, models.Account{Name: "High-Yield Savings", Type: models.AccountTypeCash, Description: "3.10% p.a.", Currency: b.base, InterestRate: 3.10}, cents(36000)},
		{demoAcctFD, models.Account{Name: "Maybank FD 12M", Type: models.AccountTypeFixedDeposit, Currency: b.base, InterestRate: 3.5}, 0},
		{demoAcctIBKR, models.Account{Name: "IBKR Brokerage", Type: models.AccountTypeInvestment, Currency: b.base, Broker: "Interactive Brokers"}, 0},
		{demoAcctAmex, models.Account{Name: "Amex Gold", Type: models.AccountTypeCreditCard, Currency: b.base, CreditLimit: cents(15000), InterestRate: 24.99, DueDate: dueDay}, cents(6000)},
	}
	if withFX {
		specs = append(specs, struct {
			key     string
			acct    models.Account
			opening int64
		}{demoAcctFX, models.Account{Name: "Travel Wallet", Type: models.AccountTypeCash, Description: "Foreign-currency spending", Currency: b.foreignCurrency()}, cents(1500)})
	}
	for i := range specs {
		sp := &specs[i]
		a := sp.acct
		a.UserID = b.userID
		a.IsActive = true
		if err := b.db.Create(&a).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableAccounts, a.ID)
		b.accts[sp.key] = &a
		if sp.opening > 0 {
			b.bal[a.ID] += sp.opening
			if err := b.insertTxn(&models.Transaction{
				UserID: b.userID, AccountID: a.ID, Type: models.TransactionTypeOpeningBalance,
				Amount: sp.opening, Description: "Initial balance", Date: openDate, Source: models.TransactionSourceSystem,
			}); err != nil {
				return err
			}
		}
	}

	// The fixed deposit is funded from savings, like CreateFixedDepositAccount.
	const fdPrincipal = int64(2000000)
	fd := b.accts[demoAcctFD]
	hys := b.accts[demoAcctHYS]
	fdTransfer := &models.Transaction{
		UserID: b.userID, AccountID: hys.ID, ToAccountID: &fd.ID, Type: models.TransactionTypeTransfer,
		Amount: fdPrincipal, Description: "Fixed deposit: " + fd.Name, Date: fdStart, Source: models.TransactionSourceWeb,
	}
	if err := b.insertTxn(fdTransfer); err != nil {
		return err
	}
	b.applyTransfer(hys, fd, fdPrincipal, fdPrincipal)

	maturity := fdStart.AddDate(1, 0, 0)
	comp := models.CompoundingMonthly
	details := &models.FixedDepositDetails{
		AccountID: fd.ID, Principal: fdPrincipal, StartDate: fdStart, MaturityDate: maturity,
		PayoutType: models.PayoutTypeCumulative, Compounding: comp,
		MaturityValue: fdmath.MaturityValue(fdPrincipal, fd.InterestRate, fdStart, maturity, compoundingPeriods(comp)),
		Status:        models.FixedDepositStatusActive,
	}
	if err := b.db.Create(details).Error; err != nil {
		return b.fail(err)
	}
	b.rec.add(demoTableFixedDeposit, details.ID)
	return nil
}

// insertTxn writes one transaction row and lists it.
func (b *demoBuilder) insertTxn(t *models.Transaction) error {
	if err := b.db.Create(t).Error; err != nil {
		return b.fail(err)
	}
	b.rec.add(demoTableTransactions, t.ID)
	return nil
}

func (b *demoBuilder) applyDelta(a *models.Account, typ models.TransactionType, amount int64) {
	if d, ok := balanceDelta(a.Type, typ, amount); ok {
		b.bal[a.ID] += d
	}
}

func (b *demoBuilder) applyTransfer(from, to *models.Account, amount, toAmount int64) {
	b.applyDelta(from, models.TransactionTypeExpense, amount)
	b.applyDelta(to, models.TransactionTypeIncome, toAmount)
}

func (b *demoBuilder) createTransactions(today time.Time, withFX bool) error {
	specs := demoTransactions(today, withFX)
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].daysAgo > specs[j].daysAgo })
	b.txns = map[string]*models.Transaction{}
	for i := range specs {
		sp := &specs[i]
		acct := b.accts[sp.acct]
		if acct == nil {
			continue
		}
		t := &models.Transaction{
			UserID: b.userID, AccountID: acct.ID, Type: sp.typ, Amount: sp.amount,
			Description: sp.desc, Date: today.AddDate(0, 0, -sp.daysAgo), Notes: sp.notes, Source: sp.source,
		}
		if sp.category != "" {
			if c := b.cats[sp.category]; c != nil {
				id := c.ID
				t.CategoryID = &id
			}
		}
		if sp.typ == models.TransactionTypeTransfer {
			to := b.accts[sp.toAcct]
			if to == nil {
				continue
			}
			t.ToAccountID = &to.ID
			b.applyTransfer(acct, to, sp.amount, sp.amount)
		} else {
			b.applyDelta(acct, sp.typ, sp.amount)
		}
		if err := b.insertTxn(t); err != nil {
			return err
		}
		if sp.key != "" {
			b.txns[sp.key] = t
		}
	}
	return nil
}

func (b *demoBuilder) createReceivables() error {
	type spec struct {
		txKey  string
		who    string
		kind   models.ReceivableKind
		amount int64
		paid   int64
		paidTx string
	}
	for _, sp := range []spec{
		{"arjun", "Arjun", models.ReceivableKindLoan, cents(400), 0, ""},
		{"coldplay", "Priya", models.ReceivableKindSplit, cents(120), cents(60), "priya-paid"},
		{"traderjoes", "Wei Ling", models.ReceivableKindSplit, cents(32.37), 0, ""},
	} {
		exp := b.txns[sp.txKey]
		if exp == nil {
			continue
		}
		r := &models.Receivable{
			UserID: b.userID, TransactionID: exp.ID, CounterpartyName: sp.who,
			CounterpartyKey: models.NormalizeCounterpartyKey(sp.who), Kind: sp.kind,
			Amount: sp.amount, PaidAmount: sp.paid,
		}
		r.Status = r.DeriveStatus()
		if err := b.db.Create(r).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableReceivables, r.ID)
		if err := recomputePersonalAmount(b.db, b.userID, exp.ID); err != nil {
			return err
		}
		if sp.paid > 0 {
			pay := b.txns[sp.paidTx]
			if pay == nil {
				continue
			}
			s := &models.ReceivableSettlement{
				UserID: b.userID, ReceivableID: r.ID, TransactionID: pay.ID,
				AccountID: pay.AccountID, Amount: sp.paid, Date: pay.Date,
			}
			if err := b.db.Create(s).Error; err != nil {
				return b.fail(err)
			}
			b.rec.add(demoTableSettlements, s.ID)
		}
	}
	return nil
}

func (b *demoBuilder) createBudgets() error {
	for _, sp := range demoBudgets {
		c := b.cats[sp.category]
		if c == nil {
			continue
		}
		bud := &models.Budget{
			UserID: b.userID, CategoryID: c.ID, Name: sp.category, Amount: cents(sp.amount),
			Period: models.BudgetPeriodMonthly, IsActive: true, Currency: b.base,
		}
		if err := b.db.Create(bud).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableBudgets, bud.ID)
	}
	return nil
}

func (b *demoBuilder) createRules() error {
	for i, sp := range []struct{ name, text, category string }{
		{"Whole Foods is Groceries", "Whole Foods", "Groceries"},
		{"Payroll is Salary", "payroll", "Salary"},
		{"Spotify is Subscriptions", "Spotify", "Subscriptions"},
	} {
		c := b.cats[sp.category]
		if c == nil {
			continue
		}
		rule := &models.TransactionRule{UserID: b.userID, Name: sp.name, Priority: i, IsActive: true}
		if err := b.db.Omit("Conditions", "Actions").Create(rule).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableRules, rule.ID)
		cond := &models.TransactionRuleCondition{RuleID: rule.ID, Field: models.RuleFieldDescription, Operator: models.RuleOpContains, ValueText: sp.text}
		if err := b.db.Create(cond).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableConditions, cond.ID)
		catID := c.ID
		act := &models.TransactionRuleAction{RuleID: rule.ID, ActionType: models.RuleActionSetCategory, CategoryID: &catID}
		if err := b.db.Omit("Category").Create(act).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableActions, act.ID)
	}
	return nil
}

// demoSecurityExchange keeps the shared sample securities apart from real ones
// and per currency (a security's currency is fixed once it has trades).
func (b *demoBuilder) demoSecurityExchange() string { return "DEMO-" + b.base }

func (b *demoBuilder) createHoldings(today time.Time) error {
	ibkr := b.accts[demoAcctIBKR]
	priceTime := b.now.UTC().Truncate(time.Hour)
	buyDate := today.AddDate(0, -7, 0)
	for idx, h := range demoHoldings {
		sec, created, err := b.findOrCreateSecurity(h)
		if err != nil {
			return err
		}
		if created {
			for k, p := range demoPrices(h, idx, 16) {
				row := &models.SecurityPrice{SecurityID: sec.ID, Price: p, Currency: b.base, RecordedAt: priceTime.AddDate(0, 0, -7*k)}
				if err := b.db.Create(row).Error; err != nil {
					return b.fail(err)
				}
			}
		}
		unit := cents(h.cost)
		total, err := lineAmount(h.qty, unit)
		if err != nil {
			return err
		}
		inv := &models.Investment{
			AccountID: ibkr.ID, SecurityID: sec.ID, Quantity: h.qty,
			CostBasis: total, CostBasisBase: total,
		}
		if err := b.db.Omit("Security", "Account", "Transactions").Create(inv).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableInvestments, inv.ID)
		rate := identityRate
		base := total
		it := &models.InvestmentTransaction{
			InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: buyDate,
			Quantity: h.qty, PricePerUnit: unit, TotalAmount: total,
			FXRateToBase: &rate, BaseAmount: &base,
		}
		if err := b.db.Omit("Investment").Create(it).Error; err != nil {
			return b.fail(err)
		}
		b.rec.add(demoTableInvTx, it.ID)
	}
	return nil
}

func (b *demoBuilder) findOrCreateSecurity(h demoHolding) (*models.Security, bool, error) {
	var found []models.Security
	if err := b.db.Where("symbol = ? AND exchange = ?", h.symbol, b.demoSecurityExchange()).
		Limit(1).Find(&found).Error; err != nil {
		return nil, false, b.fail(err)
	}
	if len(found) > 0 {
		return &found[0], false, nil
	}
	sec := models.Security{Symbol: h.symbol, Name: h.name, AssetType: h.asset, Currency: b.base, Exchange: b.demoSecurityExchange()}
	if err := b.db.Create(&sec).Error; err != nil {
		return nil, false, b.fail(err)
	}
	b.rec.add(demoTableSecurities, sec.ID)
	return &sec, true, nil
}

// writeBalances stores the balances the generated ledger adds up to. The
// investment account's balance is its market value, worked out when read.
func (b *demoBuilder) writeBalances() error {
	for _, a := range b.accts {
		if a.Type == models.AccountTypeInvestment {
			continue
		}
		if err := b.db.Model(&models.Account{}).Where("id = ?", a.ID).UpdateColumn("balance", b.bal[a.ID]).Error; err != nil {
			return b.fail(err)
		}
	}
	return nil
}
