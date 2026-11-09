package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
)

// exportBatchSize is how many rows are loaded at a time while streaming a table.
const exportBatchSize = 500

// ExportServicer streams a user's data as a zip of CSV files.
type ExportServicer interface {
	// WriteExport writes a zip archive with one CSV per table to w, scoped to
	// userID. Soft-deleted rows are included with their deleted_at. It writes as
	// it reads, so memory stays flat however large the data is.
	WriteExport(userID string, w io.Writer) error
}

type exportService struct {
	db  *gorm.DB
	now func() time.Time
}

// NewExportService creates an ExportServicer.
func NewExportService(db *gorm.DB) ExportServicer {
	return &exportService{db: db, now: time.Now}
}

// exportEmit appends one CSV record to the table being written.
type exportEmit func(fields ...string) error

// WriteExport implements ExportServicer.
func (s *exportService) WriteExport(userID string, w io.Writer) error {
	var user models.User
	if err := s.db.Select("id", "base_currency").First(&user, "id = ?", userID).Error; err != nil {
		return fmt.Errorf("load user for export: %w", err)
	}

	zw := zip.NewWriter(w)

	accounts, err := s.loadAccounts(userID)
	if err != nil {
		return err
	}
	accountCurrency := make(map[string]string, len(accounts))
	for i := range accounts {
		accountCurrency[accounts[i].ID] = accounts[i].Currency
	}
	holdings, securities, err := s.loadHoldings(userID)
	if err != nil {
		return err
	}
	securityCurrency := make(map[string]string, len(securities))
	for i := range securities {
		securityCurrency[securities[i].ID] = securities[i].Currency
	}
	holdingCurrency := make(map[string]string, len(holdings))
	for i := range holdings {
		holdingCurrency[holdings[i].ID] = securityCurrency[holdings[i].SecurityID]
	}

	steps := []struct {
		name string
		run  func(emit exportEmit) error
		head []string
	}{
		{"accounts.csv", func(emit exportEmit) error { return s.accountRows(accounts, emit) }, accountsHeader},
		{"categories.csv", func(emit exportEmit) error { return s.categoryRows(userID, emit) }, categoriesHeader},
		{"transactions.csv", func(emit exportEmit) error { return s.transactionRows(userID, accountCurrency, emit) }, transactionsHeader},
		{"budgets.csv", func(emit exportEmit) error { return s.budgetRows(userID, emit) }, budgetsHeader},
		{"investments.csv", func(emit exportEmit) error {
			return s.investmentRows(holdings, securityCurrency, user.BaseCurrency, emit)
		}, investmentsHeader},
		{"investment_transactions.csv", func(emit exportEmit) error {
			return s.investmentTransactionRows(userID, holdingCurrency, user.BaseCurrency, emit)
		}, investmentTransactionsHeader},
		{"securities.csv", func(emit exportEmit) error { return s.securityRows(securities, emit) }, securitiesHeader},
		{"receivables.csv", func(emit exportEmit) error { return s.receivableRows(userID, emit) }, receivablesHeader},
		{"receivable_settlements.csv", func(emit exportEmit) error { return s.settlementRows(userID, emit) }, settlementsHeader},
		{"rules.csv", func(emit exportEmit) error { return s.ruleRows(userID, emit) }, rulesHeader},
	}
	for _, st := range steps {
		if err := s.writeTable(zw, st.name, st.head, st.run); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close export zip: %w", err)
	}
	return nil
}

// writeTable adds one CSV entry to the zip and fills it through run.
func (s *exportService) writeTable(zw *zip.Writer, name string, header []string, run func(emit exportEmit) error) error {
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: s.now().UTC()})
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	cw := csv.NewWriter(fw)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write %s header: %w", name, err)
	}
	emit := func(fields ...string) error {
		if len(fields) != len(header) {
			return fmt.Errorf("%s: row has %d fields, header has %d", name, len(fields), len(header))
		}
		return cw.Write(fields)
	}
	if err := run(emit); err != nil {
		return fmt.Errorf("export %s: %w", name, err)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush %s: %w", name, err)
	}
	return nil
}

// ---- formatting helpers ----

// exportCents formats an int64 amount of cents as a plain decimal ("-12.50").
// The currency has its own column; Kuberan only supports 2-decimal currencies.
func exportCents(n int64) string {
	neg := n < 0
	u := uint64(n)
	if neg {
		u = -u
	}
	s := fmt.Sprintf("%d.%02d", u/100, u%100)
	if neg {
		return "-" + s
	}
	return s
}

func exportInt(n int64) string { return strconv.FormatInt(n, 10) }

func exportOptInt(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}

func exportOptCents(n *int64) string {
	if n == nil {
		return ""
	}
	return exportCents(*n)
}

func exportFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func exportBool(b bool) string { return strconv.FormatBool(b) }

func exportTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func exportOptTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return exportTime(*t)
}

func exportDeleted(d gorm.DeletedAt) string {
	if !d.Valid {
		return ""
	}
	return exportTime(d.Time)
}

func exportOptStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// exportText neutralises spreadsheet formula injection in user-entered text:
// a cell that starts with =, +, - or @ (or a control character a spreadsheet
// treats as a prefix) is prefixed with an apostrophe.
func exportText(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// ---- loaders for the small reference tables kept in memory ----

func (s *exportService) loadAccounts(userID string) ([]models.Account, error) {
	var accounts []models.Account
	if err := s.db.Unscoped().Where("user_id = ?", userID).Order("id").Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("load accounts for export: %w", err)
	}
	return accounts, nil
}

// loadHoldings returns the user's holdings (incl. soft-deleted) and the
// securities they reference.
func (s *exportService) loadHoldings(userID string) ([]models.Investment, []models.Security, error) {
	accountIDs := s.db.Unscoped().Model(&models.Account{}).Select("id").Where("user_id = ?", userID)
	var holdings []models.Investment
	if err := s.db.Unscoped().Where("account_id IN (?)", accountIDs).Order("id").Find(&holdings).Error; err != nil {
		return nil, nil, fmt.Errorf("load investments for export: %w", err)
	}
	seen := make(map[string]struct{}, len(holdings))
	ids := make([]string, 0, len(holdings))
	for i := range holdings {
		if _, ok := seen[holdings[i].SecurityID]; !ok {
			seen[holdings[i].SecurityID] = struct{}{}
			ids = append(ids, holdings[i].SecurityID)
		}
	}
	var securities []models.Security
	if len(ids) > 0 {
		if err := s.db.Unscoped().Where("id IN ?", ids).Order("id").Find(&securities).Error; err != nil {
			return nil, nil, fmt.Errorf("load securities for export: %w", err)
		}
	}
	return holdings, securities, nil
}

// ---- tables ----

var accountsHeader = []string{
	"id", "name", "type", "description", "currency", "balance", "balance_formatted",
	"is_active", "is_pinned", "broker", "account_number", "interest_rate",
	"due_date", "credit_limit", "credit_limit_formatted",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) accountRows(accounts []models.Account, emit exportEmit) error {
	for i := range accounts {
		a := &accounts[i]
		if err := emit(a.ID, exportText(a.Name), string(a.Type), exportText(a.Description), a.Currency,
			exportInt(a.Balance), exportCents(a.Balance),
			exportBool(a.IsActive), exportBool(a.IsPinned), exportText(a.Broker), exportText(a.AccountNumber),
			exportFloat(a.InterestRate), exportTime(a.DueDate), exportInt(a.CreditLimit), exportCents(a.CreditLimit),
			exportTime(a.CreatedAt), exportTime(a.UpdatedAt), exportDeleted(a.DeletedAt)); err != nil {
			return err
		}
	}
	return nil
}

var categoriesHeader = []string{
	"id", "name", "type", "description", "icon", "color", "parent_id",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) categoryRows(userID string, emit exportEmit) error {
	var batch []models.Category
	return s.db.Unscoped().Where("user_id = ?", userID).FindInBatches(&batch, exportBatchSize, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			c := &batch[i]
			if err := emit(c.ID, exportText(c.Name), string(c.Type), exportText(c.Description), c.Icon, c.Color,
				exportOptStr(c.ParentID), exportTime(c.CreatedAt), exportTime(c.UpdatedAt), exportDeleted(c.DeletedAt)); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

var transactionsHeader = []string{
	"id", "account_id", "type", "date", "description", "category_id",
	"amount", "amount_formatted", "currency",
	"to_account_id", "to_amount", "to_amount_formatted", "to_currency",
	"personal_amount", "personal_amount_formatted", "import_ref",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) transactionRows(userID string, accountCurrency map[string]string, emit exportEmit) error {
	var batch []models.Transaction
	return s.db.Unscoped().Where("user_id = ?", userID).FindInBatches(&batch, exportBatchSize, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			t := &batch[i]
			toCurrency, toAmount, toAmountFmt := "", "", ""
			if t.ToAccountID != nil {
				toCurrency = accountCurrency[*t.ToAccountID]
				toAmount = exportInt(t.EffectiveToAmount())
				toAmountFmt = exportCents(t.EffectiveToAmount())
			}
			if err := emit(t.ID, t.AccountID, string(t.Type), exportTime(t.Date), exportText(t.Description), exportOptStr(t.CategoryID),
				exportInt(t.Amount), exportCents(t.Amount), accountCurrency[t.AccountID],
				exportOptStr(t.ToAccountID), toAmount, toAmountFmt, toCurrency,
				exportOptInt(t.PersonalAmount), exportOptCents(t.PersonalAmount), exportOptStr(t.ImportRef),
				exportTime(t.CreatedAt), exportTime(t.UpdatedAt), exportDeleted(t.DeletedAt)); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

var budgetsHeader = []string{
	"id", "name", "category_id", "period", "amount", "amount_formatted", "currency", "is_active",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) budgetRows(userID string, emit exportEmit) error {
	var batch []models.Budget
	return s.db.Unscoped().Where("user_id = ?", userID).FindInBatches(&batch, exportBatchSize, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			b := &batch[i]
			if err := emit(b.ID, exportText(b.Name), b.CategoryID, string(b.Period),
				exportInt(b.Amount), exportCents(b.Amount), b.Currency, exportBool(b.IsActive),
				exportTime(b.CreatedAt), exportTime(b.UpdatedAt), exportDeleted(b.DeletedAt)); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

var investmentsHeader = []string{
	"id", "account_id", "security_id", "quantity", "currency",
	"cost_basis", "cost_basis_formatted", "realized_gain_loss", "realized_gain_loss_formatted",
	"base_currency", "cost_basis_base", "cost_basis_base_formatted",
	"realized_gain_loss_base", "realized_gain_loss_base_formatted",
	"wallet_address", "created_at", "updated_at", "deleted_at",
}

func (s *exportService) investmentRows(holdings []models.Investment, securityCurrency map[string]string, baseCurrency string, emit exportEmit) error {
	for i := range holdings {
		h := &holdings[i]
		if err := emit(h.ID, h.AccountID, h.SecurityID, exportFloat(h.Quantity), securityCurrency[h.SecurityID],
			exportInt(h.CostBasis), exportCents(h.CostBasis), exportInt(h.RealizedGainLoss), exportCents(h.RealizedGainLoss),
			baseCurrency, exportInt(h.CostBasisBase), exportCents(h.CostBasisBase),
			exportInt(h.RealizedGainLossBase), exportCents(h.RealizedGainLossBase),
			exportText(h.WalletAddress), exportTime(h.CreatedAt), exportTime(h.UpdatedAt), exportDeleted(h.DeletedAt)); err != nil {
			return err
		}
	}
	return nil
}

var investmentTransactionsHeader = []string{
	"id", "investment_id", "type", "date", "quantity", "currency",
	"price_per_unit", "price_per_unit_formatted", "total_amount", "total_amount_formatted",
	"fee", "fee_formatted", "realized_gain_loss", "realized_gain_loss_formatted",
	"split_ratio", "dividend_type", "notes",
	"base_currency", "fx_rate_to_base", "base_amount", "base_amount_formatted",
	"realized_gain_loss_base", "realized_gain_loss_base_formatted",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) investmentTransactionRows(userID string, holdingCurrency map[string]string, baseCurrency string, emit exportEmit) error {
	accountIDs := s.db.Unscoped().Model(&models.Account{}).Select("id").Where("user_id = ?", userID)
	holdingIDs := s.db.Unscoped().Model(&models.Investment{}).Select("id").Where("account_id IN (?)", accountIDs)
	var batch []models.InvestmentTransaction
	return s.db.Unscoped().Where("investment_id IN (?)", holdingIDs).FindInBatches(&batch, exportBatchSize, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			t := &batch[i]
			rate := ""
			if t.FXRateToBase != nil {
				rate = t.FXRateToBase.String()
			}
			if err := emit(t.ID, t.InvestmentID, string(t.Type), exportTime(t.Date), exportFloat(t.Quantity), holdingCurrency[t.InvestmentID],
				exportInt(t.PricePerUnit), exportCents(t.PricePerUnit), exportInt(t.TotalAmount), exportCents(t.TotalAmount),
				exportInt(t.Fee), exportCents(t.Fee), exportInt(t.RealizedGainLoss), exportCents(t.RealizedGainLoss),
				exportFloat(t.SplitRatio), t.DividendType, exportText(t.Notes),
				baseCurrency, rate, exportOptInt(t.BaseAmount), exportOptCents(t.BaseAmount),
				exportInt(t.RealizedGainLossBase), exportCents(t.RealizedGainLossBase),
				exportTime(t.CreatedAt), exportTime(t.UpdatedAt), exportDeleted(t.DeletedAt)); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

var securitiesHeader = []string{
	"id", "symbol", "name", "asset_type", "currency", "exchange", "provider_symbol",
	"maturity_date", "yield_to_maturity", "coupon_rate", "network", "property_type",
	"created_at", "updated_at", "deleted_at",
}

func (s *exportService) securityRows(securities []models.Security, emit exportEmit) error {
	for i := range securities {
		sec := &securities[i]
		if err := emit(sec.ID, exportText(sec.Symbol), exportText(sec.Name), string(sec.AssetType), sec.Currency,
			sec.Exchange, sec.ProviderSymbol, exportOptTime(sec.MaturityDate),
			exportFloat(sec.YieldToMaturity), exportFloat(sec.CouponRate), sec.Network, sec.PropertyType,
			exportTime(sec.CreatedAt), exportTime(sec.UpdatedAt), exportDeleted(sec.DeletedAt)); err != nil {
			return err
		}
	}
	return nil
}

var receivablesHeader = []string{
	"id", "transaction_id", "counterparty_name", "kind", "status", "currency",
	"amount", "amount_formatted", "paid_amount", "paid_amount_formatted",
	"written_off_amount", "written_off_amount_formatted", "outstanding", "outstanding_formatted",
	"created_at", "updated_at", "deleted_at",
}

// receivableExportRow is a receivable joined with the currency of its expense
// transaction's account (a receivable has no currency column of its own).
type receivableExportRow struct {
	ID               string
	TransactionID    string
	CounterpartyName string
	Kind             string
	Status           string
	Currency         string
	Amount           int64
	PaidAmount       int64
	WrittenOffAmount int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
}

func (s *exportService) receivableRows(userID string, emit exportEmit) error {
	rows, err := s.db.Table("receivables AS r").
		Select("r.id, r.transaction_id, r.counterparty_name, r.kind, r.status, a.currency AS currency, "+
			"r.amount, r.paid_amount, r.written_off_amount, r.created_at, r.updated_at, r.deleted_at").
		Joins("LEFT JOIN transactions t ON t.id = r.transaction_id").
		Joins("LEFT JOIN accounts a ON a.id = t.account_id").
		Where("r.user_id = ?", userID).Order("r.id").Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r receivableExportRow
		if err := s.db.ScanRows(rows, &r); err != nil {
			return err
		}
		out := r.Amount - r.PaidAmount - r.WrittenOffAmount
		if err := emit(r.ID, r.TransactionID, exportText(r.CounterpartyName), r.Kind, r.Status, r.Currency,
			exportInt(r.Amount), exportCents(r.Amount), exportInt(r.PaidAmount), exportCents(r.PaidAmount),
			exportInt(r.WrittenOffAmount), exportCents(r.WrittenOffAmount), exportInt(out), exportCents(out),
			exportTime(r.CreatedAt), exportTime(r.UpdatedAt), exportDeleted(r.DeletedAt)); err != nil {
			return err
		}
	}
	return rows.Err()
}

var settlementsHeader = []string{
	"id", "receivable_id", "transaction_id", "account_id", "date", "currency",
	"amount", "amount_formatted", "written_off_amount", "written_off_amount_formatted",
	"account_currency", "account_amount", "account_amount_formatted",
	"created_at", "updated_at", "deleted_at",
}

// settlementExportRow is a settlement joined with the receivable's currency
// and the currency of the account that received the payment.
type settlementExportRow struct {
	ID               string
	ReceivableID     string
	TransactionID    string
	AccountID        string
	Date             time.Time
	Currency         string
	Amount           int64
	WrittenOffAmount int64
	AccountCurrency  string
	AccountAmount    *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
}

func (s *exportService) settlementRows(userID string, emit exportEmit) error {
	rows, err := s.db.Table("receivable_settlements AS s").
		Select("s.id, s.receivable_id, s.transaction_id, s.account_id, s.date, ra.currency AS currency, "+
			"s.amount, s.written_off_amount, sa.currency AS account_currency, s.account_amount, "+
			"s.created_at, s.updated_at, s.deleted_at").
		Joins("LEFT JOIN receivables r ON r.id = s.receivable_id").
		Joins("LEFT JOIN transactions t ON t.id = r.transaction_id").
		Joins("LEFT JOIN accounts ra ON ra.id = t.account_id").
		Joins("LEFT JOIN accounts sa ON sa.id = s.account_id").
		Where("s.user_id = ?", userID).Order("s.id").Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r settlementExportRow
		if err := s.db.ScanRows(rows, &r); err != nil {
			return err
		}
		acct := r.Amount
		if r.AccountAmount != nil {
			acct = *r.AccountAmount
		}
		if err := emit(r.ID, r.ReceivableID, r.TransactionID, r.AccountID, exportTime(r.Date), r.Currency,
			exportInt(r.Amount), exportCents(r.Amount), exportInt(r.WrittenOffAmount), exportCents(r.WrittenOffAmount),
			r.AccountCurrency, exportInt(acct), exportCents(acct),
			exportTime(r.CreatedAt), exportTime(r.UpdatedAt), exportDeleted(r.DeletedAt)); err != nil {
			return err
		}
	}
	return rows.Err()
}

var rulesHeader = []string{
	"id", "name", "priority", "is_active", "conditions", "actions",
	"created_at", "updated_at", "deleted_at",
}

// ruleCondition and ruleAction are the JSON shapes of a rule's children inside
// the rules.csv "conditions" and "actions" columns.
type ruleCondition struct {
	Field     string `json:"field"`
	Operator  string `json:"operator"`
	ValueText string `json:"value_text,omitempty"`
	AmountMin *int64 `json:"amount_min,omitempty"`
	AmountMax *int64 `json:"amount_max,omitempty"`
}

type ruleAction struct {
	ActionType string `json:"action_type"`
	CategoryID string `json:"category_id,omitempty"`
	ValueText  string `json:"value_text,omitempty"`
}

func (s *exportService) ruleRows(userID string, emit exportEmit) error {
	var batch []models.TransactionRule
	q := s.db.Unscoped().Where("user_id = ?", userID).
		Preload("Conditions", func(tx *gorm.DB) *gorm.DB { return tx.Unscoped().Order("id") }).
		Preload("Actions", func(tx *gorm.DB) *gorm.DB { return tx.Unscoped().Order("id") })
	return q.FindInBatches(&batch, exportBatchSize, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			r := &batch[i]
			conds := make([]ruleCondition, 0, len(r.Conditions))
			for j := range r.Conditions {
				c := &r.Conditions[j]
				conds = append(conds, ruleCondition{Field: string(c.Field), Operator: string(c.Operator), ValueText: c.ValueText, AmountMin: c.AmountMin, AmountMax: c.AmountMax})
			}
			acts := make([]ruleAction, 0, len(r.Actions))
			for j := range r.Actions {
				a := &r.Actions[j]
				acts = append(acts, ruleAction{ActionType: string(a.ActionType), CategoryID: exportOptStr(a.CategoryID), ValueText: a.ValueText})
			}
			cj, err := json.Marshal(conds)
			if err != nil {
				return err
			}
			aj, err := json.Marshal(acts)
			if err != nil {
				return err
			}
			// JSON starts with "[", so the text guard is not needed here.
			if err := emit(r.ID, exportText(r.Name), strconv.Itoa(r.Priority), exportBool(r.IsActive), string(cj), string(aj),
				exportTime(r.CreatedAt), exportTime(r.UpdatedAt), exportDeleted(r.DeletedAt)); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

// ExportFileName returns the download file name for an export taken at t.
func ExportFileName(t time.Time) string {
	return "kuberan-export-" + t.UTC().Format("2006-01-02") + ".zip"
}
