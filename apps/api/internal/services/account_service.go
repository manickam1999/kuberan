package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services/fdmath"
)

// accountService handles account-related business logic.
type accountService struct {
	db              *gorm.DB
	categoryService CategoryServicer
}

// NewAccountService creates a new AccountServicer.
//
// The optional categoryService argument is used by the fixed-deposit close path
// to provision the "Interest Income" category (plan 020, B5). Pass it in
// production wiring; it may be omitted in tests that do not exercise that path.
func NewAccountService(db *gorm.DB, categoryService ...CategoryServicer) AccountServicer {
	var cs CategoryServicer
	if len(categoryService) > 0 {
		cs = categoryService[0]
	}
	return &accountService{db: db, categoryService: cs}
}

// CreateCashAccount creates a new cash account for a user
func (s *accountService) CreateCashAccount(userID, name, description, currency string, initialBalance int64) (*models.Account, error) {
	// Validate input
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD" // Default currency
	}

	// Create account
	account := &models.Account{
		UserID:      userID,
		Name:        name,
		Type:        models.AccountTypeCash,
		Description: description,
		Balance:     initialBalance,
		Currency:    currency,
		IsActive:    true,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}

		if initialBalance > 0 {
			transaction := &models.Transaction{
				UserID:      userID,
				AccountID:   account.ID,
				Type:        models.TransactionTypeIncome,
				Amount:      initialBalance,
				Description: "Initial balance",
				Date:        time.Now(),
			}
			if err := tx.Create(transaction).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return account, nil
}

// CreateInvestmentAccount creates a new investment account for a user.
func (s *accountService) CreateInvestmentAccount(userID, name, description, currency, broker, accountNumber string) (*models.Account, error) {
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD"
	}

	account := &models.Account{
		UserID:        userID,
		Name:          name,
		Type:          models.AccountTypeInvestment,
		Description:   description,
		Currency:      currency,
		Broker:        broker,
		AccountNumber: accountNumber,
		IsActive:      true,
	}

	if err := s.db.Create(account).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return account, nil
}

// CreateCreditCardAccount creates a new credit card account for a user.
func (s *accountService) CreateCreditCardAccount(userID, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error) {
	if name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}

	if currency == "" {
		currency = "USD"
	}

	account := &models.Account{
		UserID:       userID,
		Name:         name,
		Type:         models.AccountTypeCreditCard,
		Description:  description,
		Balance:      0,
		Currency:     currency,
		IsActive:     true,
		CreditLimit:  creditLimit,
		InterestRate: interestRate,
	}

	if dueDate != nil {
		account.DueDate = *dueDate
	}

	if err := s.db.Create(account).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return account, nil
}

// GetUserAccounts retrieves a paginated list of accounts for a user.
func (s *accountService) GetUserAccounts(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.Account{}).Where("user_id = ? AND is_active = ?", userID, true)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var accounts []models.Account
	if err := base.Preload("FixedDeposit").Order("is_pinned DESC, name ASC").Scopes(pagination.Paginate(page)).Find(&accounts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if err := s.enrichInvestmentBalances(accounts); err != nil {
		return nil, err
	}

	result := pagination.NewPageResponse(accounts, page.Page, page.PageSize, totalItems)
	return &result, nil
}

// GetAccountByID retrieves an account by ID for a specific user
func (s *accountService) GetAccountByID(userID, accountID string) (*models.Account, error) {
	var account models.Account
	if err := s.db.Preload("FixedDeposit").Where("id = ? AND user_id = ? AND is_active = ?", accountID, userID, true).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAccountNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if account.Type == models.AccountTypeInvestment {
		accounts := []models.Account{account}
		if err := s.enrichInvestmentBalances(accounts); err != nil {
			return nil, err
		}
		account = accounts[0]
	}

	return &account, nil
}

// UpdateAccount updates an existing account for any account type.
// Only fields relevant to the account's type are applied.
func (s *accountService) UpdateAccount(userID, accountID string, fields AccountUpdateFields) (*models.Account, error) {
	account, err := s.GetAccountByID(userID, accountID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	// Common fields (all account types)
	if fields.Name != nil && *fields.Name != "" {
		updates["name"] = *fields.Name
	}
	if fields.Description != nil {
		updates["description"] = *fields.Description
	}
	if fields.IsActive != nil {
		updates["is_active"] = *fields.IsActive
	}
	if fields.IsPinned != nil {
		updates["is_pinned"] = *fields.IsPinned
	}

	// Investment-only fields
	if account.Type == models.AccountTypeInvestment {
		if fields.Broker != nil {
			updates["broker"] = *fields.Broker
		}
		if fields.AccountNumber != nil {
			updates["account_number"] = *fields.AccountNumber
		}
	}

	// Credit card-only fields
	if account.Type == models.AccountTypeCreditCard {
		if fields.InterestRate != nil {
			updates["interest_rate"] = *fields.InterestRate
		}
		if fields.DueDate != nil {
			updates["due_date"] = *fields.DueDate
		}
		if fields.CreditLimit != nil {
			updates["credit_limit"] = *fields.CreditLimit
		}
	}

	// Fixed deposit mutable fields.
	// Principal and StartDate are immutable; any attempt to change them is silently
	// ignored here — the handler should reject such requests before calling this.
	if account.Type == models.AccountTypeFixedDeposit {
		if fields.InterestRate != nil {
			updates["interest_rate"] = *fields.InterestRate
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(account).Updates(updates).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		// Reload to get fresh data
		if err := s.db.Where("id = ?", account.ID).First(account).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	// Fixed deposit: update mutable details fields and recompute maturity value
	// when anything affecting the projection changes.
	if account.Type == models.AccountTypeFixedDeposit {
		if err := s.updateFixedDepositDetails(account, fields); err != nil {
			return nil, err
		}
	}

	return account, nil
}

// updateFixedDepositDetails applies mutable FixedDepositDetails fields and
// recomputes MaturityValue when rate, maturity date, compounding, or payout
// settings change. Called within UpdateAccount after the accounts row is saved.
func (s *accountService) updateFixedDepositDetails(account *models.Account, fields AccountUpdateFields) error {
	// Load current details.
	var details models.FixedDepositDetails
	if err := s.db.Where("account_id = ?", account.ID).First(&details).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No details row - nothing to update.
			return nil
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	detailUpdates := make(map[string]interface{})
	recomputeMV := false

	if fields.MaturityDate != nil {
		detailUpdates["maturity_date"] = *fields.MaturityDate
		details.MaturityDate = *fields.MaturityDate
		recomputeMV = true
	}
	if fields.Compounding != nil {
		detailUpdates["compounding"] = *fields.Compounding
		details.Compounding = *fields.Compounding
		recomputeMV = true
	}
	if fields.InterestRate != nil {
		// Rate lives on accounts but MaturityValue is in details; flag recompute.
		account.InterestRate = *fields.InterestRate
		recomputeMV = true
	}
	if fields.PayoutType != nil {
		detailUpdates["payout_type"] = *fields.PayoutType
		details.PayoutType = *fields.PayoutType
	}
	if fields.PayoutFrequency != nil {
		detailUpdates["payout_frequency"] = *fields.PayoutFrequency
		details.PayoutFrequency = fields.PayoutFrequency
	}
	if fields.LinkedAccountID != nil {
		if *fields.LinkedAccountID == "" {
			detailUpdates["linked_account_id"] = nil
		} else {
			detailUpdates["linked_account_id"] = *fields.LinkedAccountID
		}
	}

	if recomputeMV {
		mv := fdmath.MaturityValue(
			details.Principal,
			account.InterestRate,
			details.StartDate,
			details.MaturityDate,
			compoundingPeriods(details.Compounding),
		)
		detailUpdates["maturity_value"] = mv
	}

	if len(detailUpdates) > 0 {
		if err := s.db.Model(&details).Updates(detailUpdates).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}

	return nil
}

// isLiabilityAccount reports whether an account's balance represents an amount
// owed (credit_card and debt), so signs are inverted: expenses increase the
// balance and income/repayments decrease it.
func isLiabilityAccount(t models.AccountType) bool {
	return t == models.AccountTypeCreditCard || t == models.AccountTypeDebt
}

// balanceDelta returns the signed change to an account's balance for a
// transaction of the given type. Income and reimbursement credit the account,
// expense debits it; liability accounts (credit_card, debt) are inverted. The
// bool is false for types that do not have a single-leg balance effect.
func balanceDelta(accountType models.AccountType, txnType models.TransactionType, amount int64) (int64, bool) {
	var delta int64
	switch txnType {
	case models.TransactionTypeIncome, models.TransactionTypeReimbursement:
		delta = amount
	case models.TransactionTypeExpense:
		delta = -amount
	default:
		return 0, false
	}
	if isLiabilityAccount(accountType) {
		delta = -delta
	}
	return delta, true
}

// UpdateAccountBalance updates the balance of an account based on transaction.
// Liability accounts (credit_card, debt) hold the amount owed: an expense
// increases the balance, while income and reimbursements decrease it.
// Transaction types without a single-leg balance effect are ignored.
func (s *accountService) UpdateAccountBalance(tx *gorm.DB, account *models.Account, transactionType models.TransactionType, amount int64) error {
	delta, ok := balanceDelta(account.Type, transactionType, amount)
	if ok {
		account.Balance += delta
	}

	// Save the updated balance
	if err := tx.Model(account).Update("balance", account.Balance).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// ApplyBalanceDelta applies the balance effect of a transaction to an account
// with a relative SQL update (balance = balance + delta) instead of writing an
// absolute value, so it never overwrites a concurrent change. The caller passes
// the account's type (it already holds the locked account row), which avoids a
// re-read. Only income, expense and reimbursement are supported; the sign rules
// match UpdateAccountBalance.
func (s *accountService) ApplyBalanceDelta(tx *gorm.DB, accountID string, accountType models.AccountType, txnType models.TransactionType, amount int64) error {
	if amount < 0 {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "amount must not be negative")
	}
	delta, ok := balanceDelta(accountType, txnType, amount)
	if !ok {
		return apperrors.ErrInvalidTransactionType
	}

	res := tx.Model(&models.Account{}).
		Where("id = ?", accountID).
		UpdateColumn("balance", gorm.Expr("balance + ?", delta))
	if res.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrAccountNotFound
	}
	return nil
}

// compoundingPeriods maps a Compounding enum to the number of compounding periods
// per year consumed by fdmath.MaturityValue.
func compoundingPeriods(c models.Compounding) int {
	switch c {
	case models.CompoundingMonthly:
		return 12
	case models.CompoundingQuarterly:
		return 4
	case models.CompoundingSemiannual:
		return 2
	case models.CompoundingAnnual:
		return 1
	default:
		return 1
	}
}

// nextPayoutDateAfter returns the first payout date that falls strictly after
// startDate for a given PayoutFrequency. It advances by calendar months so that
// "monthly" from Jan 15 yields Feb 15, etc.
func nextPayoutDateAfter(startDate time.Time, freq models.PayoutFrequency) time.Time {
	switch freq {
	case models.PayoutFrequencyMonthly:
		return startDate.AddDate(0, 1, 0)
	case models.PayoutFrequencyQuarterly:
		return startDate.AddDate(0, 3, 0)
	case models.PayoutFrequencySemiannual:
		return startDate.AddDate(0, 6, 0)
	case models.PayoutFrequencyAnnual:
		return startDate.AddDate(1, 0, 0)
	default:
		return startDate.AddDate(0, 1, 0)
	}
}

// CreateFixedDepositAccount opens a new fixed deposit account and atomically
// transfers the principal from a source cash account.
//
// Validation: source account must belong to userID, be active, have type==cash,
// and hold at least params.Principal cents. Currency of source (and optional
// linked) account must match params.Currency. params.MaturityDate must be after
// params.StartDate. For periodic payout types, PayoutFrequency is required and
// LinkedAccountID (if provided) must be an active user cash account.
//
// The DB transaction: creates the FD account (balance 0), creates the
// fixed_deposit_details row, then posts a transfer record that debits the source
// account and credits the FD account, ending with FD.Balance == params.Principal.
func (s *accountService) CreateFixedDepositAccount(userID string, params CreateFixedDepositParams) (*models.Account, error) {
	// --- basic input validation ---
	if params.Name == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "account name is required")
	}
	if params.Principal <= 0 {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "principal must be greater than zero")
	}
	if !params.MaturityDate.After(params.StartDate) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "maturity date must be after start date")
	}
	if params.PayoutType == models.PayoutTypePeriodic && params.PayoutFrequency == nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "payout frequency is required for periodic payout type")
	}
	if params.Currency == "" {
		params.Currency = "USD"
	}

	// --- validate source account ---
	sourceAccount, err := s.GetAccountByID(userID, params.SourceAccountID)
	if err != nil {
		return nil, err
	}
	if sourceAccount.Type != models.AccountTypeCash {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidAccountType, "source account must be a cash account")
	}
	if sourceAccount.Currency != params.Currency {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "source account currency must match the fixed deposit currency")
	}
	if sourceAccount.Balance < params.Principal {
		return nil, apperrors.ErrSourceAccountInsufficient
	}

	// --- validate linked account (if provided) ---
	if params.LinkedAccountID != nil {
		linkedAccount, lerr := s.GetAccountByID(userID, *params.LinkedAccountID)
		if lerr != nil {
			return nil, lerr
		}
		if linkedAccount.Type != models.AccountTypeCash {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidAccountType, "linked account must be a cash account")
		}
		if linkedAccount.Currency != params.Currency {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "linked account currency must match the fixed deposit currency")
		}
	}

	// --- compute projected maturity value ---
	mv := fdmath.MaturityValue(
		params.Principal,
		params.InterestRate,
		params.StartDate,
		params.MaturityDate,
		compoundingPeriods(params.Compounding),
	)

	// --- determine next payout date for periodic FDs ---
	var nextPayoutDate *time.Time
	if params.PayoutType == models.PayoutTypePeriodic && params.PayoutFrequency != nil {
		npd := nextPayoutDateAfter(params.StartDate, *params.PayoutFrequency)
		nextPayoutDate = &npd
	}

	var fdAccount models.Account
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Create the FD account with balance 0.
		fdAccount = models.Account{
			UserID:       userID,
			Name:         params.Name,
			Type:         models.AccountTypeFixedDeposit,
			Description:  params.Description,
			Balance:      0,
			Currency:     params.Currency,
			IsActive:     true,
			InterestRate: params.InterestRate,
		}
		if txErr := tx.Create(&fdAccount).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// 2. Create the fixed_deposit_details row.
		details := models.FixedDepositDetails{
			AccountID:       fdAccount.ID,
			Principal:       params.Principal,
			StartDate:       params.StartDate,
			MaturityDate:    params.MaturityDate,
			PayoutType:      params.PayoutType,
			PayoutFrequency: params.PayoutFrequency,
			Compounding:     params.Compounding,
			LinkedAccountID: params.LinkedAccountID,
			NextPayoutDate:  nextPayoutDate,
			MaturityValue:   mv,
			Status:          models.FixedDepositStatusActive,
		}
		if txErr := tx.Create(&details).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// 3. Transfer principal from source -> FD.
		// Replicate transfer semantics (matching CreateTransfer in transaction_service.go):
		//   - one TransactionTypeTransfer record with ToAccountID set
		//   - debit source (TransactionTypeExpense semantics)
		//   - credit FD (TransactionTypeIncome semantics)
		transferTx := &models.Transaction{
			UserID:      userID,
			AccountID:   sourceAccount.ID,
			ToAccountID: &fdAccount.ID,
			Type:        models.TransactionTypeTransfer,
			Amount:      params.Principal,
			Description: "Fixed deposit funding - " + params.Name,
			Date:        params.StartDate,
		}
		if txErr := tx.Create(transferTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Debit source.
		sourceAccount.Balance -= params.Principal
		if txErr := tx.Model(sourceAccount).Update("balance", sourceAccount.Balance).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Credit FD (brings it to principal).
		fdAccount.Balance += params.Principal
		if txErr := tx.Model(&fdAccount).Update("balance", fdAccount.Balance).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Attach details to the account so callers get them back.
		fdAccount.FixedDeposit = &details
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &fdAccount, nil
}

// CloseFixedDeposit realizes a fixed deposit: returns principal to a destination
// cash account and posts the net interest as an income transaction categorized to
// the user's "Interest Income" category.
//
// It handles both maturity realization and premature withdrawal. When status is
// already "closed" the call is rejected. Destination defaults to
// details.LinkedAccountID when params.DestinationAccountID is nil.
//
// Interest computation:
//   - If params.InterestOverride is non-nil: use that value directly.
//   - Periodic payout: 0 (interest was already disbursed on each payout; principal only).
//   - Cumulative + matured: (MaturityValue - Principal).
//   - Cumulative + premature: fdmath.AccruedInterest as of today.
//
// Penalty (params.Penalty) is subtracted from the gross interest; net interest is
// floored at 0 (the penalty never turns into a principal deduction here).
func (s *accountService) CloseFixedDeposit(userID, accountID string, params CloseFixedDepositParams) (*models.Account, error) {
	// Load FD account with details.
	var fdAccount models.Account
	if err := s.db.Preload("FixedDeposit").
		Where("id = ? AND user_id = ?", accountID, userID).
		First(&fdAccount).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAccountNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if fdAccount.Type != models.AccountTypeFixedDeposit {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidAccountType, "account is not a fixed deposit")
	}
	if fdAccount.FixedDeposit == nil {
		return nil, apperrors.ErrFixedDepositDetailsNotFound
	}

	details := fdAccount.FixedDeposit

	if details.Status == models.FixedDepositStatusClosed {
		return nil, apperrors.ErrFixedDepositAlreadyClosed
	}

	// --- resolve destination account ---
	destID := params.DestinationAccountID
	if destID == nil {
		destID = details.LinkedAccountID
	}
	if destID == nil {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "destination account is required (no linked account set)")
	}

	destAccount, err := s.GetAccountByID(userID, *destID)
	if err != nil {
		return nil, err
	}
	if destAccount.Type != models.AccountTypeCash {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidAccountType, "destination account must be a cash account")
	}

	// --- compute interest ---
	var grossInterest int64
	switch {
	case params.InterestOverride != nil:
		grossInterest = *params.InterestOverride
	case details.PayoutType == models.PayoutTypePeriodic:
		// Periodic FDs disburse interest on each scheduled payout, so closing only
		// returns principal. Any partial, not-yet-paid period is forfeited on early
		// close (matching how real-world FDs treat incomplete interest periods).
		grossInterest = 0
	case details.Status == models.FixedDepositStatusMatured:
		// Cumulative, matured: realize the full projected interest in a lump sum.
		grossInterest = details.MaturityValue - details.Principal
	default:
		// Cumulative, premature withdrawal: accrue simple interest to date.
		grossInterest = fdmath.AccruedInterest(
			details.Principal,
			fdAccount.InterestRate,
			details.StartDate,
			time.Now(),
		)
	}

	netInterest := grossInterest - params.Penalty
	if netInterest < 0 {
		netInterest = 0
	}

	// --- provision Interest Income category (idempotent) ---
	var interestCategoryID *string
	if netInterest > 0 && s.categoryService != nil {
		cat, cerr := s.categoryService.EnsureInterestIncomeCategory(userID)
		if cerr != nil {
			return nil, cerr
		}
		interestCategoryID = &cat.ID
	}

	principal := details.Principal

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Transfer principal FD -> destination (FD balance -> 0).
		transferTx := &models.Transaction{
			UserID:      userID,
			AccountID:   fdAccount.ID,
			ToAccountID: destID,
			Type:        models.TransactionTypeTransfer,
			Amount:      principal,
			Description: "Fixed deposit closure - principal return",
			Date:        time.Now(),
		}
		if txErr := tx.Create(transferTx).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Debit FD (balance -> 0).
		fdAccount.Balance -= principal
		if txErr := tx.Model(&fdAccount).Update("balance", fdAccount.Balance).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// Credit destination.
		destAccount.Balance += principal
		if txErr := tx.Model(destAccount).Update("balance", destAccount.Balance).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		// 2. Post interest income transaction (if any) to destination.
		if netInterest > 0 {
			interestTx := &models.Transaction{
				UserID:      userID,
				AccountID:   destAccount.ID,
				CategoryID:  interestCategoryID,
				Type:        models.TransactionTypeIncome,
				Amount:      netInterest,
				Description: "Fixed deposit interest - " + fdAccount.Name,
				Date:        time.Now(),
			}
			if txErr := tx.Create(interestTx).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}

			// Credit destination balance for the interest income.
			destAccount.Balance += netInterest
			if txErr := tx.Model(destAccount).Update("balance", destAccount.Balance).Error; txErr != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
			}
		}

		// 3. Mark the details as closed and deactivate the account.
		if txErr := tx.Model(details).Updates(map[string]interface{}{
			"status": models.FixedDepositStatusClosed,
		}).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}
		if txErr := tx.Model(&fdAccount).Update("is_active", false).Error; txErr != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, txErr)
		}

		fdAccount.IsActive = false
		details.Status = models.FixedDepositStatusClosed
		fdAccount.FixedDeposit = details
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &fdAccount, nil
}

// enrichInvestmentBalances computes market-value balances for investment accounts
// in the given slice. Non-investment accounts are left unchanged.
func (s *accountService) enrichInvestmentBalances(accounts []models.Account) error {
	// Collect investment account IDs
	var investmentAccountIDs []string
	for i := range accounts {
		if accounts[i].Type == models.AccountTypeInvestment {
			investmentAccountIDs = append(investmentAccountIDs, accounts[i].ID)
		}
	}
	if len(investmentAccountIDs) == 0 {
		return nil
	}

	// Fetch investments for those accounts
	type holding struct {
		AccountID  string
		SecurityID string
		Quantity   float64
	}
	var holdings []holding
	if err := s.db.Table("investments").
		Select("account_id, security_id, quantity").
		Where("account_id IN ? AND deleted_at IS NULL", investmentAccountIDs).
		Scan(&holdings).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	if len(holdings) == 0 {
		return nil
	}

	// Collect distinct security IDs
	secIDSet := make(map[string]struct{})
	for _, h := range holdings {
		secIDSet[h.SecurityID] = struct{}{}
	}
	secIDs := make([]string, 0, len(secIDSet))
	for id := range secIDSet {
		secIDs = append(secIDs, id)
	}

	// Batch-fetch latest prices
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return err
	}

	// Accumulate market value per account
	balances := make(map[string]int64)
	for _, h := range holdings {
		balances[h.AccountID] += int64(h.Quantity * float64(prices[h.SecurityID]))
	}

	// Set balances on the account slice
	for i := range accounts {
		if accounts[i].Type == models.AccountTypeInvestment {
			accounts[i].Balance = balances[accounts[i].ID]
		}
	}

	return nil
}
