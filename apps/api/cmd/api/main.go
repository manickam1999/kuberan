package main

import (
	"context"
	"fmt"
	"kuberan/internal/config"
	"kuberan/internal/database"
	"kuberan/internal/handlers"
	"kuberan/internal/hydra"
	"kuberan/internal/logger"
	"kuberan/internal/middleware"
	"kuberan/internal/services"
	"kuberan/internal/storage"
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
	categoryService := services.NewCategoryService(db)
	accountService := services.NewAccountService(db, categoryService)
	ruleService := services.NewRuleService(db, categoryService)
	transactionService := services.NewTransactionService(db, accountService, ruleService)
	receivableService := services.NewReceivableService(db, accountService, ruleService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	securityService := services.NewSecurityService(db)
	snapshotService := services.NewPortfolioSnapshotService(db)
	fdService := services.NewFixedDepositService(db, categoryService)
	auditService := services.NewAuditService(db)
	telegramService := services.NewTelegramService(db)
	trustedClientService := services.NewTrustedClientService(db)

	// Receipt attachments (plan 017): blob store + service. The store is only
	// reachable through the ownership-checked API; MinIO stays private. When no
	// bucket is configured (e.g. a dev instance without MinIO) the API still
	// boots with a disabled store: attachment requests fail cleanly rather than
	// taking the whole server down.
	var blobStore storage.BlobStore
	if appConfig.StorageBucket == "" {
		log.Warn("STORAGE_BUCKET not set; receipt attachments are disabled")
		blobStore = storage.NewDisabledBlobStore()
	} else {
		s3Store, err := storage.NewS3BlobStore(appConfig.StorageConfig())
		if err != nil {
			return fmt.Errorf("failed to initialize blob store: %w", err)
		}
		blobStore = s3Store
	}
	attachmentService := services.NewAttachmentService(db, blobStore, services.AttachmentLimits{
		MaxUploadBytes:      appConfig.MaxUploadBytes,
		MaxAttachmentsPerTx: appConfig.MaxAttachmentsPerTx,
	})

	// Hydra admin client for the OAuth login/consent bridge (private network only).
	hydraAdmin := hydra.NewAdminClient(appConfig.HydraAdminURL)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(userService, auditService)
	oauthHandler := handlers.NewOAuthHandler(hydraAdmin, userService, trustedClientService, auditService, appConfig.OAuthScopes, appConfig.MCPResourceURL)
	registrationHandler := handlers.NewRegistrationHandler(hydraAdmin, auditService, appConfig.OAuthScopes)
	accountHandler := handlers.NewAccountHandler(accountService, auditService)
	categoryHandler := handlers.NewCategoryHandler(categoryService, auditService)
	transactionHandler := handlers.NewTransactionHandler(transactionService, auditService)
	receivableHandler := handlers.NewReceivableHandler(receivableService, auditService)
	attachmentHandler := handlers.NewAttachmentHandler(attachmentService, auditService, appConfig.MaxUploadBytes)
	budgetHandler := handlers.NewBudgetHandler(budgetService, auditService)
	ruleHandler := handlers.NewRuleHandler(ruleService, transactionService, auditService)
	investmentHandler := handlers.NewInvestmentHandler(investmentService, auditService)
	securityHandler := handlers.NewSecurityHandler(securityService, auditService)
	snapshotHandler := handlers.NewPortfolioSnapshotHandler(snapshotService, auditService)
	fdPipelineHandler := handlers.NewFixedDepositPipelineHandler(fdService)
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
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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

	// Register every route (single source of truth shared with the tests).
	handlers.RegisterRoutes(router, handlers.Routes{
		Auth:                 authHandler,
		OAuth:                oauthHandler,
		Registration:         registrationHandler,
		Account:              accountHandler,
		Category:             categoryHandler,
		Transaction:          transactionHandler,
		Receivable:           receivableHandler,
		Attachment:           attachmentHandler,
		Budget:               budgetHandler,
		Rule:                 ruleHandler,
		Investment:           investmentHandler,
		Security:             securityHandler,
		Snapshot:             snapshotHandler,
		FixedDepositPipeline: fdPipelineHandler,
		Telegram:             telegramHandler,
	}, handlers.RouteSecrets{
		BotInternalSecret: appConfig.BotInternalSecret,
		PipelineAPIKey:    appConfig.PipelineAPIKey,
	})

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
