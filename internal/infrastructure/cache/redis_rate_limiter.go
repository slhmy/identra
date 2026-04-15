package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter provides rate limiting and brute-force protection based on a
// per-key attempt counter stored in Redis.
type RateLimiter interface {
	// IsAllowed returns true when the number of recorded attempts for key is
	// strictly below the configured maximum. It does NOT modify the counter.
	IsAllowed(ctx context.Context, key string) (bool, error)

	// Record increments the attempt counter for key. On the very first
	// increment the key's TTL is set to the configured window duration.
	Record(ctx context.Context, key string) error

	// Reset deletes the attempt counter for key (call after a successful
	// action to give the user a fresh window).
	Reset(ctx context.Context, key string) error
}

// NewRedisRateLimiter creates a Redis-backed RateLimiter.
//
//   - prefix      key prefix (e.g. "identra:rl:login:")
//   - maxAttempts maximum number of attempts allowed within window before
//     IsAllowed returns false
//   - window      duration after which the counter automatically expires
func NewRedisRateLimiter(rdb redis.UniversalClient, prefix string, maxAttempts int, window time.Duration) (RateLimiter, error) {
	if rdb == nil {
		return nil, errors.New("redis client is required for rate limiter")
	}
	if maxAttempts <= 0 {
		return nil, errors.New("maxAttempts must be positive")
	}
	if window <= 0 {
		return nil, errors.New("window must be positive")
	}
	return &redisRateLimiter{
		rdb:         rdb,
		prefix:      prefix,
		maxAttempts: int64(maxAttempts),
		window:      window,
	}, nil
}

type redisRateLimiter struct {
	rdb         redis.UniversalClient
	prefix      string
	maxAttempts int64
	window      time.Duration
}

func (r *redisRateLimiter) fullKey(key string) string {
	return r.prefix + key
}

// isAllowedScript checks whether the current counter is below maxAttempts.
// KEYS[1]: counter key
// ARGV[1]: maxAttempts
var isAllowedScript = redis.NewScript(`
local v = redis.call("GET", KEYS[1])
if not v then return 1 end
if tonumber(v) < tonumber(ARGV[1]) then return 1 end
return 0
`)

func (r *redisRateLimiter) IsAllowed(ctx context.Context, key string) (bool, error) {
	res, err := isAllowedScript.Run(ctx, r.rdb, []string{r.fullKey(key)}, r.maxAttempts).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// recordScript increments the counter and sets the TTL on the first increment.
// KEYS[1]: counter key
// ARGV[1]: window in seconds
var recordScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
    redis.call("EXPIRE", KEYS[1], tonumber(ARGV[1]))
end
return n
`)

func (r *redisRateLimiter) Record(ctx context.Context, key string) error {
	return recordScript.Run(ctx, r.rdb, []string{r.fullKey(key)}, int(r.window.Seconds())).Err()
}

func (r *redisRateLimiter) Reset(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, r.fullKey(key)).Err()
}
