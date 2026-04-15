package cache

import (
"context"
"encoding/json"
"errors"
"time"

"github.com/poly-workshop/identra/internal/infrastructure/oauth"
goredis "github.com/redis/go-redis/v9"
)

// NewRedisOAuthStateStore creates a Redis-backed OAuth state store.
// Expiry is enforced by Redis TTL; the ExpiresAt field of returned State is not populated.
func NewRedisOAuthStateStore(ttl time.Duration, rdb goredis.UniversalClient) (oauth.StateStore, error) {
if ttl <= 0 {
ttl = time.Minute
}
if rdb == nil {
return nil, errors.New("redis client is required for oauth state store")
}
return &redisOAuthStateStore{rdb: rdb, ttl: ttl, prefix: "identra:oauth_state:"}, nil
}

type redisOAuthStateStore struct {
rdb    goredis.UniversalClient
ttl    time.Duration
prefix string
}

type oauthStateValue struct {
Provider    string `json:"provider"`
RedirectURL string `json:"redirect_url"`
}

func (s *redisOAuthStateStore) key(state string) string {
return s.prefix + state
}

// Add stores the state with its provider and redirect URL in Redis with a TTL.
func (s *redisOAuthStateStore) Add(ctx context.Context, state, provider, redirectURL string) error {
val, err := json.Marshal(oauthStateValue{Provider: provider, RedirectURL: redirectURL})
if err != nil {
return err
}
return s.rdb.Set(ctx, s.key(state), val, s.ttl).Err()
}

// consumeStateScript atomically retrieves and deletes the state key.
// Returns the value if found, or nil if not present.
var consumeStateScript = goredis.NewScript(`
local v = redis.call("GET", KEYS[1])
if not v then return false end
redis.call("DEL", KEYS[1])
return v
`)

// Consume retrieves and atomically removes the state from Redis.
// Returns false (with no error) when the state is not found or has expired.
// ExpiresAt is not populated in the returned State because Redis enforces expiry via TTL.
func (s *redisOAuthStateStore) Consume(ctx context.Context, state string) (oauth.State, bool, error) {
res, err := consumeStateScript.Run(ctx, s.rdb, []string{s.key(state)}).Text()
if err != nil {
if errors.Is(err, goredis.Nil) {
return oauth.State{}, false, nil
}
return oauth.State{}, false, err
}

var val oauthStateValue
if err := json.Unmarshal([]byte(res), &val); err != nil {
return oauth.State{}, false, err
}

return oauth.State{
Provider:    val.Provider,
RedirectURL: val.RedirectURL,
}, true, nil
}
