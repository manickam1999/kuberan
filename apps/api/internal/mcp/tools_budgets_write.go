package mcp

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

// budgetsWriteScope gates the budget write tools.
const budgetsWriteScope = "write:budgets"

func (s *Server) registerBudgetWriteTools() {
	s.addWriteTool(
		writeTool("create_budget",
			"Create a monthly or yearly spending budget for an expense category. "+
				"A category can have one active budget per period.",
			false,
			mcp.WithString("category_id", mcp.Required(), mcp.Description("Expense category ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Budget name (max 100 characters)")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Budget limit in dollars, positive")),
			mcp.WithString("period", mcp.Required(), mcp.Enum("monthly", "yearly"), mcp.Description("'monthly' or 'yearly'")),
		),
		budgetsWriteScope, s.prepareCreateBudget,
	)

	s.addWriteTool(
		writeTool("update_budget",
			"Change a budget's name, limit or period, or pause/resume it. Only the fields given change.",
			true,
			mcp.WithString("budget_id", mcp.Required(), mcp.Description("Budget ID")),
			mcp.WithString("name", mcp.Description("New name (max 100 characters)")),
			mcp.WithNumber("amount", mcp.Description("New limit in dollars, positive")),
			mcp.WithString("period", mcp.Enum("monthly", "yearly"), mcp.Description("New period: 'monthly' or 'yearly'")),
			mcp.WithBoolean("is_active", mcp.Description("false pauses the budget, true resumes it")),
		),
		budgetsWriteScope, s.prepareUpdateBudget,
	)
}

// parseBudgetPeriod validates a budget period argument.
func parseBudgetPeriod(v string) (models.BudgetPeriod, *mcp.CallToolResult) {
	switch p := models.BudgetPeriod(v); p {
	case models.BudgetPeriodMonthly, models.BudgetPeriodYearly:
		return p, nil
	default:
		return "", mcp.NewToolResultError("period must be 'monthly' or 'yearly'")
	}
}

func (s *Server) prepareCreateBudget(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	categoryID, res := requiredString(args, "category_id")
	if res != nil {
		return nil, res
	}
	name, res := requiredString(args, "name")
	if res != nil {
		return nil, res
	}
	if res := checkLength("name", name, services.MaxNameLen); res != nil {
		return nil, res
	}
	amount, res := s.requiredAmount(args)
	if res != nil {
		return nil, res
	}
	period, res := parseBudgetPeriod(stringArg(args, "period"))
	if res != nil {
		return nil, res
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		budget, err := s.services.Budgets.CreateBudget(c.UserID, categoryID, name, amount, period)
		if err != nil {
			return "", nil, serviceError("create budget", err)
		}
		return fmt.Sprintf("Created %s budget %s of %s\n  ID: %s\n", budget.Period, untrusted(budget.Name), formatCents(budget.Amount), budget.ID),
			[]auditEntry{{
				Action: "MCP_CREATE_BUDGET", ResourceType: "budget", ResourceID: budget.ID,
				Changes: map[string]interface{}{"name": name, "amount": amount, "period": period, "category_id": categoryID},
			}}, nil
	}}, nil
}

func (s *Server) prepareUpdateBudget(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	budgetID, res := requiredString(args, "budget_id")
	if res != nil {
		return nil, res
	}
	name := stringArg(args, "name")
	if res := checkLength("name", name, services.MaxNameLen); res != nil {
		return nil, res
	}
	amount, amountGiven, res := s.amountArg(args)
	if res != nil {
		return nil, res
	}
	var period *models.BudgetPeriod
	if v := stringArg(args, "period"); v != "" {
		p, res := parseBudgetPeriod(v)
		if res != nil {
			return nil, res
		}
		period = &p
	}
	var isActive *bool
	if v, ok := args["is_active"].(bool); ok {
		isActive = &v
	}
	if name == "" && !amountGiven && period == nil && isActive == nil {
		return nil, mcp.NewToolResultError("nothing to update: give at least one of name, amount, period, is_active")
	}

	before, err := s.services.Budgets.GetBudgetByID(c.UserID, budgetID)
	if err != nil {
		return nil, serviceError("find budget", err)
	}
	changes := map[string]interface{}{}
	var newAmount *int64
	if name != "" {
		changes["name"] = change(before.Name, name)
	}
	if amountGiven {
		newAmount = &amount
		changes["amount"] = change(before.Amount, amount)
	}
	if period != nil {
		changes["period"] = change(before.Period, *period)
	}
	if isActive != nil {
		changes["is_active"] = change(before.IsActive, *isActive)
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		budget, err := s.services.Budgets.UpdateBudget(c.UserID, budgetID, name, newAmount, period, isActive)
		if err != nil {
			return "", nil, serviceError("update budget", err)
		}
		state := "active"
		if !budget.IsActive {
			state = "paused"
		}
		return fmt.Sprintf("Updated %s budget %s: %s (%s)\n  ID: %s\n",
				budget.Period, untrusted(budget.Name), formatCents(budget.Amount), state, budget.ID),
			[]auditEntry{{Action: "MCP_UPDATE_BUDGET", ResourceType: "budget", ResourceID: budget.ID, Changes: changes}}, nil
	}}, nil
}
