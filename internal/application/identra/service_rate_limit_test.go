package identra

import (
	"context"
	"testing"

	identra_v1_pb "github.com/poly-workshop/identra/gen/go/identra/v1"
	"github.com/poly-workshop/identra/internal/domain"
	"github.com/poly-workshop/identra/internal/infrastructure/cache"
	"github.com/poly-workshop/identra/internal/infrastructure/notification/smtp"
	"github.com/poly-workshop/identra/internal/infrastructure/security"
	"google.golang.org/grpc/codes"
)

// mockEmailCodeStore is a simple in-memory email code store for testing.
type mockEmailCodeStore struct {
	codes map[string]string
}

func newMockEmailCodeStore() *mockEmailCodeStore {
	return &mockEmailCodeStore{codes: make(map[string]string)}
}

func (m *mockEmailCodeStore) Set(_ context.Context, email, code string) error {
	m.codes[email] = code
	return nil
}

func (m *mockEmailCodeStore) Consume(_ context.Context, email, code string) (bool, error) {
	v, ok := m.codes[email]
	if !ok {
		return false, nil
	}
	if v != code {
		return false, nil
	}
	delete(m.codes, email)
	return true, nil
}

// fakeMailer is a no-op mail sender for tests.
type fakeMailer struct{}

func (f *fakeMailer) SendEmail(_ smtp.Message) error { return nil }

// mockRateLimiter is a controllable RateLimiter for unit tests.
type mockRateLimiter struct {
	allowed  bool
	recorded int
	resets   int
}

func newMockRateLimiter(allowed bool) *mockRateLimiter {
	return &mockRateLimiter{allowed: allowed}
}

func (m *mockRateLimiter) IsAllowed(_ context.Context, _ string) (bool, error) {
	return m.allowed, nil
}

func (m *mockRateLimiter) Record(_ context.Context, _ string) error {
	m.recorded++
	return nil
}

func (m *mockRateLimiter) Reset(_ context.Context, _ string) error {
	m.resets++
	return nil
}

// Verify mockRateLimiter satisfies the cache.RateLimiter interface at compile time.
var _ cache.RateLimiter = (*mockRateLimiter)(nil)

// ---- LoginByPassword rate-limit tests ----

func TestLoginByPassword_RateLimit_BlocksWhenLimitExceeded(t *testing.T) {
	hash, err := security.HashPassword("correct")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store := newMockUserStore()
	_ = store.Create(context.Background(), &domain.UserModel{
		ID:             "uid1",
		Email:          "user@example.com",
		HashedPassword: &hash,
	})

	svc := &Service{
		userStore:        store,
		loginRateLimiter: newMockRateLimiter(false), // limiter says: blocked
	}

	_, loginErr := svc.LoginByPassword(context.Background(), &identra_v1_pb.LoginByPasswordRequest{
		Email:    "user@example.com",
		Password: "correct",
	})
	requireCode(t, loginErr, codes.ResourceExhausted)
}

func TestLoginByPassword_RateLimit_RecordsOnFailure(t *testing.T) {
	hash, err := security.HashPassword("correct")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store := newMockUserStore()
	_ = store.Create(context.Background(), &domain.UserModel{
		ID:             "uid1",
		Email:          "user@example.com",
		HashedPassword: &hash,
	})

	limiter := newMockRateLimiter(true)
	svc := &Service{
		userStore:        store,
		loginRateLimiter: limiter,
	}

	_, loginErr := svc.LoginByPassword(context.Background(), &identra_v1_pb.LoginByPasswordRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})
	requireCode(t, loginErr, codes.Unauthenticated)

	if limiter.recorded != 1 {
		t.Errorf("expected 1 recorded failure, got %d", limiter.recorded)
	}
	if limiter.resets != 0 {
		t.Errorf("expected no resets on failure, got %d", limiter.resets)
	}
}

func TestLoginByPassword_RateLimit_ResetsOnSuccess(t *testing.T) {
	hash, err := security.HashPassword("correct")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store := newMockUserStore()
	_ = store.Create(context.Background(), &domain.UserModel{
		ID:             "uid1",
		Email:          "user@example.com",
		HashedPassword: &hash,
	})

	limiter := newMockRateLimiter(true)
	svc := &Service{
		userStore:        store,
		tokenCfg:         newTestTokenConfig(t),
		loginRateLimiter: limiter,
	}

	_, loginErr := svc.LoginByPassword(context.Background(), &identra_v1_pb.LoginByPasswordRequest{
		Email:    "user@example.com",
		Password: "correct",
	})
	if loginErr != nil {
		t.Fatalf("expected success, got %v", loginErr)
	}

	if limiter.recorded != 0 {
		t.Errorf("expected no recorded failures on success, got %d", limiter.recorded)
	}
	if limiter.resets != 1 {
		t.Errorf("expected 1 reset on success, got %d", limiter.resets)
	}
}

// ---- LoginByEmailCode rate-limit tests ----

func TestLoginByEmailCode_RateLimit_BlocksWhenLimitExceeded(t *testing.T) {
	emailStore := newMockEmailCodeStore()
	_ = emailStore.Set(context.Background(), "user@example.com", "123456")

	svc := &Service{
		userStore:        newMockUserStore(),
		emailCodeStore:   emailStore,
		loginRateLimiter: newMockRateLimiter(false),
	}

	_, err := svc.LoginByEmailCode(context.Background(), &identra_v1_pb.LoginByEmailCodeRequest{
		Email: "user@example.com",
		Code:  "123456",
	})
	requireCode(t, err, codes.ResourceExhausted)
}

func TestLoginByEmailCode_RateLimit_RecordsOnFailure(t *testing.T) {
	emailStore := newMockEmailCodeStore()
	_ = emailStore.Set(context.Background(), "user@example.com", "123456")

	limiter := newMockRateLimiter(true)
	svc := &Service{
		userStore:        newMockUserStore(),
		emailCodeStore:   emailStore,
		loginRateLimiter: limiter,
	}

	_, err := svc.LoginByEmailCode(context.Background(), &identra_v1_pb.LoginByEmailCodeRequest{
		Email: "user@example.com",
		Code:  "wrong",
	})
	requireCode(t, err, codes.Unauthenticated)

	if limiter.recorded != 1 {
		t.Errorf("expected 1 recorded failure, got %d", limiter.recorded)
	}
	if limiter.resets != 0 {
		t.Errorf("expected no resets on failure, got %d", limiter.resets)
	}
}

func TestLoginByEmailCode_RateLimit_ResetsOnSuccess(t *testing.T) {
	emailStore := newMockEmailCodeStore()
	_ = emailStore.Set(context.Background(), "user@example.com", "123456")

	limiter := newMockRateLimiter(true)
	svc := &Service{
		userStore:        newMockUserStore(),
		emailCodeStore:   emailStore,
		tokenCfg:         newTestTokenConfig(t),
		loginRateLimiter: limiter,
	}

	_, err := svc.LoginByEmailCode(context.Background(), &identra_v1_pb.LoginByEmailCodeRequest{
		Email: "user@example.com",
		Code:  "123456",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if limiter.recorded != 0 {
		t.Errorf("expected no recorded failures on success, got %d", limiter.recorded)
	}
	if limiter.resets != 1 {
		t.Errorf("expected 1 reset on success, got %d", limiter.resets)
	}
}

// ---- SendLoginEmailCode rate-limit tests ----

func TestSendLoginEmailCode_RateLimit_BlocksWhenLimitExceeded(t *testing.T) {
	svc := &Service{
		mailer:              &fakeMailer{},
		emailCodeStore:      newMockEmailCodeStore(),
		sendCodeRateLimiter: newMockRateLimiter(false),
	}

	_, err := svc.SendLoginEmailCode(context.Background(), &identra_v1_pb.SendLoginEmailCodeRequest{
		Email: "user@example.com",
	})
	requireCode(t, err, codes.ResourceExhausted)
}

func TestSendLoginEmailCode_RateLimit_RecordsOnAllowed(t *testing.T) {
	limiter := newMockRateLimiter(true)
	svc := &Service{
		mailer:              &fakeMailer{},
		emailCodeStore:      newMockEmailCodeStore(),
		sendCodeRateLimiter: limiter,
	}

	_, err := svc.SendLoginEmailCode(context.Background(), &identra_v1_pb.SendLoginEmailCodeRequest{
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if limiter.recorded != 1 {
		t.Errorf("expected 1 recorded send, got %d", limiter.recorded)
	}
}
