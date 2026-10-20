package handlers

import (
	"github.com/gin-gonic/gin"

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
	auth := v1.Group("/auth")
	auth.POST("/register", h.Auth.Register)
	auth.POST("/login", h.Auth.Login)
	auth.POST("/refresh", h.Auth.RefreshToken)

	// OAuth login/consent bridge (public: driven by the apps/web pages during the
	// Hydra authorization flow, before a Kuberan session exists). See plan 15.
	oauth := v1.Group("/oauth")
	oauth.POST("/login", h.OAuth.Login)
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
	router.POST("/oauth2/register", h.Registration.Register)
	oauth.POST("/register", h.Registration.Register)

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

	// Telegram routes
	telegram := protected.Group("/telegram")
	telegram.GET("/link", h.Telegram.GetLink)
	telegram.POST("/generate-code", h.Telegram.GenerateCode)
	telegram.DELETE("/unlink", h.Telegram.Unlink)

	// Internal routes (for bot service communication)
	internal := v1.Group("/internal")
	internal.Use(middleware.InternalAuthMiddleware(secrets.BotInternalSecret))
	internal.POST("/telegram/complete-link", h.Telegram.CompleteLink)
	internal.GET("/telegram/resolve/:telegram_user_id", h.Telegram.ResolveUser)
	internal.POST("/telegram/activity/:telegram_user_id", h.Telegram.RecordActivity)

	// Pipeline routes (API key auth, no JWT)
	pipeline := v1.Group("/pipeline")
	pipeline.Use(middleware.PipelineAuthMiddleware(secrets.PipelineAPIKey))
	pipeline.GET("/securities", h.Security.ListAllSecurities)
	pipeline.POST("/securities", h.Security.CreateSecurity)
	pipeline.POST("/securities/prices", h.Security.RecordPrices)
	pipeline.POST("/snapshots", h.Snapshot.ComputeSnapshots)
	pipeline.POST("/fixed-deposits/process", h.FixedDepositPipeline.ProcessFixedDeposits)
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
	accounts.POST("/fixed-deposit", h.Account.CreateFixedDepositAccount)
	accounts.GET("", h.Account.GetUserAccounts)
	accounts.GET("/:id", h.Account.GetAccountByID)
	accounts.PUT("/:id", h.Account.UpdateAccount)
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
