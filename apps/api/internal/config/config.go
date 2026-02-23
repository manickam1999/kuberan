package config

import (
	"fmt"
	"os"
	"time"

	"kuberan/internal/logger"

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
}

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
	return nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
