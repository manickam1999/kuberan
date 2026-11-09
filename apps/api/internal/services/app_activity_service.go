package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// maxSummaryText is how many characters of user-written text (a description,
// a name) a summary carries.
const maxSummaryText = 80

// unnamedApp is shown for activity whose grant no longer exists.
const unnamedApp = "an app"

// AppActivityItem is one change a connected app made, as shown under
// Settings > Connected apps. Summary is built by the server from a fixed
// template per action and is plain text: clients must never render it as HTML.
type AppActivityItem struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Summary      string    `json:"summary"`
}

// AllAppActivityItem is an AppActivityItem tagged with the app that made it.
type AllAppActivityItem struct {
	AppActivityItem
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`
}

// AppActivityServicer lists what connected apps changed (plan 023, 1.6).
type AppActivityServicer interface {
	// ListByClient returns one app's changes for the user, newest first.
	ListByClient(userID, clientID string, page pagination.PageRequest) (*pagination.PageResponse[AppActivityItem], error)
	// ListAll returns the changes of all the user's apps, newest first.
	ListAll(userID string, page pagination.PageRequest) (*pagination.PageResponse[AllAppActivityItem], error)
}

type appActivityService struct {
	db *gorm.DB
}

// NewAppActivityService creates a new AppActivityServicer.
func NewAppActivityService(db *gorm.DB) AppActivityServicer {
	return &appActivityService{db: db}
}

// ListByClient implements AppActivityServicer.
func (s *appActivityService) ListByClient(userID, clientID string, page pagination.PageRequest) (*pagination.PageResponse[AppActivityItem], error) {
	if userID == "" || clientID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "user_id and client_id are required")
	}
	rows, total, err := s.query(userID, clientID, &page)
	if err != nil {
		return nil, err
	}
	summaries, err := s.summaries(userID, rows)
	if err != nil {
		return nil, err
	}
	items := make([]AppActivityItem, len(rows))
	for i := range rows {
		items[i] = activityItem(&rows[i], summaries[i])
	}
	res := pagination.NewPageResponse(items, page.Page, page.PageSize, total)
	return &res, nil
}

// ListAll implements AppActivityServicer.
func (s *appActivityService) ListAll(userID string, page pagination.PageRequest) (*pagination.PageResponse[AllAppActivityItem], error) {
	if userID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "user_id is required")
	}
	rows, total, err := s.query(userID, "", &page)
	if err != nil {
		return nil, err
	}
	summaries, err := s.summaries(userID, rows)
	if err != nil {
		return nil, err
	}
	names, err := s.clientNames(userID, rows)
	if err != nil {
		return nil, err
	}
	items := make([]AllAppActivityItem, len(rows))
	for i := range rows {
		name := names[rows[i].ClientID]
		if name == "" {
			name = unnamedApp
		}
		items[i] = AllAppActivityItem{
			AppActivityItem: activityItem(&rows[i], summaries[i]),
			ClientID:        rows[i].ClientID,
			ClientName:      name,
		}
	}
	res := pagination.NewPageResponse(items, page.Page, page.PageSize, total)
	return &res, nil
}

// query returns one page of the user's MCP audit rows (of one client when
// clientID is set) and the total count. Every query is scoped by user_id.
func (s *appActivityService) query(userID, clientID string, page *pagination.PageRequest) ([]models.AuditLog, int64, error) {
	page.Defaults()
	q := s.db.Model(&models.AuditLog{}).
		Where("user_id = ? AND source = ?", userID, models.AuditSourceMCP)
	if clientID != "" {
		q = q.Where("client_id = ?", clientID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	var rows []models.AuditLog
	if err := q.Scopes(pagination.Paginate(*page)).
		Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return rows, total, nil
}

func activityItem(r *models.AuditLog, summary string) AppActivityItem {
	item := AppActivityItem{
		ID:           r.ID,
		CreatedAt:    r.CreatedAt,
		Action:       r.Action,
		ResourceType: r.ResourceType,
		Summary:      summary,
	}
	if r.ResourceID != nil {
		item.ResourceID = *r.ResourceID
	}
	return item
}

// clientNames maps the client ids in rows to the user's grant names.
func (s *appActivityService) clientNames(userID string, rows []models.AuditLog) (map[string]string, error) {
	seen := map[string]bool{}
	var ids []string
	for i := range rows {
		if id := rows[i].ClientID; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	names := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var grants []models.OAuthClientGrant
	if err := s.db.Select("client_id", "client_name").
		Where("user_id = ? AND client_id IN ?", userID, ids).Find(&grants).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for i := range grants {
		names[grants[i].ClientID] = grants[i].ClientName
	}
	return names, nil
}

// txnInfo is what a summary borrows from the transaction an audit row is about.
type txnInfo struct {
	Description string
	Currency    string
}

// loadTransactions fetches the description and account currency of the
// transactions the given rows refer to. It is unscoped for deleted rows (a
// transaction may have been deleted since) but always scoped to the user.
func (s *appActivityService) loadTransactions(userID string, rows []models.AuditLog) (map[string]txnInfo, error) {
	var ids []string
	for i := range rows {
		if rows[i].ResourceType == "transaction" && rows[i].ResourceID != nil {
			ids = append(ids, *rows[i].ResourceID)
		}
	}
	out := make(map[string]txnInfo, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		ID          string
		Description string
		Currency    string
	}
	var found []row
	err := s.db.Unscoped().Table("transactions").
		Select("transactions.id AS id, transactions.description AS description, accounts.currency AS currency").
		Joins("LEFT JOIN accounts ON accounts.id = transactions.account_id").
		Where("transactions.user_id = ? AND transactions.id IN ?", userID, ids).
		Scan(&found).Error
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	for _, r := range found {
		out[r.ID] = txnInfo{Description: r.Description, Currency: r.Currency}
	}
	return out, nil
}

// summaries builds the summary of each row, in order.
func (s *appActivityService) summaries(userID string, rows []models.AuditLog) ([]string, error) {
	txns, err := s.loadTransactions(userID, rows)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i := range rows {
		var txn *txnInfo
		if rows[i].ResourceID != nil {
			if t, ok := txns[*rows[i].ResourceID]; ok {
				txn = &t
			}
		}
		out[i] = buildActivitySummary(rows[i].Action, parseAuditChanges(rows[i].Changes), txn)
	}
	return out, nil
}

// parseAuditChanges decodes an audit row's changes; ” and invalid JSON give
// an empty map.
func parseAuditChanges(raw string) map[string]any {
	if !strings.HasPrefix(raw, "{") {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// cleanSummaryText makes user-written text safe to embed in a one-line plain
// text summary: control characters and bidirectional overrides are dropped,
// whitespace runs collapse to one space, and the result is cut to
// maxSummaryText characters (with an ellipsis). Summaries are plain text; the
// UI must not render them as HTML.
func cleanSummaryText(s string) string {
	var sb strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r), r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F:
			continue
		}
		if space && sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		space = false
		sb.WriteRune(r)
	}
	out := []rune(sb.String())
	if len(out) > maxSummaryText {
		return string(out[:maxSummaryText]) + "…"
	}
	return string(out)
}

// summaryMoney renders "MYR 18.71" (the ISO code, never a symbol, like the MCP
// tools), or "" without a currency or amount.
func summaryMoney(cents int64, currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return ""
	}
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%s %d.%02d", sign, cleanSummaryText(currency), cents/100, cents%100)
}

func changeInt(changes map[string]any, key string) (int64, bool) {
	v, ok := changes[key].(float64)
	if !ok {
		return 0, false
	}
	return int64(v), true
}

func changeText(changes map[string]any, key string) (string, bool) {
	v, ok := changes[key].(string)
	return v, ok
}

// changedText reads a name from an audit row's changes: stored as-is (a create)
// or as a {from, to} pair (an update), in which case the new value is returned.
func changedText(changes map[string]any, key string) (string, bool) {
	if v, ok := changeText(changes, key); ok {
		return v, true
	}
	pair, _ := changes[key].(map[string]any)
	to, ok := pair["to"].(string)
	return to, ok
}

// joinSummary joins the non-empty parts with " · ".
func joinSummary(parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// buildActivitySummary renders the summary for an audit action from a fixed
// template. Details come from the row's changes (written by the MCP write
// wrapper) and, for older rows that lack them, from the transaction itself; a
// row with neither falls back to the action alone ("Added a transaction").
func buildActivitySummary(action string, changes map[string]any, txn *txnInfo) string {
	desc := func() string {
		if d, ok := changeText(changes, "description"); ok {
			return cleanSummaryText(d)
		}
		if txn != nil {
			return cleanSummaryText(txn.Description)
		}
		return ""
	}
	money := func() string {
		cents, ok := changeInt(changes, "amount")
		if !ok {
			return ""
		}
		currency, _ := changeText(changes, "currency")
		if currency == "" && txn != nil {
			currency = txn.Currency
		}
		return summaryMoney(cents, currency)
	}
	named := func(verb, fallback string) string {
		name, _ := changedText(changes, "name")
		return withDetail(verb, fallback, cleanSummaryText(name))
	}

	switch action {
	case "MCP_CREATE_TRANSACTION":
		return withDetail("Added transaction", "Added a transaction", desc(), money())
	case "MCP_CREATE_TRANSFER":
		return withDetail("Added transfer", "Added a transfer", desc(), money())
	case "MCP_UPDATE_TRANSACTION":
		return withDetail("Edited transaction", "Edited a transaction", desc())
	case "MCP_CREATE_BUDGET":
		return named("Created budget", "Created a budget")
	case "MCP_UPDATE_BUDGET":
		return named("Edited budget", "Edited a budget")
	case "MCP_CREATE_CATEGORY":
		return named("Created category", "Created a category")
	case "MCP_UPDATE_CATEGORY":
		return named("Edited category", "Edited a category")
	case "MCP_RECORD_REPAYMENT":
		return withDetail("Recorded repayment", "Recorded a repayment", money())
	case "MCP_WRITE_OFF_RECEIVABLE":
		return withDetail("Wrote off receivable", "Wrote off a receivable", money())
	}
	return "Made a change"
}

// withDetail is "verb · detail · detail", or fallback when no detail is known.
func withDetail(verb, fallback string, details ...string) string {
	if d := joinSummary(details...); d != "" {
		return verb + " · " + d
	}
	return fallback
}

// attachSourceClientNames fills the response-only SourceClientName of every
// transaction written through an OAuth client, from the user's grant. A
// transaction whose grant is gone gets "an app". One query per call.
func attachSourceClientNames(db *gorm.DB, userID string, txns []models.Transaction) error {
	var ids []string
	seen := map[string]bool{}
	for i := range txns {
		if id := txns[i].SourceClientID; id != nil && *id != "" && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var grants []models.OAuthClientGrant
	if err := db.Select("client_id", "client_name").
		Where("user_id = ? AND client_id IN ?", userID, ids).Find(&grants).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	names := make(map[string]string, len(grants))
	for i := range grants {
		names[grants[i].ClientID] = grants[i].ClientName
	}
	for i := range txns {
		id := txns[i].SourceClientID
		if id == nil || *id == "" {
			continue
		}
		txns[i].SourceClientName = names[*id]
		if txns[i].SourceClientName == "" {
			txns[i].SourceClientName = unnamedApp
		}
	}
	return nil
}
