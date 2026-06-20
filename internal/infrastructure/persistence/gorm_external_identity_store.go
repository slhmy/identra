package persistence

import (
	"context"
	"log/slog"

	"github.com/poly-workshop/identra/internal/domain"
	"github.com/poly-workshop/identra/internal/ports"
	"gorm.io/gorm"
)

type gormExternalIdentityStore struct {
	db *gorm.DB
}

// NewGormExternalIdentityStore creates a new GORM-based external identity store.
func NewGormExternalIdentityStore(db *gorm.DB) ports.ExternalIdentityStore {
	return &gormExternalIdentityStore{db: db}
}

func (r *gormExternalIdentityStore) Create(
	ctx context.Context,
	identity *domain.ExternalIdentityModel,
) error {
	record := externalIdentityRecordFromDomain(identity)
	err := r.db.WithContext(ctx).Create(&record).Error
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
	copyExternalIdentityRecordToDomain(record, identity)
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
	var record externalIdentityRecord
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&record).Error
	if err != nil {
		return nil, wrapGormError(err)
	}
	return externalIdentityRecordToDomain(record), nil
}

func (r *gormExternalIdentityStore) GetByUserID(
	ctx context.Context,
	userID string,
) ([]*domain.ExternalIdentityModel, error) {
	var records []externalIdentityRecord
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&records).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to list external identities", "error", err, "user_id", userID)
		return nil, err
	}
	identities := make([]*domain.ExternalIdentityModel, 0, len(records))
	for _, record := range records {
		identities = append(identities, externalIdentityRecordToDomain(record))
	}
	return identities, nil
}
