package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// CORSOrigins is the exact-match allowlist of browser origins allowed to
	// make credentialed cross-origin requests (CORS_ORIGIN, comma-separated).
	CORSOrigins []string

	// CookieSecure sets the Secure flag on the refresh-token cookie
	// (COOKIE_SECURE; defaults to true outside development).
	CookieSecure bool

	// TrustedProxyCIDRs lists the networks whose forwarding headers
	// (CF-Connecting-IP, X-Forwarded-For) the API believes for the client IP
	// (API_TRUSTED_PROXY_CIDRS). Empty means the TCP peer address is used.
	TrustedProxyCIDRs []string

	// MaxBodyBytes caps JSON request bodies (MAX_BODY_BYTES, default 1 MiB).
	// The attachment upload route uses MaxUploadBytes instead.
	MaxBodyBytes int64

	// RateLimits holds the per-route abuse limits for public auth endpoints.
	RateLimits RateLimits

	// Bot
	BotInternalSecret string

	// OAuth / Hydra (MCP authorization). See plans/015-mcp-oauth-hydra.
	HydraIssuerURL      string // public issuer, e.g. https://auth.<domain>
	HydraAdminURL       string // private admin API, e.g. http://hydra:4445
	MCPResourceURL      string // RS resource identifier, e.g. https://mcp.<domain>
	OAuthScopes         string // space-delimited API scope set (read:*, plus write:* when MCP writes are enabled)
	HydraPinnedClientID string // optional pinned client (if not using TOFU)

	// MCP write access (plan 022). Off by default; when off, write scopes are
	// neither advertised, registrable nor grantable, and write tools are not
	// registered.
	MCPWriteEnabled       bool
	MCPMaxWriteAmount     int64 // per-call ceiling in cents
	MCPWriteRatePerMinute int   // per (user, client)
	MCPWriteRatePerDay    int   // per (user, client)

	// MCPTrustedProxyCIDRs lists the networks (e.g. the Docker network the
	// Cloudflare tunnel connects from) whose CF-Connecting-IP header the MCP
	// server trusts for audit logging. Empty means the header is ignored.
	MCPTrustedProxyCIDRs []string

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

// DefaultOAuthWriteScopes is the write:* scope set added to OAuthScopes when
// MCP writes are enabled (plan 022).
const DefaultOAuthWriteScopes = "write:transactions write:categories write:budgets"

// RateLimits configures the in-memory rate limits on public auth endpoints.
// Each value is the number of requests allowed per window; 0 disables that
// limit.
type RateLimits struct {
	LoginPerMinute      int // per client IP: /auth/login and /oauth/login (RATE_LIMIT_LOGIN_PER_MINUTE, 10)
	LoginEmailPerHour   int // per submitted email across both logins (RATE_LIMIT_LOGIN_EMAIL_PER_HOUR, 20)
	RegisterPerHour     int // per client IP: /auth/register (RATE_LIMIT_REGISTER_PER_HOUR, 5)
	RefreshPerMinute    int // per client IP: /auth/refresh (RATE_LIMIT_REFRESH_PER_MINUTE, 60)
	RegistrationPerHour int // per client IP: OAuth DCR (RATE_LIMIT_DCR_PER_HOUR, 10)
}

// DefaultCORSOrigin is the development default for CORS_ORIGIN.
const DefaultCORSOrigin = "http://localhost:3000"

// minSecretLength is the minimum length required (JWT_SECRET) or recommended
// (shared service secrets) in production.
const minSecretLength = 32

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

		// CORS / cookies / proxies / request limits
		CORSOrigins:       splitList(getEnv("CORS_ORIGIN", DefaultCORSOrigin)),
		TrustedProxyCIDRs: splitList(os.Getenv("API_TRUSTED_PROXY_CIDRS")),
		MaxBodyBytes:      getEnvInt64("MAX_BODY_BYTES", 1<<20),
		RateLimits: RateLimits{
			LoginPerMinute:      int(getEnvInt64("RATE_LIMIT_LOGIN_PER_MINUTE", 10)),
			LoginEmailPerHour:   int(getEnvInt64("RATE_LIMIT_LOGIN_EMAIL_PER_HOUR", 20)),
			RegisterPerHour:     int(getEnvInt64("RATE_LIMIT_REGISTER_PER_HOUR", 5)),
			RefreshPerMinute:    int(getEnvInt64("RATE_LIMIT_REFRESH_PER_MINUTE", 60)),
			RegistrationPerHour: int(getEnvInt64("RATE_LIMIT_DCR_PER_HOUR", 10)),
		},

		// Bot
		BotInternalSecret: os.Getenv("BOT_INTERNAL_SECRET"),

		// OAuth / Hydra (MCP authorization)
		HydraIssuerURL:      getEnv("HYDRA_ISSUER_URL", "http://localhost:4444"),
		HydraAdminURL:       getEnv("HYDRA_ADMIN_URL", "http://localhost:4445"),
		MCPResourceURL:      getEnv("MCP_RESOURCE_URL", "http://localhost:8081"),
		OAuthScopes:         getEnv("OAUTH_SCOPES", DefaultOAuthScopes),
		HydraPinnedClientID: os.Getenv("HYDRA_PINNED_CLIENT_ID"),

		// MCP write access
		MCPWriteEnabled:       getEnvBool("MCP_WRITE_ENABLED", false),
		MCPMaxWriteAmount:     getEnvInt64("MCP_MAX_WRITE_AMOUNT", 1_000_000),
		MCPWriteRatePerMinute: int(getEnvInt64("MCP_WRITE_RATE_PER_MINUTE", 30)),
		MCPWriteRatePerDay:    int(getEnvInt64("MCP_WRITE_RATE_PER_DAY", 500)),
		MCPTrustedProxyCIDRs:  splitList(os.Getenv("MCP_TRUSTED_PROXY_CIDRS")),

		// Storage (receipt attachments)
		StorageEndpoint:     os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:       os.Getenv("STORAGE_BUCKET"),
		StorageAccessKey:    os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecretKey:    os.Getenv("STORAGE_SECRET_KEY"),
		StorageUsePathStyle: getEnvBool("STORAGE_USE_PATH_STYLE", true),
		MaxUploadBytes:      getEnvInt64("MAX_UPLOAD_BYTES", 10*1024*1024),
		MaxAttachmentsPerTx: int(getEnvInt64("MAX_ATTACHMENTS_PER_TX", 10)),
	}

	config.OAuthScopes = withWriteScopes(config.OAuthScopes, config.MCPWriteEnabled)
	config.CookieSecure = getEnvBool("COOKIE_SECURE", config.Env != Development)

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
	if len(c.JWTSecret) < minSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d bytes in production", minSecretLength)
	}
	if c.DBPassword == "kuberan" {
		return fmt.Errorf("DB_PASSWORD must not be the default in production")
	}
	if c.StorageBucket != "" && c.StorageSecretKey == "" {
		return fmt.Errorf("STORAGE_SECRET_KEY must be set when STORAGE_BUCKET is configured in production")
	}
	if len(c.CORSOrigins) == 0 {
		return fmt.Errorf("CORS_ORIGIN must list the allowed web origins in production")
	}
	for _, o := range c.CORSOrigins {
		if strings.Contains(o, "*") {
			return fmt.Errorf("CORS_ORIGIN must not contain wildcards in production (got %q)", o)
		}
	}

	log := logger.Get()
	if c.DBSSLMode == "disable" {
		log.Warn("DB_SSLMODE=disable in production: database traffic is unencrypted (fine only on a private Docker network)")
	}
	if c.PipelineAPIKey != "" && len(c.PipelineAPIKey) < minSecretLength {
		log.Warnf("PIPELINE_API_KEY is shorter than %d characters; use a longer random key", minSecretLength)
	}
	if c.BotInternalSecret != "" && len(c.BotInternalSecret) < minSecretLength {
		log.Warnf("BOT_INTERNAL_SECRET is shorter than %d characters; use a longer random secret", minSecretLength)
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

// withWriteScopes returns the scope set with write:* scopes present only when
// MCP writes are enabled: it strips any write:* scope (e.g. one listed in an
// OAUTH_SCOPES override) when disabled, and adds DefaultOAuthWriteScopes when
// enabled, so the flag alone decides whether write access can be granted.
func withWriteScopes(scopes string, writeEnabled bool) string {
	out := make([]string, 0)
	for _, s := range strings.Fields(scopes) {
		if !strings.HasPrefix(s, "write:") {
			out = append(out, s)
		}
	}
	if writeEnabled {
		out = append(out, strings.Fields(DefaultOAuthWriteScopes)...)
	}
	return strings.Join(out, " ")
}

// splitList splits a comma-separated environment value, dropping blanks.
func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
