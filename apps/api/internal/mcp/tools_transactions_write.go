package mcp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// transactionsWriteScope gates every tool that creates or edits transactions,
// including repayments and write-offs, which move money through transactions.
const transactionsWriteScope = "write:transactions"

// maxCategorizeBatch caps categorize_transactions. It is kept below the default
// per-minute rate limit so a full batch can always run.
const maxCategorizeBatch = 25

// clearCategory is the category_id value that removes a transaction's category.
const clearCategory = "none"

func (s *Server) registerTransactionWriteTools() {
	s.addWriteTool(
		writeTool("create_transaction",
			"Record an income or expense transaction in one of the user's cash, credit card or loan (debt) accounts and update its balance; "+
				"fixed deposits and investment accounts cannot take plain transactions. "+
				"Get account and category IDs from list_accounts and list_categories. The amount is in the account's own currency "+
				"(shown by list_accounts); on a credit card or loan an expense raises what is owed and income lowers it. "+
				"The category must be an income category for income and an expense category for an expense. "+
				"If no category is given, the user's categorization rules may assign one. Use create_transfer, not this, to move money between accounts.",
			false,
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("type", mcp.Required(), mcp.Enum("income", "expense"), mcp.Description("'income' or 'expense'")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Amount in the account's own currency, as a positive decimal like 12.50")),
			mcp.WithString("date", mcp.Required(), mcp.Description(dateArgDescription)),
			mcp.WithString("category_id", mcp.Description("Category ID; must match the transaction type")),
			mcp.WithString("description", mcp.Description("Description, e.g. the merchant (max 500 characters)")),
		),
		transactionsWriteScope, s.prepareCreateTransaction,
	)

	s.addWriteTool(
		writeTool("create_transfer",
			"Move money between two of the user's cash, credit card or loan (debt) accounts (for example paying a credit card or a loan from a cash account). "+
				"Transfers are not income or spending. amount is in the source account's currency. When the two accounts have "+
				"different currencies, to_amount (the amount arriving, in the destination account's currency) is required; "+
				"otherwise omit it.",
			false,
			mcp.WithString("from_account_id", mcp.Required(), mcp.Description("Account the money leaves")),
			mcp.WithString("to_account_id", mcp.Required(), mcp.Description("Account the money arrives in")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Amount leaving the source account, in the source account's currency, as a positive decimal like 12.50")),
			mcp.WithNumber("to_amount", mcp.Description("Amount arriving in the destination account, in the destination account's currency, as a positive decimal. Required when the currencies differ; omit (or equal to amount) when they are the same")),
			mcp.WithString("date", mcp.Required(), mcp.Description(dateArgDescription)),
			mcp.WithString("description", mcp.Description("Description (max 500 characters)")),
		),
		transactionsWriteScope, s.prepareCreateTransfer,
	)

	s.addWriteTool(
		writeTool("update_transaction",
			"Edit an income or expense transaction. Only the fields given change; the account balance is adjusted. "+
				"The amount is in the transaction's account currency. A category must match the transaction's (new) type; "+
				"changing the type needs a category_id that fits it, or 'none', when the transaction has a category. "+
				"Transfers, investment transactions, repayments and opening balances cannot be edited here.",
			true,
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID")),
			mcp.WithString("type", mcp.Enum("income", "expense"), mcp.Description("New type: 'income' or 'expense'")),
			mcp.WithNumber("amount", mcp.Description("New amount in the transaction's account currency, as a positive decimal like 12.50")),
			mcp.WithString("category_id", mcp.Description("New category ID, or 'none' to remove the category")),
			mcp.WithString("description", mcp.Description("New description (max 500 characters)")),
			mcp.WithString("date", mcp.Description("New date, in the user's own time zone (YYYY-MM-DD)")),
		),
		transactionsWriteScope, s.prepareUpdateTransaction,
	)

	s.addWriteTool(
		writeTool("categorize_transactions",
			fmt.Sprintf("Set the category of up to %d income or expense transactions at once. "+
				"Each transaction's type must match the category type; ones that do not are skipped and reported.", maxCategorizeBatch),
			true,
			mcp.WithArray("transaction_ids", mcp.Required(), mcp.WithStringItems(),
				mcp.MinItems(1), mcp.MaxItems(maxCategorizeBatch), mcp.Description("Transaction IDs")),
			mcp.WithString("category_id", mcp.Required(), mcp.Description("Category ID to assign")),
		),
		transactionsWriteScope, s.prepareCategorizeTransactions,
	)
}

// parseTxnType validates a write tool's transaction type: only income and
// expense can be created or edited through MCP.
func parseTxnType(v string) (models.TransactionType, *mcp.CallToolResult) {
	switch t := models.TransactionType(v); t {
	case models.TransactionTypeIncome, models.TransactionTypeExpense:
		return t, nil
	default:
		return "", mcp.NewToolResultError("type must be 'income' or 'expense'")
	}
}

// checkCategory verifies that categoryID belongs to the user and that its type
// matches txType. The transaction service enforces both too (CATEGORY_TYPE_MISMATCH
// on create and update); checking here first gives the model a message that names
// the category and its type, and keeps a refused write from counting against the
// rate limit.
func (s *Server) checkCategory(userID, categoryID string, txType models.TransactionType) *mcp.CallToolResult {
	category, err := s.services.Categories.GetCategoryByID(userID, categoryID)
	if err != nil {
		return serviceError("find category", err)
	}
	if string(category.Type) != string(txType) {
		return mcp.NewToolResultError(fmt.Sprintf(
			"category %s is an %s category and cannot be used for an %s transaction", untrusted(category.Name), category.Type, txType))
	}
	return nil
}

// spendingAccountOnly rejects accounts that income, expenses and transfers cannot
// be recorded in. Cash, credit card and loan accounts take day-to-day
// transactions; a fixed deposit and an investment account are changed by the
// app's own flows (opening or closing the deposit, trades), and a plain
// transaction there would leave a record that does not match the real balance.
func spendingAccountOnly(a *models.Account) *mcp.CallToolResult {
	if a.Type.IsSpending() {
		return nil
	}
	return mcp.NewToolResultError(fmt.Sprintf(
		"%s is a %s account: transactions can only be recorded in cash, credit card and loan (debt) accounts; use the Kuberan app for fixed deposits and investments",
		untrusted(a.Name), a.Type))
}

// categoryLine reports a created transaction's category by name; byRule marks
// one the user's categorization rules chose.
func (s *Server) categoryLine(userID, categoryID string, byRule bool) string {
	category, err := s.services.Categories.GetCategoryByID(userID, categoryID)
	if err != nil {
		return fmt.Sprintf("  Category ID: %s\n", categoryID)
	}
	suffix := ""
	if byRule {
		suffix = " (assigned by a rule)"
	}
	return fmt.Sprintf("  Category: %s%s\n    Category ID: %s\n", untrusted(category.Name), suffix, categoryID)
}

// balanceLine reports an account's balance after a write, or "" if it cannot
// be read (the write itself has already succeeded).
func (s *Server) balanceLine(userID, accountID string) string {
	account, err := s.services.Accounts.GetAccountByID(userID, accountID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("  %s balance is now %s\n", untrusted(account.Name), formatMoney(account.Balance, account.Currency))
}

func (s *Server) prepareCreateTransaction(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	accountID, res := requiredString(args, "account_id")
	if res != nil {
		return nil, res
	}
	txType, res := parseTxnType(stringArg(args, "type"))
	if res != nil {
		return nil, res
	}
	amount, res := s.requiredAmount(args)
	if res != nil {
		return nil, res
	}
	date, res := requiredDate(args, time.Now())
	if res != nil {
		return nil, res
	}
	description, res := descriptionArg(args)
	if res != nil {
		return nil, res
	}
	account, err := s.services.Accounts.GetAccountByID(c.UserID, accountID)
	if err != nil {
		return nil, serviceError("find account", err)
	}
	if res := spendingAccountOnly(account); res != nil {
		return nil, res
	}
	if res := s.checkCeiling(c.UserID, "amount", amount, account.Currency); res != nil {
		return nil, res
	}
	var categoryID *string
	if v := stringArg(args, "category_id"); v != "" {
		if res := s.checkCategory(c.UserID, v, txType); res != nil {
			return nil, res
		}
		categoryID = &v
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		txn, err := s.services.Transactions.CreateTransaction(c.UserID, accountID, categoryID, txType, amount, description, date, mcpMeta(c))
		if err != nil {
			return "", nil, serviceError("create transaction", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Created %s of %s on %s: %s\n",
			txn.Type, formatMoney(txn.Amount, account.Currency), txn.Date.UTC().Format("2006-01-02"), untrusted(txn.Description))
		fmt.Fprintf(&sb, "  ID: %s\n", txn.ID)
		if txn.CategoryID != nil {
			sb.WriteString(s.categoryLine(c.UserID, *txn.CategoryID, categoryID == nil))
		}
		sb.WriteString(s.balanceLine(c.UserID, accountID))
		return sb.String(), []auditEntry{{
			Action: "MCP_CREATE_TRANSACTION", ResourceType: "transaction", ResourceID: txn.ID,
			Changes: map[string]interface{}{
				"type": txType, "amount": amount, "account_id": accountID,
				"description": description, "currency": currencyCode(account.Currency),
			},
		}}, nil
	}}, nil
}

func (s *Server) prepareCreateTransfer(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	fromID, res := requiredString(args, "from_account_id")
	if res != nil {
		return nil, res
	}
	toID, res := requiredString(args, "to_account_id")
	if res != nil {
		return nil, res
	}
	amount, res := s.requiredAmount(args)
	if res != nil {
		return nil, res
	}
	date, res := requiredDate(args, time.Now())
	if res != nil {
		return nil, res
	}
	description, res := descriptionArg(args)
	if res != nil {
		return nil, res
	}

	fromAcc, err := s.services.Accounts.GetAccountByID(c.UserID, fromID)
	if err != nil {
		return nil, serviceError("find source account", err)
	}
	toAcc, err := s.services.Accounts.GetAccountByID(c.UserID, toID)
	if err != nil {
		return nil, serviceError("find destination account", err)
	}
	if res := spendingAccountOnly(fromAcc); res != nil {
		return nil, res
	}
	if res := spendingAccountOnly(toAcc); res != nil {
		return nil, res
	}
	// to_amount follows the API's rules: required when the currencies differ,
	// otherwise omitted or equal to amount. Both legs count against the ceiling.
	toAmount, toPresent, res := moneyArg(args, "to_amount")
	if res != nil {
		return nil, res
	}
	sameCurrency := strings.EqualFold(fromAcc.Currency, toAcc.Currency)
	switch {
	case !sameCurrency && !toPresent:
		return nil, s.toAmountRequired(fromAcc.Currency, toAcc.Currency, amount, date)
	case sameCurrency && toPresent && toAmount != amount:
		return nil, mcp.NewToolResultError(fmt.Sprintf(
			"to_amount must be omitted or equal amount: both accounts are in %s", currencyCode(fromAcc.Currency)))
	}
	if res := s.checkCeiling(c.UserID, "amount", amount, fromAcc.Currency); res != nil {
		return nil, res
	}
	var toArg *int64
	if toPresent && !sameCurrency {
		if res := s.checkCeiling(c.UserID, "to_amount", toAmount, toAcc.Currency); res != nil {
			return nil, res
		}
		toArg = &toAmount
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		txn, err := s.services.Transactions.CreateTransfer(c.UserID, fromID, toID, amount, toArg, description, date, mcpMeta(c))
		if err != nil {
			return "", nil, serviceError("create transfer", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Transferred %s on %s\n", transferAmounts(txn, fromAcc.Currency, toAcc.Currency), txn.Date.UTC().Format("2006-01-02"))
		fmt.Fprintf(&sb, "  ID: %s\n", txn.ID)
		sb.WriteString(s.balanceLine(c.UserID, fromID))
		sb.WriteString(s.balanceLine(c.UserID, toID))
		changes := map[string]interface{}{
			"amount": amount, "from_account_id": fromID, "to_account_id": toID,
			"description": description, "currency": currencyCode(fromAcc.Currency),
		}
		if toArg != nil {
			changes["to_amount"] = toAmount
		}
		return sb.String(), []auditEntry{{
			Action: "MCP_CREATE_TRANSFER", ResourceType: "transaction", ResourceID: txn.ID, Changes: changes,
		}}, nil
	}}, nil
}

// transferAmounts renders a created transfer: one amount for a same-currency
// transfer, "FROM → TO (rate r)" when the legs differ.
func transferAmounts(txn *models.Transaction, fromCurrency, toCurrency string) string {
	return transactionAmount(&models.Transaction{
		Type: models.TransactionTypeTransfer, Amount: txn.Amount, ToAmount: txn.ToAmount, ImpliedRate: txn.ImpliedRate,
		Account: models.Account{Currency: fromCurrency}, ToAccount: &models.Account{Currency: toCurrency},
	})
}

// toAmountRequired is the error for a cross-currency transfer without
// to_amount. It carries the market-rate suggestion when a rate exists, like
// the API's FX_AMOUNT_REQUIRED error.
func (s *Server) toAmountRequired(fromCurrency, toCurrency string, amount int64, at time.Time) *mcp.CallToolResult {
	return mcp.NewToolResultError(s.amountRequiredText("to_amount", fromCurrency, toCurrency, amount, at, "accounts are"))
}

// amountRequiredText explains that field must be given in toCurrency, with the
// market-rate suggestion when one exists.
func (s *Server) amountRequiredText(field, fromCurrency, toCurrency string, amount int64, at time.Time, who string) string {
	msg := fmt.Sprintf("%s is required: the %s in different currencies (%s and %s); give the amount in %s",
		field, who, currencyCode(fromCurrency), currencyCode(toCurrency), currencyCode(toCurrency))
	if s.services.FX == nil {
		return msg
	}
	quote, err := s.services.FX.Rate(fromCurrency, toCurrency, at)
	if err != nil {
		return msg
	}
	suggested, err := fx.Convert(amount, quote.Rate)
	if err != nil || suggested <= 0 {
		return msg
	}
	return fmt.Sprintf("%s (the market rate suggests %s, rate %s)", msg, formatMoney(suggested, toCurrency), quote.Rate.String())
}

func (s *Server) prepareUpdateTransaction(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	txnID, res := requiredString(args, "transaction_id")
	if res != nil {
		return nil, res
	}
	before, err := s.services.Transactions.GetTransactionByID(c.UserID, txnID)
	if err != nil {
		return nil, serviceError("find transaction", err)
	}

	switch before.Type {
	case models.TransactionTypeTransfer, models.TransactionTypeInvestment, models.TransactionTypeReimbursement, models.TransactionTypeOpeningBalance:
		// Refused here, before it counts against the rate limit; the service
		// refuses them too.
		return nil, mcp.NewToolResultError(fmt.Sprintf(
			"%s transactions cannot be edited here: only income and expense transactions can be", before.Type))
	}

	account, err := s.services.Accounts.GetAccountByID(c.UserID, before.AccountID)
	if err != nil {
		return nil, serviceError("find account", err)
	}
	currency := account.Currency

	updates := services.TransactionUpdateFields{}
	changes := map[string]interface{}{}
	effectiveType := before.Type
	effectiveAmount := before.Amount
	note := ""

	if v := stringArg(args, "type"); v != "" {
		t, res := parseTxnType(v)
		if res != nil {
			return nil, res
		}
		updates.Type = &t
		effectiveType = t
		changes["type"] = change(before.Type, t)
	}
	if amount, present, res := s.amountArg(args); res != nil {
		return nil, res
	} else if present {
		if res := s.checkCeiling(c.UserID, "amount", amount, currency); res != nil {
			return nil, res
		}
		// The balance moves by the difference, which a huge-to-tiny edit makes
		// large even though the new amount is small.
		delta := amount - before.Amount
		if delta < 0 {
			delta = -delta
		}
		if res := s.checkCeiling(c.UserID, "the amount change", delta, currency); res != nil {
			return nil, res
		}
		updates.Amount = &amount
		effectiveAmount = amount
		changes["amount"] = change(before.Amount, amount)
	}
	if updates.Type != nil && *updates.Type != before.Type {
		// Flipping the type reverses the old amount and applies the new one, so
		// the transaction's amount moves the balance twice over. Both sides must
		// be within the per-change limit, not just an amount passed explicitly.
		if res := s.checkCeiling(c.UserID, "the existing amount", before.Amount, currency); res != nil {
			return nil, res
		}
		if res := s.checkCeiling(c.UserID, "the amount", effectiveAmount, currency); res != nil {
			return nil, res
		}
	}
	description, res := optionalDescriptionArg(args)
	if res != nil {
		return nil, res
	}
	if description != nil {
		updates.Description = description
		changes["description"] = change(before.Description, *description)
	}
	if date, present, res := dateArg(args, time.Now()); res != nil {
		return nil, res
	} else if present {
		updates.Date = &date
		changes["date"] = change(before.Date, date)
	}
	if v := stringArg(args, "category_id"); v != "" {
		var newCategory *string
		if !strings.EqualFold(v, clearCategory) {
			if res := s.checkCategory(c.UserID, v, effectiveType); res != nil {
				return nil, res
			}
			newCategory = &v
		}
		updates.CategoryID = &newCategory
		changes["category_id"] = change(before.CategoryID, newCategory)
	} else if updates.Type != nil && before.CategoryID != nil {
		// Changing the type keeps the old category, so it must still fit. A
		// category the user has since deleted can't be checked or kept: clear it.
		_, err := s.services.Categories.GetCategoryByID(c.UserID, *before.CategoryID)
		switch {
		case errors.Is(err, apperrors.ErrCategoryNotFound):
			var none *string
			updates.CategoryID = &none
			changes["category_id"] = change(before.CategoryID, none)
			note = "  Its category had been deleted, so it is now uncategorized.\n"
		case err != nil:
			return nil, serviceError("find category", err)
		default:
			if res := s.checkCategory(c.UserID, *before.CategoryID, effectiveType); res != nil {
				return nil, mcp.NewToolResultError(toolText(res) +
					"; also pass category_id with a matching category, or 'none'")
			}
		}
	}
	if len(changes) == 0 {
		return nil, mcp.NewToolResultError("nothing to update: give at least one of type, amount, category_id, description, date")
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		txn, err := s.services.Transactions.UpdateTransaction(c.UserID, txnID, updates)
		if err != nil {
			return "", nil, serviceError("update transaction", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Updated transaction: %s  %s  %s  %s\n",
			txn.Date.UTC().Format("2006-01-02"), txn.Type, formatMoney(txn.Amount, currency), untrusted(txn.Description))
		fmt.Fprintf(&sb, "  ID: %s\n", txn.ID)
		sb.WriteString(note)
		sb.WriteString(s.balanceLine(c.UserID, txn.AccountID))
		return sb.String(), []auditEntry{{
			Action: "MCP_UPDATE_TRANSACTION", ResourceType: "transaction", ResourceID: txn.ID, Changes: changes,
		}}, nil
	}}, nil
}

func (s *Server) prepareCategorizeTransactions(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	ids, res := stringListArg(args, "transaction_ids", maxCategorizeBatch)
	if res != nil {
		return nil, res
	}
	categoryID, res := requiredString(args, "category_id")
	if res != nil {
		return nil, res
	}
	category, err := s.services.Categories.GetCategoryByID(c.UserID, categoryID)
	if err != nil {
		return nil, serviceError("find category", err)
	}

	// Decide which transactions can take the category before charging the
	// rate limit, so skipped ones do not count as writes.
	type pending struct {
		id     string
		before *string
	}
	var eligible []pending
	var skipped []string
	for _, id := range ids {
		txn, err := s.services.Transactions.GetTransactionByID(c.UserID, id)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %s", id, serviceErrorMessage(err)))
			continue
		}
		if string(txn.Type) != string(category.Type) {
			skipped = append(skipped, fmt.Sprintf("%s: %s transaction cannot use %s category", id, txn.Type, category.Type))
			continue
		}
		eligible = append(eligible, pending{id: id, before: txn.CategoryID})
	}

	return &writeOp{Cost: len(eligible), Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		var audits []auditEntry
		for _, p := range eligible {
			newCategory := &categoryID
			if _, err := s.services.Transactions.UpdateTransaction(c.UserID, p.id,
				services.TransactionUpdateFields{CategoryID: &newCategory}); err != nil {
				skipped = append(skipped, fmt.Sprintf("%s: %s", p.id, serviceErrorMessage(err)))
				continue
			}
			// Each transaction is audited as a transaction update; the tool
			// name in the entry records that it came from a batch.
			audits = append(audits, auditEntry{
				Action: "MCP_UPDATE_TRANSACTION", ResourceType: "transaction", ResourceID: p.id,
				Changes: map[string]interface{}{"category_id": change(p.before, categoryID)},
			})
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Set category %s on %d of %d transactions.\n", untrusted(category.Name), len(audits), len(ids))
		if len(skipped) > 0 {
			sb.WriteString("Skipped:\n")
			for _, line := range skipped {
				sb.WriteString("  " + line + "\n")
			}
		}
		return sb.String(), audits, nil
	}}, nil
}

// toolText returns the text of a tool result.
func toolText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
