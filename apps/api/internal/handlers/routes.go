package handlers

import (
	"time"

	"github.com/gin-gonic/gin"

	"kuberan/internal/config"
	"kuberan/internal/middleware"
)

// Routes bundles every HTTP handler the API serves. cmd/api/main.go and the
// integration tests both build one and pass it to RegisterRoutes, so the route
// table exists in exactly one place and a deleted route fails the tests.
type Routes struct {
	Auth                 *AuthHandler
	OAuth                *OAuthHandler
	Registration         *RegistrationHandler
	Account              *AccountHandler
	Category             *CategoryHandler
	Transaction          *TransactionHandler
	Receivable           *ReceivableHandler
	Attachment           *AttachmentHandler
	Budget               *BudgetHandler
	Rule                 *RuleHandler
	Investment           *InvestmentHandler
	Security             *SecurityHandler
	Snapshot             *PortfolioSnapshotHandler
	FixedDepositPipeline *FixedDepositPipelineHandler
	Telegram             *TelegramHandler
	FX                   *FXHandler
	Import               *ImportHandler
	Export               *ExportHandler
	AccountHistory       *AccountHistoryHandler
	Analytics            *AnalyticsHandler
	Reminder             *ReminderHandler
	CSVImport            *CSVImportHandler
	Demo                 *DemoHandler

	// Limits rate-limits the public auth endpoints. The zero value applies no
	// limits (used by tests that don't exercise them).
	Limits RouteLimits
}

// AttachmentUploadRoute is the Gin route pattern of the receipt upload, which
// enforces its own upload-size cap instead of the global body limit.
const AttachmentUploadRoute = "/api/v1/transactions/:id/attachments"

// exportPerMinute is the fixed budget of GET /export per user (plan 023 1.7).
const exportPerMinute = 1

// RouteLimits holds the rate-limit middleware chains for public endpoints.
// Nil entries are skipped.
type RouteLimits struct {
	Login        []gin.HandlerFunc // /auth/login and /oauth/login
	Register     []gin.HandlerFunc // /auth/register
	Refresh      []gin.HandlerFunc // /auth/refresh
	Registration []gin.HandlerFunc // OAuth DCR (/oauth2/register and its alias)
	Export       []gin.HandlerFunc // GET /export (runs after AuthMiddleware)
	Password     []gin.HandlerFunc // POST /auth/password (runs after AuthMiddleware)
}

// NewRouteLimits builds the rate limiters from configuration. Logins are
// limited per client IP and, across both login endpoints, per submitted email
// (so one account cannot be hammered from many IPs beyond the lockout).
func NewRouteLimits(cfg config.RateLimits) RouteLimits {
	return RouteLimits{
		Login: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(cfg.LoginPerMinute, time.Minute), middleware.ClientIPKey),
			middleware.RateLimit(middleware.NewRateLimiter(cfg.LoginEmailPerHour, time.Hour), middleware.EmailBodyKey),
		},
		Register: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(cfg.RegisterPerHour, time.Hour), middleware.ClientIPKey),
		},
		Refresh: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(cfg.RefreshPerMinute, time.Minute), middleware.ClientIPKey),
		},
		Registration: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(cfg.RegistrationPerHour, time.Hour), middleware.ClientIPKey),
		},
		Export: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(exportPerMinute, time.Minute), middleware.UserIDKey),
		},
		Password: []gin.HandlerFunc{
			middleware.RateLimit(middleware.NewRateLimiter(cfg.PasswordPerHour, time.Hour), middleware.UserIDKey),
		},
	}
}

// with prepends the rate-limit chain to a handler.
func with(limits []gin.HandlerFunc, h gin.HandlerFunc) []gin.HandlerFunc {
	out := make([]gin.HandlerFunc, 0, len(limits)+1)
	out = append(out, limits...)
	return append(out, h)
}

// RouteSecrets holds the shared secrets guarding the service-to-service route
// groups.
type RouteSecrets struct {
	// BotInternalSecret guards /api/v1/internal (Telegram bot).
	BotInternalSecret string
	// PipelineAPIKey guards /api/v1/pipeline (oracle / cron callers).
	PipelineAPIKey string
}

// RegisterRoutes registers the OAuth DCR alias and the whole /api/v1 tree on
// router. Global middleware (recovery, logging, CORS, ...) is the caller's job
// and must be installed before calling this.
func RegisterRoutes(router *gin.Engine, h Routes, secrets RouteSecrets) {
	// API v1 group
	v1 := router.Group("/api/v1")

	// Public routes
	v1.GET("/appearance", h.Auth.GetAppearance)
	auth := v1.Group("/auth")
	auth.POST("/register", with(h.Limits.Register, h.Auth.Register)...)
	auth.POST("/login", with(h.Limits.Login, h.Auth.Login)...)
	auth.POST("/refresh", with(h.Limits.Refresh, h.Auth.RefreshToken)...)
	auth.POST("/logout", h.Auth.Logout)

	// Authenticated account-security routes. They live under /auth so the
	// browser sends the refresh cookie, but need a Bearer access token.
	auth.POST("/password", with(append([]gin.HandlerFunc{middleware.AuthMiddleware()}, h.Limits.Password...), h.Auth.ChangePassword)...)
	auth.GET("/sessions", middleware.AuthMiddleware(), h.Auth.ListSessions)
	auth.POST("/sessions/revoke-others", middleware.AuthMiddleware(), h.Auth.RevokeOtherSessions)
	auth.DELETE("/sessions/:id", middleware.AuthMiddleware(), h.Auth.RevokeSession)

	// OAuth login/consent bridge (public: driven by the apps/web pages during the
	// Hydra authorization flow, before a Kuberan session exists). See plan 15.
	oauth := v1.Group("/oauth")
	oauth.POST("/login", with(h.Limits.Login, h.OAuth.Login)...)
	oauth.POST("/login/reject", h.OAuth.RejectLogin)
	oauth.GET("/consent", h.OAuth.GetConsent)
	oauth.POST("/consent/accept", h.OAuth.AcceptConsent)
	oauth.POST("/consent/reject", h.OAuth.RejectConsent)
	// Hardened DCR proxy: public clients only, restricted grants, capped scopes,
	// audited + alerted (Phase 5). Served at the RFC-standard /oauth2/register path
	// that Hydra advertises as its registration_endpoint ({issuer}/oauth2/register),
	// so cloudflared can route DCR through this proxy via simple path matching
	// (no path rewriting) instead of hitting Hydra's public endpoint directly.
	// The /api/v1/oauth/register alias is retained for internal/test callers.
	// One DCR handler chain serves both paths, so they share a single budget.
	dcr := with(h.Limits.Registration, h.Registration.Register)
	router.POST("/oauth2/register", dcr...)
	oauth.POST("/register", dcr...)

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware())

	// User profile
	protected.GET("/profile", h.Auth.GetProfile)
	protected.PATCH("/profile", h.Auth.UpdateProfileSettings)

	registerAccountRoutes(protected, h)
	registerTransactionRoutes(protected, h)
	registerReceivableRoutes(protected, h)
	registerBudgetRoutes(protected, h)
	registerRuleRoutes(protected, h)
	registerInvestmentRoutes(protected, h)
	registerSecurityRoutes(protected, h)
	registerCategoryRoutes(protected, h)

	// Insights summary (plan 023, 5.4).
	protected.GET("/analytics/typical", h.Analytics.GetTypical)

	// Exchange rates (plan 024). Static /latest is registered beside the list route.
	protected.GET("/fx-rates", h.FX.ListRates)
	protected.GET("/fx-rates/latest", h.FX.LatestRate)

	// Statement imports (plan 024, phase 3b). The upload enforces its own size cap
	// (see ImportIBKRRoute).
	protected.POST("/imports/ibkr", h.Import.ImportIBKR)
	// Bank-statement CSV import (plan 023 5.8); also enforces its own size cap
	// (see ImportCSVRoute).
	protected.POST("/imports/csv", h.CSVImport.ImportCSV)

	// Sample household (plan 023 5.7): 404 unless DEMO_ENABLED.
	protected.POST("/demo/load", h.Demo.LoadDemo)
	protected.DELETE("/demo", h.Demo.RemoveDemo)

	// Data export (plan 023 1.7): streamed zip of CSVs, 1 per minute per user.
	protected.GET("/export", with(h.Limits.Export, h.Export.Export)...)
	protected.GET("/system/status", h.Export.GetSystemStatus)

	// Telegram routes
	// Connected apps: the user's OAuth client grants (plan 022).
	protected.GET("/oauth/clients", h.OAuth.ListConnectedApps)
	protected.GET("/oauth/clients/activity", h.OAuth.GetAllAppActivity)
	protected.GET("/oauth/clients/:client_id/activity", h.OAuth.GetAppActivity)
	protected.DELETE("/oauth/clients/:client_id", h.OAuth.DisconnectApp)

	telegram := protected.Group("/telegram")
	telegram.GET("/link", h.Telegram.GetLink)
	telegram.POST("/generate-code", h.Telegram.GenerateCode)
	telegram.DELETE("/unlink", h.Telegram.Unlink)
	telegram.PUT("/reminder-settings", h.Telegram.UpdateReminderSettings)

	// Reminders (plan 023, 5.6): card due, loan due and deposit maturity.
	protected.GET("/reminders", h.Reminder.List)
	protected.PATCH("/reminders/:id", h.Reminder.SetEnabled)

	// Internal routes (for bot service communication)
	internal := v1.Group("/internal")
	internal.Use(middleware.InternalAuthMiddleware(secrets.BotInternalSecret))
	internal.POST("/telegram/complete-link", h.Telegram.CompleteLink)
	internal.GET("/telegram/resolve/:telegram_user_id", h.Telegram.ResolveUser)
	internal.POST("/telegram/activity/:telegram_user_id", h.Telegram.RecordActivity)
	internal.GET("/telegram/reminders/due", h.Reminder.Due)
	internal.POST("/telegram/reminders/:id/sent", h.Reminder.MarkSent)

	// Pipeline routes (API key auth, no JWT)
	pipeline := v1.Group("/pipeline")
	pipeline.Use(middleware.PipelineAuthMiddleware(secrets.PipelineAPIKey))
	pipeline.GET("/securities", h.Security.ListAllSecurities)
	pipeline.POST("/securities", h.Security.CreateSecurity)
	pipeline.POST("/securities/prices", h.Security.RecordPrices)
	pipeline.PATCH("/securities/:id", h.Security.UpdateSecurity)
	pipeline.POST("/snapshots", h.Snapshot.ComputeSnapshots)
	pipeline.POST("/fixed-deposits/process", h.FixedDepositPipeline.ProcessFixedDeposits)
	pipeline.POST("/fx-rates", h.FX.RecordRates)
	pipeline.GET("/fx-pairs", h.FX.GetPairs)
}

// registerAccountRoutes registers the /accounts tree.
// Static creation routes (/cash, /investment, /credit-card, /fixed-deposit) are
// registered before the param routes (/:id) so Gin resolves them as static siblings
// — the same pattern already in use for cash/investment/credit-card.
func registerAccountRoutes(protected *gin.RouterGroup, h Routes) {
	accounts := protected.Group("/accounts")
	accounts.POST("/cash", h.Account.CreateCashAccount)
	accounts.POST("/investment", h.Account.CreateInvestmentAccount)
	accounts.POST("/credit-card", h.Account.CreateCreditCardAccount)
	accounts.POST("/debt", h.Account.CreateDebtAccount)
	accounts.POST("/fixed-deposit", h.Account.CreateFixedDepositAccount)
	accounts.GET("", h.Account.GetUserAccounts)
	accounts.GET("/sparklines", h.AccountHistory.GetSparklines)
	accounts.GET("/:id", h.Account.GetAccountByID)
	accounts.PUT("/:id", h.Account.UpdateAccount)
	accounts.GET("/:id/balance-history", h.AccountHistory.GetBalanceHistory)
	accounts.GET("/:id/transactions", h.Transaction.GetAccountTransactions)
	accounts.GET("/:id/investments", h.Investment.GetAccountInvestments)
	accounts.POST("/:id/fixed-deposit/close", h.Account.CloseFixedDeposit)
}

// registerTransactionRoutes registers the /transactions tree, including the
// shared-expense, receivables and attachment sub-routes.
func registerTransactionRoutes(protected *gin.RouterGroup, h Routes) {
	transactions := protected.Group("/transactions")
	transactions.GET("", h.Transaction.GetUserTransactions)
	transactions.POST("", h.Transaction.CreateTransaction)
	transactions.POST("/transfer", h.Transaction.CreateTransfer)
	transactions.POST("/shared", h.Receivable.CreateSharedExpense)
	transactions.POST("/bulk", h.Transaction.BulkTransactions)
	transactions.POST("/suggest-category", h.Transaction.SuggestCategory)
	transactions.GET("/spending-by-category", h.Transaction.GetSpendingByCategory)
	transactions.GET("/cashflow", h.Transaction.GetCashflow)
	transactions.GET("/monthly-summary", h.Transaction.GetMonthlySummary)
	transactions.GET("/daily-spending", h.Transaction.GetDailySpending)
	transactions.GET("/daily-summary", h.Transaction.GetDailySummary)
	transactions.GET("/top-expenses", h.Transaction.GetTopExpenses)
	transactions.GET("/:id", h.Transaction.GetTransactionByID)
	transactions.PUT("/:id", h.Transaction.UpdateTransaction)
	transactions.PUT("/:id/receivables", h.Receivable.UpdateReceivables)
	transactions.DELETE("/:id", h.Transaction.DeleteTransaction)
	transactions.POST("/:id/attachments", h.Attachment.Upload)
	transactions.GET("/:id/attachments", h.Attachment.List)
	transactions.GET("/:id/attachments/:aid", h.Attachment.Download)
	transactions.DELETE("/:id/attachments/:aid", h.Attachment.Delete)
}

// registerReceivableRoutes registers the /receivables tree (plan 021). Static
// segments (/summary, /counterparties, /settlements/:id) are registered
// alongside the param routes; Gin resolves static siblings before /:id.
func registerReceivableRoutes(protected *gin.RouterGroup, h Routes) {
	receivables := protected.Group("/receivables")
	receivables.GET("", h.Receivable.ListReceivables)
	receivables.GET("/summary", h.Receivable.GetReceivablesSummary)
	receivables.GET("/counterparties", h.Receivable.ListCounterparties)
	receivables.POST("/counterparties/rename", h.Receivable.RenameCounterparty)
	receivables.DELETE("/settlements/:id", h.Receivable.DeleteSettlement)
	receivables.GET("/:id", h.Receivable.GetReceivable)
	receivables.POST("/:id/settle", h.Receivable.SettleReceivable)
	receivables.POST("/:id/write-off", h.Receivable.WriteOffReceivable)
	receivables.DELETE("/:id/write-off", h.Receivable.UndoWriteOff)
}

func registerBudgetRoutes(protected *gin.RouterGroup, h Routes) {
	budgets := protected.Group("/budgets")
	budgets.POST("", h.Budget.CreateBudget)
	budgets.GET("", h.Budget.GetBudgets)
	// Static route registered before the param routes so Gin resolves it correctly.
	budgets.GET("/progress", h.Budget.GetBudgetsProgress)
	budgets.GET("/:id", h.Budget.GetBudget)
	budgets.PUT("/:id", h.Budget.UpdateBudget)
	budgets.DELETE("/:id", h.Budget.DeleteBudget)
	budgets.GET("/:id/progress", h.Budget.GetBudgetProgress)
}

// registerRuleRoutes registers transaction rule routes (plan 018). Static routes
// are registered before the param route so Gin resolves /rules/reorder and
// /rules/preview correctly.
func registerRuleRoutes(protected *gin.RouterGroup, h Routes) {
	rules := protected.Group("/rules")
	rules.POST("", h.Rule.CreateRule)
	rules.GET("", h.Rule.GetRules)
	rules.POST("/reorder", h.Rule.ReorderRules)
	rules.POST("/preview", h.Rule.PreviewRule)
	rules.GET("/suggestions", h.Rule.GetSuggestions)
	rules.GET("/:id", h.Rule.GetRule)
	rules.PUT("/:id", h.Rule.UpdateRule)
	rules.DELETE("/:id", h.Rule.DeleteRule)
	rules.POST("/:id/apply", h.Rule.ApplyRule)
}

func registerInvestmentRoutes(protected *gin.RouterGroup, h Routes) {
	investments := protected.Group("/investments")
	investments.POST("", h.Investment.AddInvestment)
	investments.GET("", h.Investment.GetAllInvestments)
	investments.GET("/portfolio", h.Investment.GetPortfolio)
	investments.GET("/snapshots", h.Snapshot.GetSnapshots)
	investments.GET("/:id", h.Investment.GetInvestment)
	investments.POST("/:id/buy", h.Investment.RecordBuy)
	investments.POST("/:id/sell", h.Investment.RecordSell)
	investments.POST("/:id/dividend", h.Investment.RecordDividend)
	investments.POST("/:id/split", h.Investment.RecordSplit)
	investments.GET("/:id/transactions", h.Investment.GetInvestmentTransactions)
}

// registerSecurityRoutes registers the authenticated security routes.
func registerSecurityRoutes(protected *gin.RouterGroup, h Routes) {
	securities := protected.Group("/securities")
	securities.GET("", h.Security.ListSecurities)
	securities.GET("/:id", h.Security.GetSecurity)
	securities.GET("/:id/prices", h.Security.GetPriceHistory)
}

func registerCategoryRoutes(protected *gin.RouterGroup, h Routes) {
	categories := protected.Group("/categories")
	categories.POST("", h.Category.CreateCategory)
	categories.GET("", h.Category.GetUserCategories)
	categories.GET("/:id", h.Category.GetCategoryByID)
	categories.PUT("/:id", h.Category.UpdateCategory)
	categories.DELETE("/:id", h.Category.DeleteCategory)
}
