package store

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/slhmy/identra/internal/identra"
	gormdb "github.com/slhmy/identra/internal/store/gorm"
)

type storeFactory func(t *testing.T) (identra.UserStore, identra.ExternalIdentityStore)

func TestGormStores_UserStoreContract(t *testing.T) {
	runUserStoreContract(t, newGormStores)
}

func TestGormStores_ExternalIdentityStoreContract(t *testing.T) {
	runExternalIdentityStoreContract(t, newGormStores)
}

func newGormStores(t *testing.T) (identra.UserStore, identra.ExternalIdentityStore) {
	t.Helper()

	db, err := gormdb.NewDB(gormdb.Config{
		Driver: "sqlite",
		DbName: filepath.Join(t.TempDir(), "identra-contract.db"),
	})
	if err != nil {
		t.Fatalf("new gorm db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	if err := AutoMigrateGorm(db); err != nil {
		t.Fatalf("migrate gorm db: %v", err)
	}

	return NewGormUserStore(db), NewGormExternalIdentityStore(db)
}

func runUserStoreContract(t *testing.T, factory storeFactory) {
	t.Helper()
	ctx := context.Background()
	userStore, _ := factory(t)

	user := &identra.UserModel{Email: "user@example.com"}
	if err := userStore.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.ID == "" {
		t.Fatal("create user did not populate ID")
	}
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Fatalf("create user did not populate timestamps: created=%v updated=%v", user.CreatedAt, user.UpdatedAt)
	}

	duplicate := &identra.UserModel{Email: user.Email}
	if err := userStore.Create(ctx, duplicate); !errors.Is(err, identra.ErrAlreadyExists) {
		t.Fatalf("duplicate email error = %v, want %v", err, identra.ErrAlreadyExists)
	}

	byID, err := userStore.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if byID.Email != user.Email {
		t.Fatalf("get by id email = %q, want %q", byID.Email, user.Email)
	}

	byEmail, err := userStore.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Fatalf("get by email id = %q, want %q", byEmail.ID, user.ID)
	}

	lastLogin := time.Now().UTC().Round(time.Second)
	hash := "hash"
	user.LastLoginAt = &lastLogin
	user.HashedPassword = &hash
	if err := userStore.Update(ctx, user); err != nil {
		t.Fatalf("update user: %v", err)
	}

	updated, err := userStore.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}
	if updated.HashedPassword == nil || *updated.HashedPassword != hash {
		t.Fatalf("updated hashed password = %v, want %q", updated.HashedPassword, hash)
	}
	if updated.LastLoginAt == nil || !updated.LastLoginAt.Equal(lastLogin) {
		t.Fatalf("updated last login = %v, want %v", updated.LastLoginAt, lastLogin)
	}

	second := &identra.UserModel{Email: "second@example.com"}
	if err := userStore.Create(ctx, second); err != nil {
		t.Fatalf("create second user: %v", err)
	}

	count, err := userStore.Count(ctx)
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	listed, err := userStore.List(ctx, 0, 10)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("list len = %d, want 2", len(listed))
	}

	if err := userStore.Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := userStore.GetByID(ctx, user.ID); !errors.Is(err, identra.ErrNotFound) {
		t.Fatalf("get deleted user error = %v, want %v", err, identra.ErrNotFound)
	}
	if err := userStore.Delete(ctx, user.ID); !errors.Is(err, identra.ErrNotFound) {
		t.Fatalf("delete missing user error = %v, want %v", err, identra.ErrNotFound)
	}
}

func runExternalIdentityStoreContract(t *testing.T, factory storeFactory) {
	t.Helper()
	ctx := context.Background()
	userStore, identityStore := factory(t)

	user := &identra.UserModel{Email: "oauth-user@example.com"}
	if err := userStore.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	identity := &identra.ExternalIdentityModel{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "github-123",
	}
	if err := identityStore.Create(ctx, identity); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if identity.ID == "" {
		t.Fatal("create identity did not populate ID")
	}
	if identity.CreatedAt.IsZero() || identity.UpdatedAt.IsZero() {
		t.Fatalf("create identity did not populate timestamps: created=%v updated=%v", identity.CreatedAt, identity.UpdatedAt)
	}

	duplicate := &identra.ExternalIdentityModel{
		UserID:         user.ID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
	}
	if err := identityStore.Create(ctx, duplicate); !errors.Is(err, identra.ErrAlreadyExists) {
		t.Fatalf("duplicate provider identity error = %v, want %v", err, identra.ErrAlreadyExists)
	}

	byProvider, err := identityStore.GetByProviderID(ctx, identity.Provider, identity.ProviderUserID)
	if err != nil {
		t.Fatalf("get by provider id: %v", err)
	}
	if byProvider.UserID != user.ID {
		t.Fatalf("provider identity user id = %q, want %q", byProvider.UserID, user.ID)
	}

	second := &identra.ExternalIdentityModel{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "google-123",
	}
	if err := identityStore.Create(ctx, second); err != nil {
		t.Fatalf("create second identity: %v", err)
	}

	byUser, err := identityStore.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by user id: %v", err)
	}
	providers := make([]string, 0, len(byUser))
	for _, item := range byUser {
		providers = append(providers, item.Provider)
	}
	sort.Strings(providers)
	if got := joinProviders(providers); got != "github,google" {
		t.Fatalf("providers = %q, want github,google", got)
	}

	if _, err := identityStore.GetByProviderID(ctx, "github", "missing"); !errors.Is(err, identra.ErrNotFound) {
		t.Fatalf("get missing identity error = %v, want %v", err, identra.ErrNotFound)
	}
}

func joinProviders(providers []string) string {
	if len(providers) == 0 {
		return ""
	}
	out := providers[0]
	for _, provider := range providers[1:] {
		out += "," + provider
	}
	return out
}
