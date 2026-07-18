package app

import (
	"fmt"
	"time"

	"github.com/slhmy/identra/internal/cache"
	"github.com/slhmy/identra/internal/cache/redis"
	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/identra"
)

type redisDependencies struct {
	emailCodeStore          identra.EmailCodeStore
	oauthStateStore         identra.OAuthStateStore
	loginRateLimiter        identra.RateLimiter
	sendCodeRateLimiter     identra.RateLimiter
	refreshTokenRevocations identra.RefreshTokenRevocationStore
	serviceTokenRateLimiter identra.RateLimiter
}

func buildRedisDependencies(redisCfg redis.Config, oauthCfg config.OAuthConfig) (redisDependencies, error) {
	rdb, err := redis.NewRDB(redisCfg)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	emailStore, err := cache.NewRedisEmailCodeStore(10*time.Minute, rdb)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize email code store: %w", err)
	}

	stateTTL := oauthCfg.StateExpirationDuration
	if stateTTL <= 0 {
		stateTTL = identra.DefaultOAuthStateExpiration
	}
	oauthStore, err := cache.NewRedisOAuthStateStore(stateTTL, rdb)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize oauth state store: %w", err)
	}

	loginLimiter, err := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:login:",
		identra.DefaultLoginMaxAttempts,
		identra.DefaultLoginLockoutDuration,
	)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize login rate limiter: %w", err)
	}

	sendCodeLimiter, err := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:send_code:",
		identra.DefaultSendCodeMaxAttempts,
		identra.DefaultSendCodeWindow,
	)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize send-code rate limiter: %w", err)
	}

	refreshRevocations, err := cache.NewRedisRefreshTokenRevocationStore(rdb)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize refresh token revocation store: %w", err)
	}

	serviceTokenLimiter, err := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:service_token:",
		identra.DefaultServiceTokenMaxAttempts,
		identra.DefaultServiceTokenWindow,
	)
	if err != nil {
		return redisDependencies{}, fmt.Errorf("failed to initialize service-token rate limiter: %w", err)
	}

	return redisDependencies{
		emailCodeStore:          emailStore,
		oauthStateStore:         oauthStateStoreAdapter{store: oauthStore},
		loginRateLimiter:        loginLimiter,
		sendCodeRateLimiter:     sendCodeLimiter,
		refreshTokenRevocations: refreshRevocations,
		serviceTokenRateLimiter: serviceTokenLimiter,
	}, nil
}
