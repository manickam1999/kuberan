package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

func (s *Server) registerReceivableWriteTools() {
	s.addWriteTool(
		writeTool("record_repayment",
			"Record money received back for a receivable (for example a friend repaying their share of a bill). "+
				"It credits the chosen account with a repayment, which is not income. Get receivable IDs from list_receivables. "+
				"amount is in the receivable's currency (shown by list_receivables). When the receiving account is in a "+
				"different currency, account_amount (what actually arrived, in the account's currency) is required.",
			false,
			mcp.WithString("receivable_id", mcp.Required(), mcp.Description("Receivable ID")),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account the repayment arrived in (cash, credit card or debt)")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Amount repaid, in the receivable's currency, as a decimal like 12.50; at most the outstanding amount")),
			mcp.WithNumber("account_amount", mcp.Description("Amount credited to the account, in the account's currency, as a decimal. Required when the account's currency differs from the receivable's; omit (or equal to amount) otherwise")),
			mcp.WithString("date", mcp.Required(), mcp.Description(dateArgDescription)),
			mcp.WithString("description", mcp.Description("Description (default: 'Repayment from <name>: <expense>')")),
		),
		transactionsWriteScope, s.prepareRecordRepayment,
	)

	s.addWriteTool(
		writeTool("write_off_receivable",
			"Forgive some or all of what is still owed on a receivable. The forgiven amount becomes the user's own spending "+
				"on the original expense. The amount is in the receivable's currency. It can be undone in the Kuberan app.",
			true,
			mcp.WithString("receivable_id", mcp.Required(), mcp.Description("Receivable ID")),
			mcp.WithNumber("amount", mcp.Description("Amount to forgive, in the receivable's currency, as a decimal like 12.50 (default: the whole outstanding amount)")),
		),
		transactionsWriteScope, s.prepareWriteOffReceivable,
	)
}

// receivableLine summarizes a receivable after a write.
func receivableLine(r *models.Receivable) string {
	return fmt.Sprintf("  %s: paid %s, written off %s, outstanding %s (status: %s)\n",
		untrusted(r.CounterpartyName), formatMoney(r.PaidAmount, r.Currency), formatMoney(r.WrittenOffAmount, r.Currency),
		formatMoney(r.OutstandingAmount(), r.Currency), r.Status)
}

func (s *Server) prepareRecordRepayment(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	receivableID, res := requiredString(args, "receivable_id")
	if res != nil {
		return nil, res
	}
	accountID, res := requiredString(args, "account_id")
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

	receivable, err := s.services.Receivables.GetReceivable(c.UserID, receivableID)
	if err != nil {
		return nil, serviceError("find receivable", err)
	}
	account, err := s.services.Accounts.GetAccountByID(c.UserID, accountID)
	if err != nil {
		return nil, serviceError("find account", err)
	}
	recCurrency := receivable.Currency
	if recCurrency == "" {
		recCurrency = account.Currency
	}

	// account_amount follows the API's rules: required when the account's
	// currency differs from the receivable's, otherwise omitted or equal to
	// amount. Both amounts count against the ceiling.
	accountAmount, accountPresent, res := moneyArg(args, "account_amount")
	if res != nil {
		return nil, res
	}
	sameCurrency := strings.EqualFold(recCurrency, account.Currency)
	switch {
	case !sameCurrency && !accountPresent:
		return nil, mcp.NewToolResultError(s.amountRequiredText("account_amount", recCurrency, account.Currency, amount, date,
			"receivable and the receiving account are"))
	case sameCurrency && accountPresent && accountAmount != amount:
		return nil, mcp.NewToolResultError(fmt.Sprintf(
			"account_amount must be omitted or equal amount: the receivable and the account are both in %s", currencyCode(recCurrency)))
	}
	if res := s.checkCeiling(c.UserID, "amount", amount, recCurrency); res != nil {
		return nil, res
	}
	var accountAmountArg *int64
	if accountPresent && !sameCurrency {
		if res := s.checkCeiling(c.UserID, "account_amount", accountAmount, account.Currency); res != nil {
			return nil, res
		}
		accountAmountArg = &accountAmount
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		rec, settlement, err := s.services.Receivables.Settle(c.UserID, receivableID, services.SettleInput{
			AccountID:     accountID,
			Amount:        amount,
			AccountAmount: accountAmountArg,
			Date:          date,
			Description:   description,
		})
		if err != nil {
			return "", nil, serviceError("record repayment", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Recorded repayment of %s on %s\n", formatMoney(settlement.Amount, rec.Currency), settlement.Date.Format("2006-01-02"))
		if settlement.AccountAmount != nil {
			fmt.Fprintf(&sb, "  Credited to the account: %s\n", formatMoney(*settlement.AccountAmount, account.Currency))
		}
		sb.WriteString(receivableLine(rec))
		fmt.Fprintf(&sb, "  Repayment transaction ID: %s\n", settlement.TransactionID)
		sb.WriteString(s.balanceLine(c.UserID, accountID))
		changes := map[string]interface{}{
			"amount": amount, "account_id": accountID, "currency": currencyCode(rec.Currency),
			"settlement_id": settlement.ID, "transaction_id": settlement.TransactionID,
		}
		if accountAmountArg != nil {
			changes["account_amount"] = accountAmount
		}
		return sb.String(), []auditEntry{{
			Action: "MCP_RECORD_REPAYMENT", ResourceType: "receivable", ResourceID: rec.ID, Changes: changes,
		}}, nil
	}}, nil
}

func (s *Server) prepareWriteOffReceivable(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	receivableID, res := requiredString(args, "receivable_id")
	if res != nil {
		return nil, res
	}
	before, err := s.services.Receivables.GetReceivable(c.UserID, receivableID)
	if err != nil {
		return nil, serviceError("find receivable", err)
	}

	// Resolve the default (the whole outstanding balance) here rather than in
	// the service, so the per-change limit applies to it too.
	amount, present, res := s.amountArg(args)
	if res != nil {
		return nil, res
	}
	if present {
		if res := s.checkCeiling(c.UserID, "amount", amount, before.Currency); res != nil {
			return nil, res
		}
	} else {
		amount = before.OutstandingAmount()
		if amount <= 0 {
			return nil, mcp.NewToolResultError("nothing to write off: this receivable has no outstanding amount")
		}
		if res := s.checkCeiling(c.UserID, "the outstanding amount", amount, before.Currency); res != nil {
			return nil, res
		}
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		rec, err := s.services.Receivables.WriteOff(c.UserID, receivableID, &amount)
		if err != nil {
			return "", nil, serviceError("write off receivable", err)
		}
		return "Wrote off receivable\n" + receivableLine(rec), []auditEntry{{
			Action: "MCP_WRITE_OFF_RECEIVABLE", ResourceType: "receivable", ResourceID: rec.ID,
			Changes: map[string]interface{}{
				"amount":             amount,
				"currency":           currencyCode(rec.Currency),
				"written_off_amount": change(before.WrittenOffAmount, rec.WrittenOffAmount),
				"status":             change(before.Status, rec.Status),
			},
		}}, nil
	}}, nil
}
