package mcp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	apperrors "kuberan/internal/errors"
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
			"Record an income or expense transaction in one of the user's accounts and update its balance. "+
				"Get account and category IDs from list_accounts and list_categories. If no category is given, "+
				"the user's categorization rules may assign one.",
			false,
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("type", mcp.Required(), mcp.Enum("income", "expense"), mcp.Description("'income' or 'expense'")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Amount in dollars, positive (e.g. 12.50)")),
			mcp.WithString("date", mcp.Required(), mcp.Description(dateArgDescription)),
			mcp.WithString("category_id", mcp.Description("Category ID; must match the transaction type")),
			mcp.WithString("description", mcp.Description("Description, e.g. the merchant (max 500 characters)")),
		),
		transactionsWriteScope, s.prepareCreateTransaction,
	)

	s.addWriteTool(
		writeTool("create_transfer",
			"Move money between two of the user's accounts (for example paying a credit card from a cash account). "+
				"Transfers are not income or spending.",
			false,
			mcp.WithString("from_account_id", mcp.Required(), mcp.Description("Account the money leaves")),
			mcp.WithString("to_account_id", mcp.Required(), mcp.Description("Account the money arrives in")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Amount in dollars, positive")),
			mcp.WithString("date", mcp.Required(), mcp.Description(dateArgDescription)),
			mcp.WithString("description", mcp.Description("Description (max 500 characters)")),
		),
		transactionsWriteScope, s.prepareCreateTransfer,
	)

	s.addWriteTool(
		writeTool("update_transaction",
			"Edit an income or expense transaction. Only the fields given change; the account balance is adjusted. "+
				"Transfers, investment transactions and repayments cannot be edited here.",
			true,
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID")),
			mcp.WithString("type", mcp.Enum("income", "expense"), mcp.Description("New type: 'income' or 'expense'")),
			mcp.WithNumber("amount", mcp.Description("New amount in dollars, positive")),
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
// matches txType. The transaction service enforces ownership itself; the type
// match is an MCP-only rule (the web app allows any category), applied because
// a model picking an income category for an expense is almost always a mistake.
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

// balanceLine reports an account's balance after a write, or "" if it cannot
// be read (the write itself has already succeeded).
func (s *Server) balanceLine(userID, accountID string) string {
	account, err := s.services.Accounts.GetAccountByID(userID, accountID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("  %s balance is now %s\n", untrusted(account.Name), formatCents(account.Balance))
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
	var categoryID *string
	if v := stringArg(args, "category_id"); v != "" {
		if res := s.checkCategory(c.UserID, v, txType); res != nil {
			return nil, res
		}
		categoryID = &v
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		txn, err := s.services.Transactions.CreateTransaction(c.UserID, accountID, categoryID, txType, amount, description, date)
		if err != nil {
			return "", nil, serviceError("create transaction", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Created %s of %s on %s: %s\n",
			txn.Type, formatCents(txn.Amount), txn.Date.Format("2006-01-02"), untrusted(txn.Description))
		fmt.Fprintf(&sb, "  ID: %s\n", txn.ID)
		if txn.CategoryID != nil {
			fmt.Fprintf(&sb, "  Category ID: %s\n", *txn.CategoryID)
		}
		sb.WriteString(s.balanceLine(c.UserID, accountID))
		return sb.String(), []auditEntry{{
			Action: "MCP_CREATE_TRANSACTION", ResourceType: "transaction", ResourceID: txn.ID,
			Changes: map[string]interface{}{"type": txType, "amount": amount, "account_id": accountID},
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

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		txn, err := s.services.Transactions.CreateTransfer(c.UserID, fromID, toID, amount, description, date)
		if err != nil {
			return "", nil, serviceError("create transfer", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Transferred %s on %s\n", formatCents(txn.Amount), txn.Date.Format("2006-01-02"))
		fmt.Fprintf(&sb, "  ID: %s\n", txn.ID)
		sb.WriteString(s.balanceLine(c.UserID, fromID))
		sb.WriteString(s.balanceLine(c.UserID, toID))
		return sb.String(), []auditEntry{{
			Action: "MCP_CREATE_TRANSFER", ResourceType: "transaction", ResourceID: txn.ID,
			Changes: map[string]interface{}{"amount": amount, "from_account_id": fromID, "to_account_id": toID},
		}}, nil
	}}, nil
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
		updates.Amount = &amount
		effectiveAmount = amount
		changes["amount"] = change(before.Amount, amount)
	}
	if updates.Type != nil && *updates.Type != before.Type {
		// Flipping the type reverses the old amount and applies the new one, so
		// the transaction's amount moves the balance twice over. Both sides must
		// be within the per-change limit, not just an amount passed explicitly.
		if res := s.checkCeiling("the existing amount", before.Amount); res != nil {
			return nil, res
		}
		if res := s.checkCeiling("the amount", effectiveAmount); res != nil {
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
			txn.Date.Format("2006-01-02"), txn.Type, formatCents(txn.Amount), untrusted(txn.Description))
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
