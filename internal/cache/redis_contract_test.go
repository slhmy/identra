package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	redisconfig "github.com/slhmy/identra/internal/cache/redis"
)

func TestRedisEmailCodeStoreContract(t *testing.T) {
	ctx, rdb := newRedisContractClient(t)

	store, err := NewRedisEmailCodeStore(time.Minute, rdb)
	if err != nil {
		t.Fatalf("new email code store: %v", err)
	}

	email := "contract-" + uuid.NewString() + "@example.com"
	if err := store.Set(ctx, email, "123456"); err != nil {
		t.Fatalf("set email code: %v", err)
	}

	ok, err := store.Consume(ctx, email, "000000")
	if err != nil {
		t.Fatalf("consume wrong email code: %v", err)
	}
	if ok {
		t.Fatal("wrong code consumed successfully")
	}

	ok, err = store.Consume(ctx, email, "123456")
	if err != nil {
		t.Fatalf("consume correct email code: %v", err)
	}
	if !ok {
		t.Fatal("correct code did not consume successfully")
	}

	ok, err = store.Consume(ctx, email, "123456")
	if err != nil {
		t.Fatalf("consume used email code: %v", err)
	}
	if ok {
		t.Fatal("used code consumed twice")
	}
}

func TestRedisOAuthStateStoreContract(t *testing.T) {
	ctx, rdb := newRedisContractClient(t)

	store, err := NewRedisOAuthStateStore(time.Minute, rdb)
	if err != nil {
		t.Fatalf("new oauth state store: %v", err)
	}

	state := "state-" + uuid.NewString()
	if err := store.Add(ctx, state, "github", "https://app.example.com/callback"); err != nil {
		t.Fatalf("add oauth state: %v", err)
	}

	data, ok, err := store.Consume(ctx, state)
	if err != nil {
		t.Fatalf("consume oauth state: %v", err)
	}
	if !ok {
		t.Fatal("stored oauth state was not found")
	}
	if data.Provider != "github" || data.RedirectURL != "https://app.example.com/callback" {
		t.Fatalf("oauth state = %+v, want provider github and callback redirect", data)
	}

	_, ok, err = store.Consume(ctx, state)
	if err != nil {
		t.Fatalf("consume used oauth state: %v", err)
	}
	if ok {
		t.Fatal("oauth state consumed twice")
	}
}

func TestRedisRefreshTokenRevocationStoreContract(t *testing.T) {
	ctx, rdb := newRedisContractClient(t)

	store, err := NewRedisRefreshTokenRevocationStore(rdb)
	if err != nil {
		t.Fatalf("new refresh token revocation store: %v", err)
	}

	tokenID := "refresh-" + uuid.NewString()
	revoked, err := store.IsRevoked(ctx, tokenID)
	if err != nil {
		t.Fatalf("check initial revocation: %v", err)
	}
	if revoked {
		t.Fatal("new token id starts revoked")
	}

	if err := store.Revoke(ctx, tokenID, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	revoked, err = store.IsRevoked(ctx, tokenID)
	if err != nil {
		t.Fatalf("check revoked token: %v", err)
	}
	if !revoked {
		t.Fatal("revoked token not reported as revoked")
	}

	expiredID := "expired-" + uuid.NewString()
	if err := store.Revoke(ctx, expiredID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("revoke expired token: %v", err)
	}
	revoked, err = store.IsRevoked(ctx, expiredID)
	if err != nil {
		t.Fatalf("check expired token: %v", err)
	}
	if revoked {
		t.Fatal("expired token revocation should not be stored")
	}
}

func TestRedisRateLimiterContract(t *testing.T) {
	ctx, rdb := newRedisContractClient(t)

	limiter, err := NewRedisRateLimiter(rdb, "identra:test:rl:"+uuid.NewString()+":", 2, time.Minute)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	key := "subject"
	allowed, err := limiter.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("initial is allowed: %v", err)
	}
	if !allowed {
		t.Fatal("new key should be allowed")
	}

	if err := limiter.Record(ctx, key); err != nil {
		t.Fatalf("record first attempt: %v", err)
	}
	allowed, err = limiter.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("is allowed after one record: %v", err)
	}
	if !allowed {
		t.Fatal("key should be allowed after one attempt")
	}

	if err := limiter.Record(ctx, key); err != nil {
		t.Fatalf("record second attempt: %v", err)
	}
	allowed, err = limiter.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("is allowed after max records: %v", err)
	}
	if allowed {
		t.Fatal("key should be blocked after max attempts")
	}

	if err := limiter.Reset(ctx, key); err != nil {
		t.Fatalf("reset key: %v", err)
	}
	allowed, err = limiter.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("is allowed after reset: %v", err)
	}
	if !allowed {
		t.Fatal("key should be allowed after reset")
	}
}

func newRedisContractClient(t *testing.T) (context.Context, goredis.UniversalClient) {
	t.Helper()

	redisURL := os.Getenv("IDENTRA_REDIS_URL")
	if redisURL == "" {
		t.Skip("set IDENTRA_REDIS_URL to run Redis cache contract tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	rdb, err := redisconfig.NewRDB(redisconfig.Config{
		Urls:     []string{redisURL},
		Password: os.Getenv("IDENTRA_REDIS_PASSWORD"),
	})
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}

	return ctx, rdb
}
