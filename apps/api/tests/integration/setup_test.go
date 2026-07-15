package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"kuberan/internal/handlers"
	"kuberan/internal/logger"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/validator"
)

// testApp holds the full application stack for integration tests.
type testApp struct {
	DB     *gorm.DB
	Router *gin.Engine
}

// dbCounter ensures each test gets a unique in-memory database.
var dbCounter atomic.Int64

func init() {
	gin.SetMode(gin.TestMode)
	logger.Init("test")
	validator.Register()
}

// setupIsolatedDB creates an isolated in-memory SQLite database for a single test.
func setupIsolatedDB(t *testing.T) *gorm.DB {
	t.Helper()

	n := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	allModels := []interface{}{
		&models.User{},
		&models.Account{},
		&models.Category{},
		&models.Transaction{},
		&models.Budget{},
		&models.Security{},
		&models.SecurityPrice{},
		&models.PortfolioSnapshot{},
		&models.Investment{},
		&models.InvestmentTransaction{},
		&models.AuditLog{},
		&models.TrustedOAuthClient{},
	}
	if err := db.AutoMigrate(allModels...); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

// setupApp creates a full application stack backed by an isolated in-memory SQLite.
func setupApp(t *testing.T) *testApp {
	t.Helper()

	db := setupIsolatedDB(t)

	// Services
	userService := services.NewUserService(db)
	accountService := services.NewAccountService(db)
	categoryService := services.NewCategoryService(db)
	transactionService := services.NewTransactionService(db, accountService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	securityService := services.NewSecurityService(db)
	snapshotService := services.NewPortfolioSnapshotService(db)
	auditService := services.NewAuditService(db)

	// Handlers
	authHandler := handlers.NewAuthHandler(userService, auditService)
	accountHandler := handlers.NewAccountHandler(accountService, auditService)
	categoryHandler := handlers.NewCategoryHandler(categoryService, auditService)
	transactionHandler := handlers.NewTransactionHandler(transactionService, auditService)
	budgetHandler := handlers.NewBudgetHandler(budgetService, auditService)
	investmentHandler := handlers.NewInvestmentHandler(investmentService, auditService)
	securityHandler := handlers.NewSecurityHandler(securityService, auditService)
	snapshotHandler := handlers.NewPortfolioSnapshotHandler(snapshotService, auditService)

	// Router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.ErrorHandler())

	v1 := router.Group("/api/v1")

	// Public auth routes
	auth := v1.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware())

	protected.GET("/profile", authHandler.GetProfile)

	accounts := protected.Group("/accounts")
	accounts.POST("/cash", accountHandler.CreateCashAccount)
	accounts.POST("/investment", accountHandler.CreateInvestmentAccount)
	accounts.GET("", accountHandler.GetUserAccounts)
	accounts.GET("/:id", accountHandler.GetAccountByID)
	accounts.PUT("/:id", accountHandler.UpdateAccount)
	accounts.GET("/:id/transactions", transactionHandler.GetAccountTransactions)
	accounts.GET("/:id/investments", investmentHandler.GetAccountInvestments)

	transactions := protected.Group("/transactions")
	transactions.POST("", transactionHandler.CreateTransaction)
	transactions.POST("/transfer", transactionHandler.CreateTransfer)
	transactions.GET("/:id", transactionHandler.GetTransactionByID)
	transactions.DELETE("/:id", transactionHandler.DeleteTransaction)

	categories := protected.Group("/categories")
	categories.POST("", categoryHandler.CreateCategory)
	categories.GET("", categoryHandler.GetUserCategories)

	budgets := protected.Group("/budgets")
	budgets.POST("", budgetHandler.CreateBudget)
	budgets.GET("", budgetHandler.GetBudgets)
	budgets.GET("/progress", budgetHandler.GetBudgetsProgress)
	budgets.GET("/:id", budgetHandler.GetBudget)
	budgets.PUT("/:id", budgetHandler.UpdateBudget)
	budgets.DELETE("/:id", budgetHandler.DeleteBudget)
	budgets.GET("/:id/progress", budgetHandler.GetBudgetProgress)

	investments := protected.Group("/investments")
	investments.POST("", investmentHandler.AddInvestment)
	investments.GET("", investmentHandler.GetAllInvestments)
	investments.GET("/portfolio", investmentHandler.GetPortfolio)
	investments.GET("/snapshots", snapshotHandler.GetSnapshots)
	investments.GET("/:id", investmentHandler.GetInvestment)
	investments.POST("/:id/buy", investmentHandler.RecordBuy)
	investments.POST("/:id/sell", investmentHandler.RecordSell)
	investments.POST("/:id/dividend", investmentHandler.RecordDividend)
	investments.POST("/:id/split", investmentHandler.RecordSplit)
	investments.GET("/:id/transactions", investmentHandler.GetInvestmentTransactions)

	securities := protected.Group("/securities")
	securities.GET("", securityHandler.ListSecurities)
	securities.GET("/:id", securityHandler.GetSecurity)
	securities.GET("/:id/prices", securityHandler.GetPriceHistory)

	// Pipeline routes (use test API key for integration tests)
	pipeline := v1.Group("/pipeline")
	pipeline.Use(middleware.PipelineAuthMiddleware("test-pipeline-key"))
	pipeline.GET("/securities", securityHandler.ListAllSecurities)
	pipeline.POST("/securities", securityHandler.CreateSecurity)
	pipeline.POST("/securities/prices", securityHandler.RecordPrices)
	pipeline.POST("/snapshots", snapshotHandler.ComputeSnapshots)

	return &testApp{DB: db, Router: router}
}

// request makes an HTTP request to the test router and returns the recorder.
func (app *testApp) request(method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

// pipelineRequest makes an HTTP request to a pipeline endpoint with the test API key.
func (app *testApp) pipelineRequest(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-pipeline-key")
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

// createSecurity creates a security via the pipeline API and returns the security ID.
func (app *testApp) createSecurity(t *testing.T, symbol, name, assetType string) string {
	t.Helper()
	body := fmt.Sprintf(`{"symbol":%q,"name":%q,"asset_type":%q}`, symbol, name, assetType)
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create security failed: %d %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	sec := result["security"].(map[string]interface{})
	return sec["id"].(string)
}

// parseJSON parses the response body into a map.
func parseJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return result
}

// registerUser registers a new user and returns the access token, refresh token, and user ID.
func (app *testApp) registerUser(t *testing.T, email, password string) (accessToken, refreshToken, userID string) {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q,"first_name":"Test","last_name":"User"}`, email, password)
	rec := app.request("POST", "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	user := result["user"].(map[string]interface{})
	return result["access_token"].(string), result["refresh_token"].(string), user["id"].(string)
}

// loginUser logs in and returns the access and refresh tokens.
func (app *testApp) loginUser(t *testing.T, email, password string) (accessToken, refreshToken string) {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	rec := app.request("POST", "/api/v1/auth/login", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	return result["access_token"].(string), result["refresh_token"].(string)
}
