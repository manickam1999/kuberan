package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

func (s *Server) registerCategoryTools() {
	s.mcp.AddTool(
		mcp.NewTool("list_categories",
			mcp.WithDescription("List all transaction categories (income and expense types). Limited to the first 100 categories."),
			mcp.WithString("type", mcp.Description("Filter by category type: 'income' or 'expense'")),
		),
		s.handleListCategories,
	)
}

func (s *Server) handleListCategories(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:categories"); denied != nil {
		return denied, nil
	}

	page := pagination.PageRequest{Page: 1, PageSize: 100}
	args := req.GetArguments()
	catType, _ := args["type"].(string)

	var data []models.Category
	var total int64

	if catType == "income" || catType == "expense" {
		result, err := s.services.Categories.GetUserCategoriesByType(userID, models.CategoryType(catType), page)
		if err != nil {
			return mcp.NewToolResultError("failed to list categories"), nil
		}
		data = result.Data
		total = result.TotalItems
	} else {
		result, err := s.services.Categories.GetUserCategories(userID, page)
		if err != nil {
			return mcp.NewToolResultError("failed to list categories"), nil
		}
		data = result.Data
		total = result.TotalItems
	}

	if len(data) == 0 {
		return mcp.NewToolResultText("No categories found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Categories (%d total):\n\n", total))
	for _, c := range data {
		sb.WriteString(fmt.Sprintf("  %-30s %-8s %s\n", c.Name, c.Type, c.Description))
		sb.WriteString(fmt.Sprintf("    ID: %s\n", c.ID))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
