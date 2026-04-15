package identra

import (
	"context"
	"testing"

	"github.com/poly-workshop/identra/internal/domain"
)

// mockUserStore is a simple in-memory user store for testing.
type mockUserStore struct {
	users          map[string]*domain.UserModel
	forceCreateErr error // when set, Create returns this error instead of storing the user
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[string]*domain.UserModel),
	}
}

func (m *mockUserStore) Create(ctx context.Context, user *domain.UserModel) error {
	if m.forceCreateErr != nil {
		return m.forceCreateErr
	}
	if user.ID == "" {
		user.ID = "test-user-id"
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserStore) GetByID(ctx context.Context, id string) (*domain.UserModel, error) {
	if user, ok := m.users[id]; ok {
		return user, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*domain.UserModel, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserStore) Update(ctx context.Context, user *domain.UserModel) error {
	if _, ok := m.users[user.ID]; !ok {
		return domain.ErrNotFound
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserStore) Delete(ctx context.Context, id string) error {
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}

func (m *mockUserStore) List(ctx context.Context, offset, limit int) ([]*domain.UserModel, error) {
	result := make([]*domain.UserModel, 0, len(m.users))
	for _, user := range m.users {
		result = append(result, user)
	}
	return result, nil
}

func (m *mockUserStore) Count(ctx context.Context) (int64, error) {
	return int64(len(m.users)), nil
}

// mockExternalIdentityStore is a simple in-memory external identity store for testing.
type mockExternalIdentityStore struct {
	identities     []*domain.ExternalIdentityModel
	forceCreateErr error
}

func newMockExternalIdentityStore() *mockExternalIdentityStore {
	return &mockExternalIdentityStore{}
}

func (m *mockExternalIdentityStore) Create(_ context.Context, identity *domain.ExternalIdentityModel) error {
	if m.forceCreateErr != nil {
		return m.forceCreateErr
	}
	if identity.ID == "" {
		identity.ID = "test-identity-id"
	}
	m.identities = append(m.identities, identity)
	return nil
}

func (m *mockExternalIdentityStore) GetByProviderID(
	_ context.Context,
	provider, providerUserID string,
) (*domain.ExternalIdentityModel, error) {
	for _, id := range m.identities {
		if id.Provider == provider && id.ProviderUserID == providerUserID {
			return id, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockExternalIdentityStore) GetByUserID(
	_ context.Context,
	userID string,
) ([]*domain.ExternalIdentityModel, error) {
	var result []*domain.ExternalIdentityModel
	for _, id := range m.identities {
		if id.UserID == userID {
			result = append(result, id)
		}
	}
	return result, nil
}

func TestEnsureOAuthUser_WithEmail(t *testing.T) {
	store := newMockUserStore()
	extStore := newMockExternalIdentityStore()
	svc := &Service{userStore: store, externalIdentityStore: extStore}

	info := UserInfo{
		Provider: "github",
		ID:       "github123",
		Email:    "user@example.com",
	}

	user, err := svc.ensureOAuthUser(context.Background(), info)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if user.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %q", user.Email)
	}

	// Verify external identity was created with correct fields.
	id, lookupErr := extStore.GetByProviderID(context.Background(), "github", "github123")
	if lookupErr != nil {
		t.Fatalf("expected external identity to exist, got %v", lookupErr)
	}
	if id.UserID != user.ID {
		t.Errorf("expected external identity user_id %q, got %q", user.ID, id.UserID)
	}
}

func TestEnsureOAuthUser_WithoutEmail(t *testing.T) {
	store := newMockUserStore()
	extStore := newMockExternalIdentityStore()
	svc := &Service{userStore: store, externalIdentityStore: extStore}

	info := UserInfo{
		Provider: "github",
		ID:       "github456",
		Email:    "",
	}

	user, err := svc.ensureOAuthUser(context.Background(), info)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if user.Email != "" {
		t.Errorf("expected empty email, got %q", user.Email)
	}

	id, lookupErr := extStore.GetByProviderID(context.Background(), "github", "github456")
	if lookupErr != nil {
		t.Fatalf("expected external identity to exist, got %v", lookupErr)
	}
	if id.UserID != user.ID {
		t.Errorf("expected external identity user_id %q, got %q", user.ID, id.UserID)
	}
}

func TestEnsureOAuthUser_ExistingUserByProviderID(t *testing.T) {
	store := newMockUserStore()
	extStore := newMockExternalIdentityStore()
	svc := &Service{userStore: store, externalIdentityStore: extStore}

	existingUser := &domain.UserModel{
		ID:    "existing-user-id",
		Email: "existing@example.com",
	}
	_ = store.Create(context.Background(), existingUser)
	_ = extStore.Create(context.Background(), &domain.ExternalIdentityModel{
		ID:             "eid1",
		UserID:         "existing-user-id",
		Provider:       "github",
		ProviderUserID: "github789",
	})

	info := UserInfo{
		Provider: "github",
		ID:       "github789",
		Email:    "different@example.com",
	}

	user, err := svc.ensureOAuthUser(context.Background(), info)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if user.ID != existingUser.ID {
		t.Errorf("expected existing user ID, got %q", user.ID)
	}
	// Email should be updated on the user.
	if user.Email != "different@example.com" {
		t.Errorf("expected email to be updated to 'different@example.com', got %q", user.Email)
	}
}

func TestEnsureOAuthUser_LinkExistingUserByEmail(t *testing.T) {
	store := newMockUserStore()
	extStore := newMockExternalIdentityStore()
	svc := &Service{userStore: store, externalIdentityStore: extStore}

	existingUser := &domain.UserModel{
		ID:    "existing-user-id",
		Email: "existing@example.com",
	}
	_ = store.Create(context.Background(), existingUser)

	info := UserInfo{
		Provider: "github",
		ID:       "github999",
		Email:    "existing@example.com",
	}

	user, err := svc.ensureOAuthUser(context.Background(), info)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if user.ID != existingUser.ID {
		t.Errorf("expected existing user ID, got %q", user.ID)
	}

	// Verify external identity was linked to the existing user.
	id, lookupErr := extStore.GetByProviderID(context.Background(), "github", "github999")
	if lookupErr != nil {
		t.Fatalf("expected external identity to exist, got %v", lookupErr)
	}
	if id.UserID != existingUser.ID {
		t.Errorf("expected external identity user_id %q, got %q", existingUser.ID, id.UserID)
	}
}

