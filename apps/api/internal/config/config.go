package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"kuberan/internal/logger"
	"kuberan/internal/storage"

	"github.com/joho/godotenv"
)

// Environment represents the application environment.
type Environment string

// Environment constants.
const (
	Development Environment = "development"
	Staging     Environment = "staging"
	Production  Environment = "production"
)

// Config holds application configuration.
type Config struct {
	// Environment
	Env Environment

	// Server
	Port string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// JWT
	JWTSecret        string
	JWTExpirationDur time.Duration

	// Pipeline
	PipelineAPIKey string

	// CORS
	CORSOrigin string

	// Bot
	BotInternalSecret string

	// OAuth / Hydra (MCP authorization). See plans/015-mcp-oauth-hydra.
	HydraIssuerURL      string // public issuer, e.g. https://auth.<domain>
	HydraAdminURL       string // private admin API, e.g. http://hydra:4445
	MCPResourceURL      string // RS resource identifier, e.g. https://mcp.<domain>
	OAuthScopes         string // space-delimited read:* scope set
	HydraPinnedClientID string // optional pinned client (if not using TOFU)

	// Storage (receipt attachments). See plans/017-transaction-receipts.
	StorageEndpoint     string // e.g. http://minio:9000 (internal only)
	StorageBucket       string // e.g. kuberan-receipts
	StorageAccessKey    string
	StorageSecretKey    string
	StorageUsePathStyle bool  // true for MinIO
	MaxUploadBytes      int64 // per-file cap, default 10 MiB
	MaxAttachmentsPerTx int   // per-transaction cap, default 10
}

// DefaultOAuthScopes is the full granular read:* scope set the RS enforces.
const DefaultOAuthScopes = "read:accounts read:transactions read:budgets " +
	"read:categories read:investments read:portfolio read:snapshots"

var appConfig *Config

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if not already loaded
	if err := godotenv.Load(); err != nil {
		logger.Get().Warn(".env file not found")
	}

	// Get values from environment variables with defaults
	config := &Config{
		// Environment
		Env: Environment(getEnv("ENV", string(Development))),

		// Server
		Port: getEnv("PORT", "8080"),

		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "kuberan"),
		DBPassword: getEnv("DB_PASSWORD", "kuberan"),
		DBName:     getEnv("DB_NAME", "kuberan"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		// JWT
		JWTSecret: getEnv("JWT_SECRET", "fallback-secret-key-for-dev-only"),

		// Pipeline
		PipelineAPIKey: os.Getenv("PIPELINE_API_KEY"),

		// CORS
		CORSOrigin: getEnv("CORS_ORIGIN", "*"),

		// Bot
		BotInternalSecret: os.Getenv("BOT_INTERNAL_SECRET"),

		// OAuth / Hydra (MCP authorization)
		HydraIssuerURL:      getEnv("HYDRA_ISSUER_URL", "http://localhost:4444"),
		HydraAdminURL:       getEnv("HYDRA_ADMIN_URL", "http://localhost:4445"),
		MCPResourceURL:      getEnv("MCP_RESOURCE_URL", "http://localhost:8081"),
		OAuthScopes:         getEnv("OAUTH_SCOPES", DefaultOAuthScopes),
		HydraPinnedClientID: os.Getenv("HYDRA_PINNED_CLIENT_ID"),

		// Storage (receipt attachments)
		StorageEndpoint:     os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:       os.Getenv("STORAGE_BUCKET"),
		StorageAccessKey:    os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecretKey:    os.Getenv("STORAGE_SECRET_KEY"),
		StorageUsePathStyle: getEnvBool("STORAGE_USE_PATH_STYLE", true),
		MaxUploadBytes:      getEnvInt64("MAX_UPLOAD_BYTES", 10*1024*1024),
		MaxAttachmentsPerTx: int(getEnvInt64("MAX_ATTACHMENTS_PER_TX", 10)),
	}

	// Parse JWT expiration duration
	expStr := getEnv("JWT_EXPIRES_IN", "24h")
	expDur, err := time.ParseDuration(expStr)
	if err != nil {
		logger.Get().Warnf("Invalid JWT_EXPIRES_IN value '%s', falling back to 24h", expStr)
		expDur = 24 * time.Hour
	}
	config.JWTExpirationDur = expDur

	// Validate production configuration
	if config.Env == Production {
		if err := config.validateProduction(); err != nil {
			return nil, err
		}
	}

	appConfig = config
	return config, nil
}

// Get returns the application configuration
func Get() *Config {
	if appConfig == nil {
		var err error
		appConfig, err = Load()
		if err != nil {
			logger.Get().Fatalf("Failed to load configuration: %v", err)
		}
	}
	return appConfig
}

// validateProduction checks that production-unsafe defaults are not used.
func (c *Config) validateProduction() error {
	unsafeSecrets := []string{"", "fallback-secret-key-for-dev-only", "your-super-secret-key-change-in-production"}
	for _, s := range unsafeSecrets {
		if c.JWTSecret == s {
			return fmt.Errorf("JWT_SECRET must be explicitly set in production")
		}
	}
	if c.DBPassword == "kuberan" {
		return fmt.Errorf("DB_PASSWORD must not be the default in production")
	}
	if c.StorageBucket != "" && c.StorageSecretKey == "" {
		return fmt.Errorf("STORAGE_SECRET_KEY must be set when STORAGE_BUCKET is configured in production")
	}
	return nil
}

// StorageConfig returns the blob-store connection settings for the S3/MinIO
// backend.
func (c *Config) StorageConfig() storage.S3Config {
	return storage.S3Config{
		Endpoint:     c.StorageEndpoint,
		Bucket:       c.StorageBucket,
		AccessKey:    c.StorageAccessKey,
		SecretKey:    c.StorageSecretKey,
		UsePathStyle: c.StorageUsePathStyle,
	}
}

// AttachmentConfig holds the per-request attachment limits.
type AttachmentConfig struct {
	MaxUploadBytes      int64
	MaxAttachmentsPerTx int
}

// AttachmentConfig returns the attachment upload limits.
func (c *Config) AttachmentConfig() AttachmentConfig {
	return AttachmentConfig{
		MaxUploadBytes:      c.MaxUploadBytes,
		MaxAttachmentsPerTx: c.MaxAttachmentsPerTx,
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable or returns a default value.
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		logger.Get().Warnf("Invalid boolean for %s ('%s'), falling back to %t", key, value, defaultValue)
		return defaultValue
	}
	return parsed
}

// getEnvInt64 retrieves an int64 environment variable or returns a default value.
func getEnvInt64(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logger.Get().Warnf("Invalid integer for %s ('%s'), falling back to %d", key, value, defaultValue)
		return defaultValue
	}
	return parsed
}
