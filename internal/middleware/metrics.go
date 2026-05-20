package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP request metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Authentication metrics
	authLoginAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_login_attempts_total",
			Help: "Total number of login attempts",
		},
		[]string{"method", "status"}, // method: email/google/clever/token, status: success/failure/blocked
	)

	authLoginDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "auth_login_duration_seconds",
			Help:    "Login request duration in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"method"},
	)

	authJWTValidatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_jwt_validated_total",
			Help: "Total number of JWT validations",
		},
		[]string{"status"}, // status: success/failure/expired/revoked
	)

	authRateLimitHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "auth_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
	)

	// Active tokens gauge
	authActiveTokens = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "auth_active_tokens",
			Help: "Number of active (non-blacklisted) JWT tokens",
		},
	)
)

// Metrics creates a Prometheus metrics middleware
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// RecordLoginAttempt records a login attempt metric
func RecordLoginAttempt(method, status string, duration time.Duration) {
	authLoginAttemptsTotal.WithLabelValues(method, status).Inc()
	authLoginDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordJWTValidation records a JWT validation metric
func RecordJWTValidation(status string) {
	authJWTValidatedTotal.WithLabelValues(status).Inc()
}

// RecordRateLimitHit records a rate limit hit
func RecordRateLimitHit() {
	authRateLimitHitsTotal.Inc()
}

// SetActiveTokens sets the active tokens gauge
func SetActiveTokens(count int) {
	authActiveTokens.Set(float64(count))
}
