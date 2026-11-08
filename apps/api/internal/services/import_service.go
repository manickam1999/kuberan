package services

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/importers/ibkr"
	"kuberan/internal/logger"
	"kuberan/internal/models"
)

const (
	// importMaxFiles bounds the statements in one import.
	importMaxFiles = 10
	// importDescPrefix starts the description of every cash row an import writes.
	importDescPrefix = "IBKR import: "
	// importQuantityTolerance is the largest accepted difference between a
	// replayed quantity and the broker's (IBKR prints 4 decimals).
	importQuantityTolerance = 1e-4
	// importDepositWindowDays is how far a legacy transfer may be from the
	// deposit it is matched to.
	importDepositWindowDays = 5
	// importCostWarnCents is the cost-basis difference above which the report
	// warns (smaller ones are per-row rounding).
	importCostWarnCents = 5
	// importRefMax mirrors the transactions.import_ref column.
	importRefMax = 128
	// importDateLayout is the layout of civil dates in the report.
	importDateLayout = "2006-01-02"
)

// Descriptions of the cash rows (display only; reversal never reads them).
const (
	importBuyDesc  = importDescPrefix + "Investment purchase: "
	importSellDesc = importDescPrefix + "Sale proceeds: "
	importDivDesc  = importDescPrefix + "Dividend: "
)

var ibkrAccountIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// importLocks holds one mutex per user so only one import is applied at a time.
var importLocks sync.Map

// IBKRImportParams are the inputs of an IBKR import.
type IBKRImportParams struct {
	// InvestmentAccountID is the investment account the holdings belong to.
	InvestmentAccountID string
	// CashAccountID is the cash account (in the statement base currency) that
	// receives the IBKR cash ledger.
	CashAccountID string
	// CashStartDate: cash legs are only written for events on or after it.
	CashStartDate time.Time
	// LegacyCashAccountID is the old IBKR cash account whose incoming
	// transfers are re-pointed at CashAccountID (optional).
	LegacyCashAccountID string
	// DeactivateLegacyCashAccount deactivates the legacy account after apply.
	DeactivateLegacyCashAccount bool
	// Apply writes the changes; false is a dry run.
	Apply bool
}

type importService struct {
	db             *gorm.DB
	accountService AccountServicer
	fxService      FXServicer
}

// NewImportService creates an ImportServicer. fxs supplies the trade-date FX
// rates; without it only trades in the user's base currency can be imported.
func NewImportService(db *gorm.DB, accountService AccountServicer, fxs FXServicer) ImportServicer {
	return &importService{db: db, accountService: accountService, fxService: fxs}
}

// symbolPlan is the plan for one symbol's holding.
type symbolPlan struct {
	symbol      string
	sec         *models.Security
	inv         *models.Investment // nil: created on apply
	existing    int64
	replaceIDs  []string // existing transactions the import soft-deletes (before the coverage end)
	rows        []models.InvestmentTransaction
	final       holdingState
	checkpoints []IBKRImportCheckpoint
}

// cashRow is one cash transaction to write.
type cashRow struct {
	ref    string
	typ    models.TransactionType // stored type
	effect models.TransactionType // balance effect: income or expense
	amount int64
	desc   string
	date   time.Time
}

// depositMatch is a deposit event paired with a transfer.
type depositMatch struct {
	ev       ibkr.Event
	tr       models.Transaction
	toAmount int64 // the new to_amount (the USD deposit)
	changed  bool  // false when the transfer already points at the cash account
}

type importPlan struct {
	userID  string
	params  IBKRImportParams
	set     *ibkr.EventSet
	base    string
	invAcct *models.Account
	cash    *models.Account
	legacy  *models.Account

	symbols []*symbolPlan

	opening     int64
	cashRows    []cashRow
	legs        int
	dividends   int
	fees        int
	matches     []depositMatch
	unmatched   []IBKRImportUnmatched
	cashChecks  []IBKRImportCashCheckpoint
	legacyAfter *int64

	errs     []IBKRImportIssue
	warnings []string

	fxMissing      int
	fxMissingFirst string
	fxMissingCcy   string
	fxStale        bool
	fxs            FXServicer
}

func (pl *importPlan) addError(code, format string, args ...any) {
	pl.errs = append(pl.errs, IBKRImportIssue{Code: code, Detail: fmt.Sprintf(format, args...)})
}

func (pl *importPlan) warn(format string, args ...any) {
	pl.warnings = append(pl.warnings, fmt.Sprintf(format, args...))
}

// blocked reports whether the plan must not be applied.
func (pl *importPlan) blocked() bool {
	if len(pl.errs) > 0 {
		return true
	}
	for _, sp := range pl.symbols {
		for _, c := range sp.checkpoints {
			if !c.QuantityOK {
				return true
			}
		}
	}
	return false
}

// ImportIBKR parses the statements, plans the import and, with p.Apply, writes
// it in one DB transaction. A dry run writes nothing. The uploads are only ever
// held in memory and neither their contents nor the account holder's name are
// logged.
func (s *importService) ImportIBKR(userID string, p IBKRImportParams, files []io.Reader) (*IBKRImportReport, error) {
	if p.InvestmentAccountID == "" || p.CashAccountID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "investment_account_id and cash_account_id are required")
	}
	if p.InvestmentAccountID == p.CashAccountID || (p.LegacyCashAccountID != "" && p.LegacyCashAccountID == p.CashAccountID) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the investment, cash and legacy cash accounts must be different accounts")
	}
	if p.CashStartDate.IsZero() {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "cash_start_date is required")
	}
	if len(files) == 0 || len(files) > importMaxFiles {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("upload between 1 and %d statement files", importMaxFiles))
	}

	stmts := make([]*ibkr.Statement, len(files))
	for i, f := range files {
		st, err := ibkr.Parse(f)
		if err != nil {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, fmt.Sprintf("file %d: %v", i+1, err))
		}
		stmts[i] = st
	}
	set, err := ibkr.BuildEvents(stmts)
	if err != nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error())
	}
	if !ibkrAccountIDPattern.MatchString(set.Account) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "the statements carry an unusable account id")
	}

	if !p.Apply {
		pl, err := s.plan(s.db, userID, p, set)
		if err != nil {
			return nil, err
		}
		s.logCounts(pl, files, false)
		return pl.report(false), nil
	}

	mu, _ := importLocks.LoadOrStore(userID, &sync.Mutex{})
	lock, ok := mu.(*sync.Mutex)
	if !ok || !lock.TryLock() {
		return nil, apperrors.ErrImportInProgress
	}
	defer lock.Unlock()

	var out *IBKRImportReport
	var applied *importPlan
	err = s.db.Transaction(func(tx *gorm.DB) error {
		pl, err := s.plan(tx, userID, p, set)
		if err != nil {
			return err
		}
		if pl.blocked() {
			return apperrors.WithDetails(apperrors.ErrImportInvalid, map[string]any{"report": pl.report(false)})
		}
		if err := s.write(tx, pl); err != nil {
			return err
		}
		out, applied = pl.report(true), pl
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logCounts(applied, files, true)
	return out, nil
}

// logCounts logs counts only.
func (s *importService) logCounts(pl *importPlan, files []io.Reader, applied bool) {
	newTx := 0
	for _, sp := range pl.symbols {
		newTx += len(sp.rows)
	}
	logger.Get().Infow("ibkr import",
		"applied", applied, "files", len(files), "securities", len(pl.symbols),
		"investment_transactions", newTx, "cash_rows", len(pl.cashRows),
		"matched_deposits", len(pl.matches), "errors", len(pl.errs), "warnings", len(pl.warnings))
}

// loadImportAccount loads a user's account (soft-deleted rows are invisible).
func loadImportAccount(db *gorm.DB, userID, id string, requireActive bool) (*models.Account, error) {
	q := db.Where("id = ? AND user_id = ?", id, userID)
	if requireActive {
		q = q.Where("is_active = ?", true)
	}
	var a models.Account
	if err := q.First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAccountNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &a, nil
}

// plan builds the whole import plan reading through db; it writes nothing.
func (s *importService) plan(db *gorm.DB, userID string, p IBKRImportParams, set *ibkr.EventSet) (*importPlan, error) {
	pl := &importPlan{userID: userID, params: p, set: set, fxs: s.fxService}

	var err error
	if pl.invAcct, err = loadImportAccount(db, userID, p.InvestmentAccountID, true); err != nil {
		return nil, err
	}
	if pl.invAcct.Type != models.AccountTypeInvestment {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "investment_account_id must be an investment account")
	}
	if pl.cash, err = loadImportAccount(db, userID, p.CashAccountID, true); err != nil {
		return nil, err
	}
	if pl.cash.Type != models.AccountTypeCash {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "cash_account_id must be a cash account")
	}
	if normalizeCode(pl.cash.Currency) != normalizeCode(set.BaseCurrency) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("cash_account_id must be a %s account (the statements' base currency)", normalizeCode(set.BaseCurrency)))
	}
	if p.LegacyCashAccountID != "" {
		if pl.legacy, err = loadImportAccount(db, userID, p.LegacyCashAccountID, false); err != nil {
			return nil, err
		}
		if pl.legacy.Type != models.AccountTypeCash {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "legacy_cash_account_id must be a cash account")
		}
	}
	if pl.base, err = userBaseCurrency(db, userID); err != nil {
		return nil, err
	}

	rates := &importRates{pl: pl, cache: map[string]importRate{}}
	if err := s.planSecurities(db, pl, rates); err != nil {
		return nil, err
	}
	if pl.fxMissing > 0 {
		pl.addError("fx_rate_missing",
			"no %s/%s exchange rate on or before %s (%d events affected); run `oracle backfill --fx` first",
			pl.fxMissingCcy, pl.base, pl.fxMissingFirst, pl.fxMissing)
	}
	if pl.fxStale {
		pl.warn("some trade-date exchange rates are older than the staleness limit")
	}
	if err := s.planCash(db, pl); err != nil {
		return nil, err
	}
	return pl, nil
}

type importRate struct {
	rate fx.Rate
	ok   bool
}

// importRates resolves trade-date FX rates once per timestamp.
type importRates struct {
	pl    *importPlan
	cache map[string]importRate
}

func (r *importRates) at(ccy string, at time.Time) (fx.Rate, bool, error) {
	ccy = normalizeCode(ccy)
	if ccy == r.pl.base {
		return identityRate, true, nil
	}
	key := fmt.Sprintf("%s|%d", ccy, at.Unix())
	if c, ok := r.cache[key]; ok {
		return c.rate, c.ok, nil
	}
	var res importRate
	if r.pl.fxs != nil {
		q, err := r.pl.fxs.Rate(ccy, r.pl.base, at)
		switch {
		case err == nil:
			res = importRate{rate: q.Rate, ok: true}
			if q.Stale {
				r.pl.fxStale = true
			}
		case errors.Is(err, apperrors.ErrFXRateUnavailable):
		default:
			return 0, false, err
		}
	}
	r.cache[key] = res
	return res.rate, res.ok, nil
}
