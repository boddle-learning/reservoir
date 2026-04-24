package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// GlobalLimiter is a Redis-backed token bucket applied across all instances.
// Unlike Limiter (per-user brute-force guard), this caps total system throughput
// for a shared key — the thundering-herd scenario after a dependency outage.
type GlobalLimiter struct {
	client   *redis.Client
	key      string
	capacity int     // bucket size (burst allowance)
	refill   float64 // tokens added per second
}

// tokenBucketScript atomically refills the bucket based on elapsed time and
// consumes one token if available. Returns:
//
//	[1, 0]                    -> allowed
//	[0, retryAfterMillis]     -> rejected
//
// State stored in a hash: {tokens: float, ts: int64 millis}. TTL is reset each
// call to the time needed to fully refill, so idle keys expire on their own.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local data = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])

if tokens == nil then
  tokens = capacity
  ts = now_ms
end

local elapsed_ms = now_ms - ts
if elapsed_ms < 0 then elapsed_ms = 0 end
tokens = math.min(capacity, tokens + (elapsed_ms / 1000.0) * refill)

local allowed = 0
local retry_after_ms = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry_after_ms = math.ceil(((1 - tokens) / refill) * 1000)
end

redis.call("HSET", key, "tokens", tokens, "ts", now_ms)
-- TTL: time to fully refill an empty bucket, plus a small buffer
local ttl = math.ceil((capacity / refill) + 5)
redis.call("EXPIRE", key, ttl)

return {allowed, retry_after_ms}
`)

// NewGlobalLimiter creates a new global token-bucket limiter.
// capacity is the burst size; refillPerSecond is the steady-state rate.
func NewGlobalLimiter(client *redis.Client, key string, capacity int, refillPerSecond float64) *GlobalLimiter {
	return &GlobalLimiter{
		client:   client,
		key:      key,
		capacity: capacity,
		refill:   refillPerSecond,
	}
}

// Allow attempts to consume one token. If rejected, retryAfter is how long
// the caller should wait before retrying.
func (g *GlobalLimiter) Allow(ctx context.Context) (allowed bool, retryAfter time.Duration, err error) {
	nowMs := time.Now().UnixMilli()
	res, err := tokenBucketScript.Run(ctx, g.client, []string{g.key},
		g.capacity, g.refill, nowMs,
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("token bucket script failed: %w", err)
	}
	if len(res) != 2 {
		return false, 0, fmt.Errorf("unexpected token bucket result: %v", res)
	}
	if res[0] == 1 {
		return true, 0, nil
	}
	return false, time.Duration(res[1]) * time.Millisecond, nil
}
