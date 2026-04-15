package persistence

import (
	"context"
	"errors"
	"log/slog"

	"github.com/poly-workshop/identra/internal/domain"
	"gorm.io/gorm"
)

type gormUserStore struct {
	db *gorm.DB
}

// NewGormUserStore creates a new GORM-based user store.
func NewGormUserStore(db *gorm.DB) domain.UserStore {
	return &gormUserStore{db: db}
}

// wrapGormError converts GORM-specific errors to domain errors.
func wrapGormError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrAlreadyExists
	}
	return err
}

func (r *gormUserStore) Create(ctx context.Context, user *domain.UserModel) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to create user", "error", err, "email", user.Email)
		return wrapGormError(err)
	}
	slog.InfoContext(
		ctx,
		"user created successfully",
		"user_id",
		user.ID,
		"email",
		user.Email,
	)
	return nil
}

func (r *gormUserStore) GetByID(ctx context.Context, id string) (*domain.UserModel, error) {
	var user domain.UserModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to get user by ID", "error", err, "user_id", id)
		return nil, wrapGormError(err)
	}
	slog.DebugContext(ctx, "user retrieved successfully", "user_id", user.ID, "email", user.Email)
	return &user, nil
}

func (r *gormUserStore) GetByEmail(ctx context.Context, email string) (*domain.UserModel, error) {
	var user domain.UserModel
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, wrapGormError(err)
	}
	return &user, nil
}

func (r *gormUserStore) Update(ctx context.Context, user *domain.UserModel) error {
	err := r.db.WithContext(ctx).Save(user).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to update user", "error", err, "user_id", user.ID)
		return wrapGormError(err)
	}
	slog.DebugContext(ctx, "user updated successfully", "user_id", user.ID, "email", user.Email)
	return nil
}

func (r *gormUserStore) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.UserModel{})
	if result.Error != nil {
		return wrapGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *gormUserStore) List(ctx context.Context, offset, limit int) ([]*domain.UserModel, error) {
	var users []*domain.UserModel
	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&users).Error
	if err != nil {
		slog.ErrorContext(
			ctx,
			"failed to list users",
			"error",
			err,
			"offset",
			offset,
			"limit",
			limit,
		)
		return nil, err
	}
	slog.DebugContext(
		ctx,
		"users listed successfully",
		"count",
		len(users),
		"offset",
		offset,
		"limit",
		limit,
	)
	return users, nil
}

func (r *gormUserStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.UserModel{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
