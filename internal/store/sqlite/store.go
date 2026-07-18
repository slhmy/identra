package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/slhmy/identra/internal/identra"
	"github.com/slhmy/identra/internal/store/sqlite/sqlitedb"
)

type UserStore struct {
	queries *sqlitedb.Queries
}

type ExternalIdentityStore struct {
	queries *sqlitedb.Queries
}

var _ identra.UserStore = (*UserStore)(nil)
var _ identra.ExternalIdentityStore = (*ExternalIdentityStore)(nil)

func NewStores(db sqlitedb.DBTX) (*UserStore, *ExternalIdentityStore) {
	queries := sqlitedb.New(db)
	return &UserStore{queries: queries}, &ExternalIdentityStore{queries: queries}
}

func (s *UserStore) Create(ctx context.Context, user *identra.UserModel) error {
	now := time.Now().UTC()
	id := user.ID
	if strings.TrimSpace(id) == "" {
		id = uuid.NewString()
	}
	createdAt := user.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	err := s.queries.CreateUser(ctx, sqlitedb.CreateUserParams{
		ID:             id,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
		Email:          user.Email,
		HashedPassword: nullString(user.HashedPassword),
		Hash:           nullString(user.VerificationHash),
		LastLoginAt:    nullTime(user.LastLoginAt),
	})
	if err != nil {
		return wrapError(err)
	}
	user.ID = id
	user.CreatedAt = createdAt
	user.UpdatedAt = now
	return nil
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*identra.UserModel, error) {
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		return nil, wrapError(err)
	}
	return userToDomain(user), nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*identra.UserModel, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, wrapError(err)
	}
	return userToDomain(user), nil
}

func (s *UserStore) Update(ctx context.Context, user *identra.UserModel) error {
	updatedAt := time.Now().UTC()
	rows, err := s.queries.UpdateUser(ctx, sqlitedb.UpdateUserParams{
		ID:             user.ID,
		Email:          user.Email,
		HashedPassword: nullString(user.HashedPassword),
		Hash:           nullString(user.VerificationHash),
		LastLoginAt:    nullTime(user.LastLoginAt),
		UpdatedAt:      updatedAt,
	})
	if err != nil {
		return wrapError(err)
	}
	if rows == 0 {
		return identra.ErrNotFound
	}
	user.UpdatedAt = updatedAt
	return nil
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	rows, err := s.queries.SoftDeleteUser(ctx, sqlitedb.SoftDeleteUserParams{
		ID:        id,
		DeletedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: now,
	})
	if err != nil {
		return wrapError(err)
	}
	if rows == 0 {
		return identra.ErrNotFound
	}
	return nil
}

func (s *UserStore) List(ctx context.Context, offset, limit int) ([]*identra.UserModel, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = -1
	}
	users, err := s.queries.ListUsers(ctx, sqlitedb.ListUsersParams{
		PageOffset: int64(offset),
		PageSize:   int64(limit),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*identra.UserModel, 0, len(users))
	for _, user := range users {
		result = append(result, userToDomain(user))
	}
	return result, nil
}

func (s *UserStore) Count(ctx context.Context) (int64, error) {
	return s.queries.CountUsers(ctx)
}

func (s *ExternalIdentityStore) Create(ctx context.Context, identity *identra.ExternalIdentityModel) error {
	now := time.Now().UTC()
	id := identity.ID
	if strings.TrimSpace(id) == "" {
		id = uuid.NewString()
	}
	createdAt := identity.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	err := s.queries.CreateExternalIdentity(ctx, sqlitedb.CreateExternalIdentityParams{
		ID:             id,
		UserID:         identity.UserID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	})
	if err != nil {
		return wrapError(err)
	}
	identity.ID = id
	identity.CreatedAt = createdAt
	identity.UpdatedAt = now
	return nil
}

func (s *ExternalIdentityStore) GetByProviderID(ctx context.Context, provider, providerUserID string) (*identra.ExternalIdentityModel, error) {
	identity, err := s.queries.GetExternalIdentityByProviderID(ctx, sqlitedb.GetExternalIdentityByProviderIDParams{
		Provider:       provider,
		ProviderUserID: providerUserID,
	})
	if err != nil {
		return nil, wrapError(err)
	}
	return externalIdentityToDomain(identity), nil
}

func (s *ExternalIdentityStore) GetByUserID(ctx context.Context, userID string) ([]*identra.ExternalIdentityModel, error) {
	identities, err := s.queries.ListExternalIdentitiesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]*identra.ExternalIdentityModel, 0, len(identities))
	for _, identity := range identities {
		result = append(result, externalIdentityToDomain(identity))
	}
	return result, nil
}

func wrapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return identra.ErrNotFound
	}
	if isUniqueConstraintError(err) {
		return identra.ErrAlreadyExists
	}
	return err
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "primary key constraint")
}

func userToDomain(user sqlitedb.User) *identra.UserModel {
	return &identra.UserModel{
		ID:               user.ID,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
		Email:            user.Email,
		HashedPassword:   stringPointer(user.HashedPassword),
		VerificationHash: stringPointer(user.Hash),
		LastLoginAt:      timePointer(user.LastLoginAt),
	}
}

func externalIdentityToDomain(identity sqlitedb.ExternalIdentity) *identra.ExternalIdentityModel {
	return &identra.ExternalIdentityModel{
		ID:             identity.ID,
		UserID:         identity.UserID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
		CreatedAt:      identity.CreatedAt,
		UpdatedAt:      identity.UpdatedAt,
	}
}

func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func timePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
