package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RefreshTokenRevocationStore tracks revoked refresh-token IDs until they expire.
type RefreshTokenRevocationStore interface {
	Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// NewRedisRefreshTokenRevocationStore creates a Redis-backed refresh token revocation store.
func NewRedisRefreshTokenRevocationStore(rdb redis.UniversalClient) (RefreshTokenRevocationStore, error) {
	if rdb == nil {
		return nil, errors.New("redis client is required for refresh token revocation store")
	}
	return &redisRefreshTokenRevocationStore{rdb: rdb, prefix: "identra:revoked_refresh:"}, nil
}

type redisRefreshTokenRevocationStore struct {
	rdb    redis.UniversalClient
	prefix string
}

func (s *redisRefreshTokenRevocationStore) key(tokenID string) string {
	return s.prefix + tokenID
}

func (s *redisRefreshTokenRevocationStore) Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error {
	if tokenID == "" {
		return nil
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	return s.rdb.Set(ctx, s.key(tokenID), "1", ttl).Err()
}

func (s *redisRefreshTokenRevocationStore) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, nil
	}
	n, err := s.rdb.Exists(ctx, s.key(tokenID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
