package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"kuberan/internal/handlers"
	"kuberan/internal/hydra"
	"kuberan/internal/logger"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/storage"
	"kuberan/internal/validator"
)

// testMaxUploadBytes mirrors the production upload cap used by the attachment
// path so the integration harness exercises the same MaxBytesReader behavior.
const testMaxUploadBytes int64 = 10 << 20 // 10 MiB

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
		&models.TransactionAttachment{},
		&models.Budget{},
		&models.Security{},
		&models.SecurityPrice{},
		&models.PortfolioSnapshot{},
		&models.Investment{},
		&models.InvestmentTransaction{},
		&models.AuditLog{},
		&models.TrustedOAuthClient{},
		&models.TransactionRule{},
		&models.TransactionRuleCondition{},
		&models.TransactionRuleAction{},
		&models.FixedDepositDetails{},
		&models.Receivable{},
		&models.ReceivableSettlement{},
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
	categoryService := services.NewCategoryService(db)
	accountService := services.NewAccountService(db, categoryService)
	ruleService := services.NewRuleService(db, categoryService)
	transactionService := services.NewTransactionService(db, accountService, ruleService)
	receivableService := services.NewReceivableService(db, accountService, ruleService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	securityService := services.NewSecurityService(db)
	snapshotService := services.NewPortfolioSnapshotService(db)
	auditService := services.NewAuditService(db)
	attachmentService := services.NewAttachmentService(db, storage.NewMemBlobStore(), services.AttachmentLimits{
		MaxUploadBytes:      testMaxUploadBytes,
		MaxAttachmentsPerTx: 10,
	})

	fdService := services.NewFixedDepositService(db, categoryService)
	telegramService := services.NewTelegramService(db)
	trustedClientService := services.NewTrustedClientService(db)

	// Hydra is never contacted by these tests; the OAuth routes are registered
	// (as in production) but backed by an unreachable admin URL.
	hydraAdmin := hydra.NewAdminClient("http://127.0.0.1:1")
	oauthScopes := "read:accounts"

	// Router: same global middleware ordering as cmd/api/main.go minus logging/CORS,
	// then the production route table.
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.ErrorHandler())

	handlers.RegisterRoutes(router, handlers.Routes{
		Auth:                 handlers.NewAuthHandler(userService, auditService),
		OAuth:                handlers.NewOAuthHandler(hydraAdmin, userService, trustedClientService, auditService, oauthScopes, "http://localhost:8081"),
		Registration:         handlers.NewRegistrationHandler(hydraAdmin, auditService, oauthScopes),
		Account:              handlers.NewAccountHandler(accountService, auditService),
		Category:             handlers.NewCategoryHandler(categoryService, auditService),
		Transaction:          handlers.NewTransactionHandler(transactionService, auditService),
		Receivable:           handlers.NewReceivableHandler(receivableService, auditService),
		Attachment:           handlers.NewAttachmentHandler(attachmentService, auditService, testMaxUploadBytes),
		Budget:               handlers.NewBudgetHandler(budgetService, auditService),
		Rule:                 handlers.NewRuleHandler(ruleService, transactionService, auditService),
		Investment:           handlers.NewInvestmentHandler(investmentService, auditService),
		Security:             handlers.NewSecurityHandler(securityService, auditService),
		Snapshot:             handlers.NewPortfolioSnapshotHandler(snapshotService, auditService),
		FixedDepositPipeline: handlers.NewFixedDepositPipelineHandler(fdService),
		Telegram:             handlers.NewTelegramHandler(telegramService, auditService),
	}, handlers.RouteSecrets{
		BotInternalSecret: "test-bot-secret",
		PipelineAPIKey:    "test-pipeline-key",
	})

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

// uploadFile makes a multipart/form-data POST with a single "file" part to the
// test router, mirroring how a browser uploads a receipt attachment.
func (app *testApp) uploadFile(path, fileName, contentType string, content []byte, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, fileName))
	hdr.Set("Content-Type", contentType)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		panic(err)
	}
	if _, err := part.Write(content); err != nil {
		panic(err)
	}
	if err := mw.Close(); err != nil {
		panic(err)
	}

	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
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
