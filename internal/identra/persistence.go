package identra

import (
	"context"
)

type UserStore interface {
	Create(ctx context.Context, user *UserModel) error
	GetByID(ctx context.Context, id string) (*UserModel, error)
	GetByEmail(ctx context.Context, email string) (*UserModel, error)
	Update(ctx context.Context, user *UserModel) error
	SetPassword(ctx context.Context, id, hashedPassword string) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, offset, limit int) ([]*UserModel, error)
	Count(ctx context.Context) (int64, error)
}

type ExternalIdentityStore interface {
	// Create persists a new external identity. Returns ErrAlreadyExists
	// if the (provider, provider_user_id) pair is already linked to a user.
	Create(ctx context.Context, identity *ExternalIdentityModel) error
	// GetByProviderID looks up a single external identity by (provider, provider_user_id).
	// Returns ErrNotFound if no match exists.
	GetByProviderID(ctx context.Context, provider, providerUserID string) (*ExternalIdentityModel, error)
	// GetByUserID returns all external identities linked to the given user.
	GetByUserID(ctx context.Context, userID string) ([]*ExternalIdentityModel, error)
}
