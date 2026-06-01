package middleware

import (
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// sensitiveQueryKeys are query-string parameters whose values are credentials
// and must never be written to logs. The magic-link secret (`token`) is the
// reason this exists (security review Finding 3 / LMS-6514); the OAuth
// authorization `code` and the others are redacted as defense-in-depth since
// they also appear in query strings on callback routes.
var sensitiveQueryKeys = map[string]bool{
	"token":         true,
	"secret":        true,
	"password":      true,
	"code":          true,
	"access_token":  true,
	"refresh_token": true,
}

// redactQuery returns the raw query string with the values of any sensitive
// keys replaced by "REDACTED". If the query can't be parsed it returns a fixed
// placeholder rather than risk logging an unparsed secret.
func redactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[unparseable query redacted]"
	}
	redacted := false
	for key := range values {
		if sensitiveQueryKeys[key] {
			values.Set(key, "REDACTED")
			redacted = true
		}
	}
	if !redacted {
		return rawQuery
	}
	return values.Encode()
}

// Logger creates a logging middleware using zap
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := redactQuery(c.Request.URL.RawQuery)

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get status
		status := c.Writer.Status()

		// Log request
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
		)

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, e := range c.Errors {
				logger.Error("request error", zap.Error(e.Err))
			}
		}
	}
}
