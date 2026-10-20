package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
	"kuberan/internal/uuid"
)

// ReceivableHandler handles receivable (money owed back to the user) requests.
type ReceivableHandler struct {
	receivableService services.ReceivableServicer
	auditService      services.AuditServicer
}

// NewReceivableHandler creates a new ReceivableHandler.
func NewReceivableHandler(receivableService services.ReceivableServicer, auditService services.AuditServicer) *ReceivableHandler {
	return &ReceivableHandler{receivableService: receivableService, auditService: auditService}
}

// SharedCounterpartyRequest is one counterparty of a shared expense request.
type SharedCounterpartyRequest struct {
	Name   string `json:"name"`
	Amount *int64 `json:"amount"`
}

// CreateSharedExpenseRequest represents the request payload for creating an expense with receivables.
type CreateSharedExpenseRequest struct {
	AccountID      string                      `json:"account_id" binding:"required"`
	CategoryID     *string                     `json:"category_id"`
	Amount         int64                       `json:"amount" binding:"required,gt=0"`
	Description    string                      `json:"description" binding:"max=500"`
	Date           *string                     `json:"date"`
	Kind           models.ReceivableKind       `json:"kind" binding:"required"`
	Mode           services.ShareMode          `json:"mode" binding:"required,oneof=equal custom full"`
	Counterparties []SharedCounterpartyRequest `json:"counterparties"`
}

// TransactionEnvelope wraps a single transaction in a response.
type TransactionEnvelope struct {
	Transaction models.Transaction `json:"transaction"`
}

// ReceivableEnvelope wraps a single receivable in a response.
type ReceivableEnvelope struct {
	Receivable models.Receivable `json:"receivable"`
}

// SettleResponse is the response of a recorded repayment.
type SettleResponse struct {
	Receivable models.Receivable           `json:"receivable"`
	Settlement models.ReceivableSettlement `json:"settlement"`
}

// CounterpartiesResponse is the autocomplete response.
type CounterpartiesResponse struct {
	Counterparties []services.CounterpartySuggestion `json:"counterparties"`
}

// RenameCounterpartyResponse reports how many receivables were renamed.
type RenameCounterpartyResponse struct {
	Updated int64 `json:"updated"`
}

// CreateSharedExpense creates an expense together with the amounts owed back by counterparties.
// @Summary     Create a shared expense
// @Description Create an expense whose full amount leaves the account, with receivables for each counterparty. Only the user's own share counts as spending.
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateSharedExpenseRequest true "Shared expense details"
// @Success     201 {object} TransactionEnvelope "Expense created with receivables"
// @Failure     400 {object} ErrorResponse "Invalid input, kind or shares"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Account or category not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/shared [post]
func (h *ReceivableHandler) CreateSharedExpense(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req CreateSharedExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	if !uuid.IsValid(req.AccountID) {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid account_id format"))
		return
	}
	categoryID, err := optionalUUID(req.CategoryID, "category_id")
	if err != nil {
		respondWithError(c, err)
		return
	}
	date, err := parseOptionalDate(req.Date)
	if err != nil {
		respondWithError(c, err)
		return
	}

	counterparties := make([]services.SharedCounterpartyInput, len(req.Counterparties))
	for i, cp := range req.Counterparties {
		counterparties[i] = services.SharedCounterpartyInput{Name: cp.Name, Amount: cp.Amount}
	}

	transaction, err := h.receivableService.CreateSharedExpense(userID, services.CreateSharedExpenseInput{
		AccountID:      req.AccountID,
		CategoryID:     categoryID,
		Amount:         req.Amount,
		Description:    req.Description,
		Date:           date,
		Kind:           req.Kind,
		Mode:           req.Mode,
		Counterparties: counterparties,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "CREATE_SHARED_EXPENSE", "transaction", transaction.ID, c.ClientIP(),
		map[string]interface{}{
			"account_id":     req.AccountID,
			"amount":         req.Amount,
			"kind":           req.Kind,
			"mode":           req.Mode,
			"counterparties": len(req.Counterparties),
		})

	c.JSON(http.StatusCreated, gin.H{"transaction": transaction})
}

// ReceivableShareRequest is one entry of the receivables diff payload.
type ReceivableShareRequest struct {
	ID     *string `json:"id"`
	Name   string  `json:"name"`
	Amount int64   `json:"amount"`
}

// UpdateReceivablesRequest represents the request payload for editing an expense's receivables.
type UpdateReceivablesRequest struct {
	Kind        *models.ReceivableKind   `json:"kind"`
	Receivables []ReceivableShareRequest `json:"receivables" binding:"required"`
}

// UpdateReceivables edits the receivables of an expense by diff.
// @Summary     Update an expense's receivables
// @Description Diff the receivables of an expense: entries with an id are updated, entries without are inserted, omitted unpaid receivables are removed (a paid one returns 409).
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path string                   true "Transaction ID"
// @Param       request body UpdateReceivablesRequest true "Receivables diff"
// @Success     200 {object} TransactionEnvelope "Updated expense"
// @Failure     400 {object} ErrorResponse "Invalid input, kind or shares"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Transaction or receivable not found"
// @Failure     409 {object} ErrorResponse "Receivable has payments"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /transactions/{id}/receivables [put]
func (h *ReceivableHandler) UpdateReceivables(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	txID, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req UpdateReceivablesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	shares := make([]services.ReceivableShareInput, len(req.Receivables))
	for i, r := range req.Receivables {
		id, idErr := optionalUUID(r.ID, "receivable id")
		if idErr != nil {
			respondWithError(c, idErr)
			return
		}
		shares[i] = services.ReceivableShareInput{ID: id, Name: r.Name, Amount: r.Amount}
	}

	transaction, err := h.receivableService.UpdateReceivables(userID, txID, services.UpdateReceivablesInput{
		Kind:        req.Kind,
		Receivables: shares,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UPDATE_RECEIVABLES", "transaction", txID, c.ClientIP(),
		map[string]interface{}{"receivables": len(req.Receivables)})

	c.JSON(http.StatusOK, gin.H{"transaction": transaction})
}

// ListReceivables returns a page of the user's receivables.
// @Summary     List receivables
// @Description Get a paginated list of receivables ordered by expense date (newest first), each with its expense
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       status       query string false "pending, partially_paid, settled, written_off, or open (pending + partially_paid)"
// @Param       kind         query string false "split, on_behalf, claim, loan, deposit or refund"
// @Param       counterparty query string false "Counterparty key"
// @Param       page         query int    false "Page number (default 1)"
// @Param       page_size    query int    false "Items per page (default 20, max 100)"
// @Success     200 {object} pagination.PageResponse[models.Receivable] "Paginated receivables"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables [get]
func (h *ReceivableHandler) ListReceivables(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	result, err := h.receivableService.ListReceivables(userID, page, services.ReceivableFilter{
		Status:          models.ReceivableStatus(c.Query("status")),
		Kind:            models.ReceivableKind(c.Query("kind")),
		CounterpartyKey: c.Query("counterparty"),
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetReceivablesSummary returns open receivable totals per kind and counterparty.
// @Summary     Receivables summary
// @Description Totals of open receivables per kind and per counterparty, plus a grand total
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} services.ReceivableSummary "Receivables summary"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/summary [get]
func (h *ReceivableHandler) GetReceivablesSummary(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	summary, err := h.receivableService.Summary(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, summary)
}

// ListCounterparties returns counterparty autocomplete suggestions.
// @Summary     Counterparty autocomplete
// @Description Distinct counterparties whose key contains q, most recently used first (max 20)
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       q query string false "Search text"
// @Success     200 {object} CounterpartiesResponse "Counterparty suggestions"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/counterparties [get]
func (h *ReceivableHandler) ListCounterparties(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	suggestions, err := h.receivableService.ListCounterparties(userID, c.Query("q"))
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"counterparties": suggestions})
}

// RenameCounterpartyRequest represents the request payload for renaming or merging a counterparty.
type RenameCounterpartyRequest struct {
	FromKey string `json:"from_key" binding:"required"`
	ToName  string `json:"to_name" binding:"required"`
}

// RenameCounterparty renames (or merges) a counterparty across all receivables.
// @Summary     Rename a counterparty
// @Description Rename every receivable of a counterparty key. Renaming onto an existing key merges them.
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body RenameCounterpartyRequest true "Rename details"
// @Success     200 {object} RenameCounterpartyResponse "Number of receivables updated"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/counterparties/rename [post]
func (h *ReceivableHandler) RenameCounterparty(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req RenameCounterpartyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	updated, err := h.receivableService.RenameCounterparty(userID, req.FromKey, req.ToName)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "RENAME_COUNTERPARTY", "receivable", "", c.ClientIP(),
		map[string]interface{}{"from_key": req.FromKey, "to_name": req.ToName, "updated": updated})

	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

// GetReceivable returns one receivable with its settlements and expense.
// @Summary     Get a receivable
// @Description Get a receivable with its settlements and expense
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Receivable ID"
// @Success     200 {object} ReceivableEnvelope "Receivable details"
// @Failure     400 {object} ErrorResponse "Invalid receivable ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Receivable not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/{id} [get]
func (h *ReceivableHandler) GetReceivable(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	receivable, err := h.receivableService.GetReceivable(userID, id)
	if err != nil {
		respondWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"receivable": receivable})
}

// SettleReceivableRequest represents the request payload for recording a repayment.
type SettleReceivableRequest struct {
	AccountID         string  `json:"account_id" binding:"required"`
	Amount            *int64  `json:"amount" binding:"required"`
	Date              *string `json:"date"`
	Description       string  `json:"description" binding:"max=500"`
	WriteOffRemainder bool    `json:"write_off_remainder"`
}

// SettleReceivable records a repayment against a receivable.
// @Summary     Record a repayment
// @Description Credit an account with a repayment (a reimbursement, not income) and reduce the receivable's outstanding balance
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path string                  true "Receivable ID"
// @Param       request body SettleReceivableRequest true "Repayment details"
// @Success     201 {object} SettleResponse "Repayment recorded"
// @Failure     400 {object} ErrorResponse "Invalid input, amount over outstanding or invalid account type"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Receivable or account not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/{id}/settle [post]
func (h *ReceivableHandler) SettleReceivable(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req SettleReceivableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	if !uuid.IsValid(req.AccountID) {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "Invalid account_id format"))
		return
	}
	date, err := parseOptionalDate(req.Date)
	if err != nil {
		respondWithError(c, err)
		return
	}

	receivable, settlement, err := h.receivableService.Settle(userID, id, services.SettleInput{
		AccountID:         req.AccountID,
		Amount:            *req.Amount,
		Date:              date,
		Description:       req.Description,
		WriteOffRemainder: req.WriteOffRemainder,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "SETTLE_RECEIVABLE", "receivable", id, c.ClientIP(),
		map[string]interface{}{
			"settlement_id":       settlement.ID,
			"account_id":          req.AccountID,
			"amount":              *req.Amount,
			"write_off_remainder": req.WriteOffRemainder,
		})

	c.JSON(http.StatusCreated, gin.H{"receivable": receivable, "settlement": settlement})
}

// DeleteSettlement reverses a repayment.
// @Summary     Delete a repayment
// @Description Reverse a repayment: the reimbursement is removed, the account is debited and the receivable's outstanding balance is restored
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Settlement ID"
// @Success     200 {object} ReceivableEnvelope "Updated receivable"
// @Failure     400 {object} ErrorResponse "Invalid settlement ID"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Settlement not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/settlements/{id} [delete]
func (h *ReceivableHandler) DeleteSettlement(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	receivable, err := h.receivableService.DeleteSettlement(userID, id)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "DELETE_SETTLEMENT", "receivable", receivable.ID, c.ClientIP(),
		map[string]interface{}{"settlement_id": id})

	c.JSON(http.StatusOK, gin.H{"receivable": receivable})
}

// WriteOffRequest represents the optional request payload for a write-off.
type WriteOffRequest struct {
	Amount *int64 `json:"amount"`
}

// WriteOffReceivable forgives (part of) a receivable's outstanding balance.
// @Summary     Write off a receivable
// @Description Forgive an amount (default: the whole outstanding balance). The forgiven amount becomes the user's spending on the original expense.
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id      path string          true  "Receivable ID"
// @Param       request body WriteOffRequest false "Write-off details"
// @Success     200 {object} ReceivableEnvelope "Updated receivable"
// @Failure     400 {object} ErrorResponse "Invalid input or amount over outstanding"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Receivable not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/{id}/write-off [post]
func (h *ReceivableHandler) WriteOffReceivable(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	var req WriteOffRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	receivable, err := h.receivableService.WriteOff(userID, id, req.Amount)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "WRITE_OFF_RECEIVABLE", "receivable", id, c.ClientIP(),
		map[string]interface{}{"written_off_amount": receivable.WrittenOffAmount})

	c.JSON(http.StatusOK, gin.H{"receivable": receivable})
}

// UndoWriteOff resets a receivable's write-off.
// @Summary     Undo a write-off
// @Description Reset the receivable's written-off amount to zero
// @Tags        receivables
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Receivable ID"
// @Success     200 {object} ReceivableEnvelope "Updated receivable"
// @Failure     400 {object} ErrorResponse "Nothing to undo"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Failure     404 {object} ErrorResponse "Receivable not found"
// @Failure     500 {object} ErrorResponse "Server error"
// @Router      /receivables/{id}/write-off [delete]
func (h *ReceivableHandler) UndoWriteOff(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	id, err := parsePathID(c, "id")
	if err != nil {
		respondWithError(c, err)
		return
	}

	receivable, err := h.receivableService.UndoWriteOff(userID, id)
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(userID, "UNDO_WRITE_OFF", "receivable", id, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{"receivable": receivable})
}
