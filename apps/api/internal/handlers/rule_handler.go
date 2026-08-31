package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// RuleHandler handles transaction-rule requests (plan 018). It holds both the
// rule service (CRUD/reorder) and the transaction service (preview/apply, which
// read and write the transactions table).
type RuleHandler struct {
	ruleService        services.RuleServicer
	transactionService services.TransactionServicer
	auditService       services.AuditServicer
}

// NewRuleHandler creates a new RuleHandler.
func NewRuleHandler(ruleService services.RuleServicer, transactionService services.TransactionServicer, auditService services.AuditServicer) *RuleHandler {
	return &RuleHandler{ruleService: ruleService, transactionService: transactionService, auditService: auditService}
}

// RuleConditionDTO is a single AND-ed matching clause in a rule request.
type RuleConditionDTO struct {
	Field     string `json:"field" binding:"required"`
	Operator  string `json:"operator" binding:"required"`
	ValueText string `json:"value_text"`
	AmountMin *int64 `json:"amount_min"`
	AmountMax *int64 `json:"amount_max"`
}

// RuleActionDTO is a single action in a rule request.
type RuleActionDTO struct {
	ActionType string  `json:"action_type" binding:"required"`
	CategoryID *string `json:"category_id"`
	ValueText  string  `json:"value_text"`
}

// CreateRuleRequest is the payload for creating a rule.
type CreateRuleRequest struct {
	Name       string             `json:"name" binding:"required,min=1,max=120"`
	Priority   int                `json:"priority"`
	IsActive   *bool              `json:"is_active"`
	Conditions []RuleConditionDTO `json:"conditions" binding:"required,min=1,dive"`
	Actions    []RuleActionDTO    `json:"actions" binding:"required,min=1,dive"`
}

// UpdateRuleRequest is the payload for updating a rule. Omitted conditions/actions
// are left unchanged; provided ones replace the rule's children wholesale.
type UpdateRuleRequest struct {
	Name       *string            `json:"name" binding:"omitempty,min=1,max=120"`
	Priority   *int               `json:"priority"`
	IsActive   *bool              `json:"is_active"`
	Conditions []RuleConditionDTO `json:"conditions" binding:"omitempty,min=1,dive"`
	Actions    []RuleActionDTO    `json:"actions" binding:"omitempty,min=1,dive"`
}

// ReorderRulesRequest is the payload for reordering rules.
type ReorderRulesRequest struct {
	RuleIDs []string `json:"rule_ids" binding:"required"`
}

// PreviewRuleRequest is the payload for previewing matches of unsaved conditions.
type PreviewRuleRequest struct {
	Conditions []RuleConditionDTO `json:"conditions" binding:"required,min=1,dive"`
}

// ApplyRuleRequest is the payload for backfilling a rule over existing transactions.
type ApplyRuleRequest struct {
	Scope     string `json:"scope"`     // uncategorized | all (default uncategorized)
	Overwrite bool   `json:"overwrite"` // only meaningful with scope=all
	DryRun    *bool  `json:"dry_run"`   // default true
}

func toConditionInputs(dtos []RuleConditionDTO) []services.RuleConditionInput {
	if dtos == nil {
		return nil
	}
	out := make([]services.RuleConditionInput, len(dtos))
	for i, d := range dtos {
		out[i] = services.RuleConditionInput{
			Field:     models.RuleField(d.Field),
			Operator:  models.RuleOperator(d.Operator),
			ValueText: d.ValueText,
			AmountMin: d.AmountMin,
			AmountMax: d.AmountMax,
		}
	}
	return out
}

func toActionInputs(dtos []RuleActionDTO) []services.RuleActionInput {
	if dtos == nil {
		return nil
	}
	out := make([]services.RuleActionInput, len(dtos))
	for i, d := range dtos {
		out[i] = services.RuleActionInput{
			ActionType: models.RuleActionType(d.ActionType),
			CategoryID: d.CategoryID,
			ValueText:  d.ValueText,
		}
	}
	return out
}

// CreateRule handles creating a new rule.
// @Summary     Create a transaction rule
// @Description Create an auto-categorization rule (conditions AND-ed, first-match-wins)
// @Tags        rules
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateRuleRequest true "Rule details"
// @Success     201 {object} map[string]models.TransactionRule "Rule created"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Target category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /rules [post]
func (h *RuleHandler) CreateRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	rule, err := h.ruleService.CreateRule(userID, services.CreateRuleInput{
		Name:       req.Name,
		Priority:   req.Priority,
		IsActive:   req.IsActive,
		Conditions: toConditionInputs(req.Conditions),
		Actions:    toActionInputs(req.Actions),
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_RULE", "rule", rule.ID, c.ClientIP(),
		map[string]interface{}{"name": rule.Name})

	c.JSON(http.StatusCreated, gin.H{"rule": rule})
}

// GetRules handles listing the user's rules in evaluation order.
// @Summary     List transaction rules
// @Description List all rules for the authenticated user in evaluation order
// @Tags        rules
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string][]models.TransactionRule "Rules"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /rules [get]
func (h *RuleHandler) GetRules(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	rules, err := h.ruleService.ListRules(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// GetRule handles retrieving a single rule.
// @Summary     Get a transaction rule
// @Tags        rules
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Rule ID"
// @Success     200 {object} map[string]models.TransactionRule "Rule"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Rule not found"
// @Router      /rules/{id} [get]
func (h *RuleHandler) GetRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	ruleID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	rule, err := h.ruleService.GetRule(userID, ruleID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// UpdateRule handles updating a rule.
// @Summary     Update a transaction rule
// @Tags        rules
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path string            true "Rule ID"
// @Param       request body UpdateRuleRequest true "Updated rule"
// @Success     200 {object} map[string]models.TransactionRule "Updated rule"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Rule not found"
// @Router      /rules/{id} [put]
func (h *RuleHandler) UpdateRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	ruleID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	rule, err := h.ruleService.UpdateRule(userID, ruleID, services.UpdateRuleInput{
		Name:       req.Name,
		Priority:   req.Priority,
		IsActive:   req.IsActive,
		Conditions: toConditionInputs(req.Conditions),
		Actions:    toActionInputs(req.Actions),
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_RULE", "rule", ruleID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// DeleteRule handles deleting a rule.
// @Summary     Delete a transaction rule
// @Tags        rules
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Rule ID"
// @Success     200 {object} MessageResponse "Rule deleted"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Rule not found"
// @Router      /rules/{id} [delete]
func (h *RuleHandler) DeleteRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	ruleID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.ruleService.DeleteRule(userID, ruleID); err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_RULE", "rule", ruleID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted successfully"})
}

// ReorderRules handles rewriting rule priorities to a new order.
// @Summary     Reorder transaction rules
// @Tags        rules
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body ReorderRulesRequest true "Ordered rule IDs"
// @Success     200 {object} map[string][]models.TransactionRule "Reordered rules"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /rules/reorder [post]
func (h *RuleHandler) ReorderRules(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req ReorderRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	rules, err := h.ruleService.ReorderRules(userID, req.RuleIDs)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "REORDER_RULES", "rule", "", c.ClientIP(),
		map[string]interface{}{"count": len(req.RuleIDs)})

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// PreviewRule handles counting existing transactions matching unsaved conditions.
// @Summary     Preview rule matches
// @Description Count existing transactions matching a set of unsaved conditions
// @Tags        rules
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body PreviewRuleRequest true "Conditions to preview"
// @Success     200 {object} services.RuleMatchPreview "Match preview"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /rules/preview [post]
func (h *RuleHandler) PreviewRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req PreviewRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	preview, err := h.transactionService.PreviewRuleMatches(userID, toConditionInputs(req.Conditions))
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, preview)
}

// ApplyRule handles backfilling a rule over existing transactions.
// @Summary     Apply a rule to existing transactions
// @Description Backfill a rule over existing transactions (dry-run by default)
// @Tags        rules
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path string          true  "Rule ID"
// @Param       request body ApplyRuleRequest false "Backfill options"
// @Success     200 {object} services.ApplyRuleResult "Backfill result"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Rule not found"
// @Router      /rules/{id}/apply [post]
func (h *RuleHandler) ApplyRule(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	ruleID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req ApplyRuleRequest
	// Body is optional; defaults apply when absent.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
			return
		}
	}

	scope := services.RuleApplyScopeUncategorized
	switch req.Scope {
	case "", string(services.RuleApplyScopeUncategorized):
		scope = services.RuleApplyScopeUncategorized
	case string(services.RuleApplyScopeAll):
		scope = services.RuleApplyScopeAll
	default:
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "scope must be 'uncategorized' or 'all'"))
		return
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	result, err := h.transactionService.ApplyRule(userID, ruleID, services.ApplyRuleOptions{
		Scope:     scope,
		Overwrite: req.Overwrite,
		DryRun:    dryRun,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	if !dryRun {
		h.auditService.Log(userID, "APPLY_RULE", "rule", ruleID, c.ClientIP(),
			map[string]interface{}{"scope": string(scope), "applied": result.Applied})
	}

	c.JSON(http.StatusOK, result)
}
