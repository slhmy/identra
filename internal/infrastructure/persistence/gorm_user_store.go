package persistence

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "unique constraint failed") ||
		strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "duplicate entry") {
		return domain.ErrAlreadyExists
	}
	return err
}

func (r *gormUserStore) Create(ctx context.Context, user *domain.UserModel) error {
	record := userRecordFromDomain(user)
	err := r.db.WithContext(ctx).Create(&record).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to create user", "error", err, "email", user.Email)
		return wrapGormError(err)
	}
	copyUserRecordToDomain(record, user)
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
	var record userRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to get user by ID", "error", err, "user_id", id)
		return nil, wrapGormError(err)
	}
	user := userRecordToDomain(record)
	slog.DebugContext(ctx, "user retrieved successfully", "user_id", user.ID, "email", user.Email)
	return user, nil
}

func (r *gormUserStore) GetByEmail(ctx context.Context, email string) (*domain.UserModel, error) {
	var record userRecord
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&record).Error
	if err != nil {
		return nil, wrapGormError(err)
	}
	return userRecordToDomain(record), nil
}

func (r *gormUserStore) Update(ctx context.Context, user *domain.UserModel) error {
	record := userRecordFromDomain(user)
	err := r.db.WithContext(ctx).Save(&record).Error
	if err != nil {
		slog.ErrorContext(ctx, "failed to update user", "error", err, "user_id", user.ID)
		return wrapGormError(err)
	}
	copyUserRecordToDomain(record, user)
	slog.DebugContext(ctx, "user updated successfully", "user_id", user.ID, "email", user.Email)
	return nil
}

func (r *gormUserStore) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&userRecord{})
	if result.Error != nil {
		return wrapGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *gormUserStore) List(ctx context.Context, offset, limit int) ([]*domain.UserModel, error) {
	var records []userRecord
	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&records).Error
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
	users := make([]*domain.UserModel, 0, len(records))
	for _, record := range records {
		users = append(users, userRecordToDomain(record))
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
	err := r.db.WithContext(ctx).Model(&userRecord{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
