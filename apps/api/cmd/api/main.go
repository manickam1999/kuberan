package main

import (
	"context"
	"fmt"
	"kuberan/internal/config"
	"kuberan/internal/database"
	"kuberan/internal/handlers"
	"kuberan/internal/logger"
	"kuberan/internal/middleware"
	"kuberan/internal/services"
	"kuberan/internal/validator"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "kuberan/internal/docs" // Import swagger docs
)

// @title           Kuberan API
// @version         1.0
// @description     Kuberan is a personal finance application that allows users to efficiently manage their finances, make budgets, and track investments.
// @termsOfService  http://swagger.io/terms/

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description Pipeline API key for service-to-service authentication.

func main() {
	// Initialize logger (use ENV var if available, default to development)
	logger.Init(os.Getenv("ENV"))
	defer logger.Sync()

	if err := run(); err != nil {
		logger.Get().Fatalf("Fatal error: %v", err)
	}
}

func run() error {
	log := logger.Get()

	// Load configuration
	appConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize database configuration
	dbConfig, err := database.NewConfig()
	if err != nil {
		return fmt.Errorf("failed to load database configuration: %w", err)
	}

	// Create database manager
	dbManager, err := database.NewManager(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to create database manager: %w", err)
	}

	// Run migrations automatically in development; in production, run manually via cmd/migrate
	if appConfig.Env != config.Production {
		if err := dbManager.RunMigrations(); err != nil {
			return fmt.Errorf("failed to run database migrations: %w", err)
		}
	}

	// Initialize services
	db := dbManager.DB()
	userService := services.NewUserService(db)
	accountService := services.NewAccountService(db)
	categoryService := services.NewCategoryService(db)
	transactionService := services.NewTransactionService(db, accountService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	securityService := services.NewSecurityService(db)
	snapshotService := services.NewPortfolioSnapshotService(db)
	auditService := services.NewAuditService(db)
	telegramService := services.NewTelegramService(db)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(userService, auditService)
	accountHandler := handlers.NewAccountHandler(accountService, auditService)
	categoryHandler := handlers.NewCategoryHandler(categoryService, auditService)
	transactionHandler := handlers.NewTransactionHandler(transactionService, auditService)
	budgetHandler := handlers.NewBudgetHandler(budgetService, auditService)
	investmentHandler := handlers.NewInvestmentHandler(investmentService, auditService)
	securityHandler := handlers.NewSecurityHandler(securityService, auditService)
	snapshotHandler := handlers.NewPortfolioSnapshotHandler(snapshotService, auditService)
	telegramHandler := handlers.NewTelegramHandler(telegramService, auditService)

	// Register custom validators before routes
	validator.Register()

	// Set Gin mode based on environment
	if appConfig.Env == config.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogging())
	router.Use(middleware.ErrorHandler())

	// CORS middleware — CORS_ORIGIN env var controls allowed origins (default: *)
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", appConfig.CORSOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health check endpoint
	router.GET("/api/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "database": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "connected"})
	})

	// API v1 group
	v1 := router.Group("/api/v1")

	// Public routes
	auth := v1.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware())

	// User profile
	protected.GET("/profile", authHandler.GetProfile)

	// Account routes
	accounts := protected.Group("/accounts")
	accounts.POST("/cash", accountHandler.CreateCashAccount)
	accounts.POST("/investment", accountHandler.CreateInvestmentAccount)
	accounts.POST("/credit-card", accountHandler.CreateCreditCardAccount)
	accounts.GET("", accountHandler.GetUserAccounts)
	accounts.GET("/:id", accountHandler.GetAccountByID)
	accounts.PUT("/:id", accountHandler.UpdateAccount)
	accounts.GET("/:id/transactions", transactionHandler.GetAccountTransactions)
	accounts.GET("/:id/investments", investmentHandler.GetAccountInvestments)

	// Transaction routes
	transactions := protected.Group("/transactions")
	transactions.GET("", transactionHandler.GetUserTransactions)
	transactions.POST("", transactionHandler.CreateTransaction)
	transactions.POST("/transfer", transactionHandler.CreateTransfer)
	transactions.GET("/spending-by-category", transactionHandler.GetSpendingByCategory)
	transactions.GET("/monthly-summary", transactionHandler.GetMonthlySummary)
	transactions.GET("/daily-spending", transactionHandler.GetDailySpending)
	transactions.GET("/:id", transactionHandler.GetTransactionByID)
	transactions.PUT("/:id", transactionHandler.UpdateTransaction)
	transactions.DELETE("/:id", transactionHandler.DeleteTransaction)

	// Budget routes
	budgets := protected.Group("/budgets")
	budgets.POST("", budgetHandler.CreateBudget)
	budgets.GET("", budgetHandler.GetBudgets)
	budgets.GET("/:id", budgetHandler.GetBudget)
	budgets.PUT("/:id", budgetHandler.UpdateBudget)
	budgets.DELETE("/:id", budgetHandler.DeleteBudget)
	budgets.GET("/:id/progress", budgetHandler.GetBudgetProgress)

	// Investment routes
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

	// Security routes (authenticated)
	securities := protected.Group("/securities")
	securities.GET("", securityHandler.ListSecurities)
	securities.GET("/:id", securityHandler.GetSecurity)
	securities.GET("/:id/prices", securityHandler.GetPriceHistory)

	// Category routes
	categories := protected.Group("/categories")
	categories.POST("", categoryHandler.CreateCategory)
	categories.GET("", categoryHandler.GetUserCategories)
	categories.GET("/:id", categoryHandler.GetCategoryByID)
	categories.PUT("/:id", categoryHandler.UpdateCategory)
	categories.DELETE("/:id", categoryHandler.DeleteCategory)

	// Telegram routes
	telegram := protected.Group("/telegram")
	telegram.GET("/link", telegramHandler.GetLink)
	telegram.POST("/generate-code", telegramHandler.GenerateCode)
	telegram.DELETE("/unlink", telegramHandler.Unlink)

	// Internal routes (for bot service communication)
	internal := v1.Group("/internal")
	internal.Use(middleware.InternalAuthMiddleware(appConfig.BotInternalSecret))
	internal.POST("/telegram/complete-link", telegramHandler.CompleteLink)
	internal.GET("/telegram/resolve/:telegram_user_id", telegramHandler.ResolveUser)
	internal.POST("/telegram/activity/:telegram_user_id", telegramHandler.RecordActivity)

	// Pipeline routes (API key auth, no JWT)
	pipeline := v1.Group("/pipeline")
	pipeline.Use(middleware.PipelineAuthMiddleware(appConfig.PipelineAPIKey))
	pipeline.GET("/securities", securityHandler.ListAllSecurities)
	pipeline.POST("/securities", securityHandler.CreateSecurity)
	pipeline.POST("/securities/prices", securityHandler.RecordPrices)
	pipeline.POST("/snapshots", snapshotHandler.ComputeSnapshots)

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + appConfig.Port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Infof("Starting Kuberan backend server on port %s", appConfig.Port)
		log.Infof("Swagger documentation available at http://localhost:%s/swagger/index.html", appConfig.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// Give outstanding requests 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("Server forced to shutdown: %v", err)
	}

	// Close database connections
	sqlDB, dbErr := db.DB()
	if dbErr == nil {
		if err := sqlDB.Close(); err != nil {
			log.Errorf("Error closing database: %v", err)
		}
	}

	log.Info("Server exited cleanly")
	return nil
}
