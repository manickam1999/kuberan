package mcp

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/validator"
)

// categoriesWriteScope gates the category write tools.
const categoriesWriteScope = "write:categories"

func (s *Server) registerCategoryWriteTools() {
	s.addWriteTool(
		writeTool("create_category",
			"Create an income or expense category. Check list_categories first to avoid duplicates.",
			false,
			mcp.WithString("name", mcp.Required(), mcp.Description("Category name (max 100 characters)")),
			mcp.WithString("type", mcp.Required(), mcp.Enum("income", "expense"), mcp.Description("'income' or 'expense'")),
			mcp.WithString("description", mcp.Description("Description (max 500 characters)")),
			mcp.WithString("parent_id", mcp.Description("Parent category ID, to create a subcategory")),
			mcp.WithString("icon", mcp.Description("Icon name (max 50 characters)")),
			mcp.WithString("color", mcp.Description("Color as #RGB or #RRGGBB")),
		),
		categoriesWriteScope, s.prepareCreateCategory,
	)

	s.addWriteTool(
		writeTool("update_category",
			"Rename or edit a category. Only the fields given change.",
			true,
			mcp.WithString("category_id", mcp.Required(), mcp.Description("Category ID")),
			mcp.WithString("name", mcp.Description("New name (max 100 characters)")),
			mcp.WithString("description", mcp.Description("New description (max 500 characters)")),
			mcp.WithString("parent_id", mcp.Description("New parent category ID")),
			mcp.WithString("icon", mcp.Description("New icon name (max 50 characters)")),
			mcp.WithString("color", mcp.Description("New color as #RGB or #RRGGBB")),
		),
		categoriesWriteScope, s.prepareUpdateCategory,
	)
}

// parseCategoryType validates a category type argument.
func parseCategoryType(v string) (models.CategoryType, *mcp.CallToolResult) {
	switch t := models.CategoryType(v); t {
	case models.CategoryTypeIncome, models.CategoryTypeExpense:
		return t, nil
	default:
		return "", mcp.NewToolResultError("type must be 'income' or 'expense'")
	}
}

// categoryFields holds the optional category arguments shared by create and
// update, validated like the REST API.
type categoryFields struct {
	Name, Description, Icon, Color string
	ParentID                       *string
}

func parseCategoryFields(args map[string]interface{}) (categoryFields, *mcp.CallToolResult) {
	f := categoryFields{
		Name:  stringArg(args, "name"),
		Icon:  stringArg(args, "icon"),
		Color: stringArg(args, "color"),
	}
	if res := checkLength("name", f.Name, services.MaxNameLen); res != nil {
		return f, res
	}
	if res := checkLength("icon", f.Icon, services.MaxIconLen); res != nil {
		return f, res
	}
	if f.Color != "" && !validator.IsHexColor(f.Color) {
		return f, mcp.NewToolResultError("color must be #RGB or #RRGGBB")
	}
	description, res := descriptionArg(args)
	if res != nil {
		return f, res
	}
	f.Description = description
	if v := stringArg(args, "parent_id"); v != "" {
		f.ParentID = &v
	}
	return f, nil
}

func (s *Server) prepareCreateCategory(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	f, res := parseCategoryFields(args)
	if res != nil {
		return nil, res
	}
	if f.Name == "" {
		return nil, mcp.NewToolResultError("name is required")
	}
	catType, res := parseCategoryType(stringArg(args, "type"))
	if res != nil {
		return nil, res
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		category, err := s.services.Categories.CreateCategory(c.UserID, f.Name, catType, f.Description, f.Icon, f.Color, f.ParentID)
		if err != nil {
			return "", nil, serviceError("create category", err)
		}
		return fmt.Sprintf("Created %s category %s\n  ID: %s\n", category.Type, untrusted(category.Name), category.ID),
			[]auditEntry{{
				Action: "MCP_CREATE_CATEGORY", ResourceType: "category", ResourceID: category.ID,
				Changes: map[string]interface{}{"name": category.Name, "type": category.Type},
			}}, nil
	}}, nil
}

func (s *Server) prepareUpdateCategory(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult) {
	categoryID, res := requiredString(args, "category_id")
	if res != nil {
		return nil, res
	}
	f, res := parseCategoryFields(args)
	if res != nil {
		return nil, res
	}
	if f.Name == "" && f.Description == "" && f.Icon == "" && f.Color == "" && f.ParentID == nil {
		return nil, mcp.NewToolResultError("nothing to update: give at least one of name, description, parent_id, icon, color")
	}
	before, err := s.services.Categories.GetCategoryByID(c.UserID, categoryID)
	if err != nil {
		return nil, serviceError("find category", err)
	}

	return &writeOp{Cost: 1, Run: func() (string, []auditEntry, *mcp.CallToolResult) {
		category, err := s.services.Categories.UpdateCategory(c.UserID, categoryID, f.Name, f.Description, f.Icon, f.Color, f.ParentID)
		if err != nil {
			return "", nil, serviceError("update category", err)
		}
		changes := map[string]interface{}{}
		if f.Name != "" {
			changes["name"] = change(before.Name, category.Name)
		}
		if f.Description != "" {
			changes["description"] = change(before.Description, category.Description)
		}
		if f.Icon != "" {
			changes["icon"] = change(before.Icon, category.Icon)
		}
		if f.Color != "" {
			changes["color"] = change(before.Color, category.Color)
		}
		if f.ParentID != nil {
			changes["parent_id"] = change(before.ParentID, *f.ParentID)
		}
		return fmt.Sprintf("Updated %s category %s\n  ID: %s\n", category.Type, untrusted(category.Name), category.ID),
			[]auditEntry{{Action: "MCP_UPDATE_CATEGORY", ResourceType: "category", ResourceID: category.ID, Changes: changes}}, nil
	}}, nil
}
