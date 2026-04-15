package persistence

import (
	"context"
	"log/slog"

	"github.com/poly-workshop/identra/internal/domain"
	"gorm.io/gorm"
)

type gormExternalIdentityStore struct {
	db *gorm.DB
}

// NewGormExternalIdentityStore creates a new GORM-based external identity store.
func NewGormExternalIdentityStore(db *gorm.DB) domain.ExternalIdentityStore {
	return &gormExternalIdentityStore{db: db}
}

func (r *gormExternalIdentityStore) Create(
	ctx context.Context,
	identity *domain.ExternalIdentityModel,
) error {
	err := r.db.WithContext(ctx).Create(identity).Error
	if err != nil {
		slog.ErrorContext(
			ctx,
			"failed to create external identity",
			"error", err,
			"provider", identity.Provider,
			"provider_user_id", identity.ProviderUserID,
		)
		return wrapGormError(err)
	}
	slog.InfoContext(
		ctx,
		"external identity created successfully",
		"id", identity.ID,
		"user_id", identity.UserID,
		"provider", identity.Provider,
	)
	return nil
}

func (r *gormExternalIdentityStore) GetByProviderID(
	ctx context.Context,
	provider, providerUserID string,
) (*domain.ExternalIdentityModel, error) {
	var identity domain.ExternalIdentityModel
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&identity).Error
	if err != nil {
		return nil, wrapGormError(err)
	}
	return &identity, nil
}

func (r *gormExternalIdentityStore) GetByUserID(
	ctx context.Context,
	userID string,
) ([]*domain.ExternalIdentityModel, error) {
	var identities []*domain.ExternalIdentityModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&identities).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to list external identities", "error", err, "user_id", userID)
		return nil, err
	}
	return identities, nil
}
