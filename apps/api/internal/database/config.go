package database

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"kuberan/internal/config"
)

// Connection pool defaults, overridable via DB_MAX_OPEN_CONNS,
// DB_MAX_IDLE_CONNS, DB_CONN_MAX_IDLE_TIME and DB_CONN_MAX_LIFETIME.
const (
	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 10
	defaultConnMaxIdleTime = 5 * time.Minute
	defaultConnMaxLifetime = time.Hour
)

// Config holds database configuration
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string

	// Connection pool settings.
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

// NewConfig creates a new database configuration
func NewConfig() (*Config, error) {
	// Get application configuration
	appConfig := config.Get()

	return &Config{
		Host:     appConfig.DBHost,
		Port:     appConfig.DBPort,
		User:     appConfig.DBUser,
		Password: appConfig.DBPassword,
		DBName:   appConfig.DBName,
		SSLMode:  appConfig.DBSSLMode,

		MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns),
		MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", defaultMaxIdleConns),
		ConnMaxIdleTime: envDuration("DB_CONN_MAX_IDLE_TIME", defaultConnMaxIdleTime),
		ConnMaxLifetime: envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime),
	}, nil
}

// envInt reads a positive integer from the environment, falling back to def
// when the variable is unset or invalid.
func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// envDuration reads a positive Go duration (e.g. "5m") from the environment,
// falling back to def when the variable is unset or invalid.
func envDuration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// DSN returns the PostgreSQL connection string
func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}
