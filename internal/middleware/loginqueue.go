package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GlobalLoginLimiter is the interface LoginQueue expects.
// Defined here to avoid import cycles and keep the middleware decoupled.
type GlobalLoginLimiter interface {
	Allow(ctx context.Context) (allowed bool, retryAfter time.Duration, err error)
}

// LoginQueue rejects excess login traffic with 429 + Retry-After when the
// global token bucket is empty. This is a system-wide throttle, applied in
// addition to the per-user brute-force limiter. A nil limiter disables the
// middleware so it can be toggled off via config.
func LoginQueue(limiter GlobalLoginLimiter) gin.HandlerFunc {
	if limiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		allowed, retryAfter, err := limiter.Allow(c.Request.Context())
		if err != nil {
			// Fail open: Redis hiccups shouldn't block logins.
			c.Next()
			return
		}
		if allowed {
			c.Next()
			return
		}

		RecordGlobalLoginThrottle()

		secs := int(math.Ceil(retryAfter.Seconds()))
		if secs < 1 {
			secs = 1
		}
		c.Header("Retry-After", strconv.Itoa(secs))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"error": gin.H{
				"code":        "LOGIN_THROTTLED",
				"message":     "Service is temporarily rate-limiting logins. Please retry shortly.",
				"retry_after": secs,
			},
		})
	}
}
