package database

import (
	"time"

	gormlogger "gorm.io/gorm/logger"

	"kuberan/internal/logger"
)

// slowQueryThreshold is the duration above which GORM logs a query as slow.
const slowQueryThreshold = 200 * time.Millisecond

// zapWriter adapts the global Zap logger to GORM's logger.Writer.
type zapWriter struct{}

// Printf forwards a GORM log line to Zap at warn level (GORM only emits
// warnings, errors and slow queries at the configured level).
func (zapWriter) Printf(format string, args ...interface{}) {
	logger.Get().Warnf(format, args...)
}

// newGormLogger returns a GORM logger that writes through Zap, reporting
// errors and slow queries (without bound values) but not "record not found".
func newGormLogger() gormlogger.Interface {
	return gormlogger.New(zapWriter{}, gormlogger.Config{
		SlowThreshold:             slowQueryThreshold,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
		// Log SQL with placeholders only, never bound values (PII, hashes).
		ParameterizedQueries: true,
	})
}
