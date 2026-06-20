package ports

import (
	"context"

	"github.com/poly-workshop/identra/internal/domain"
)

type UserStore interface {
	Create(ctx context.Context, user *domain.UserModel) error
	GetByID(ctx context.Context, id string) (*domain.UserModel, error)
	GetByEmail(ctx context.Context, email string) (*domain.UserModel, error)
	Update(ctx context.Context, user *domain.UserModel) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, offset, limit int) ([]*domain.UserModel, error)
	Count(ctx context.Context) (int64, error)
}

type ExternalIdentityStore interface {
	// Create persists a new external identity. Returns domain.ErrAlreadyExists
	// if the (provider, provider_user_id) pair is already linked to a user.
	Create(ctx context.Context, identity *domain.ExternalIdentityModel) error
	// GetByProviderID looks up a single external identity by (provider, provider_user_id).
	// Returns domain.ErrNotFound if no match exists.
	GetByProviderID(ctx context.Context, provider, providerUserID string) (*domain.ExternalIdentityModel, error)
	// GetByUserID returns all external identities linked to the given user.
	GetByUserID(ctx context.Context, userID string) ([]*domain.ExternalIdentityModel, error)
}
